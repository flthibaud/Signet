package data

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrDuplicateFolder reports that the user already has a folder by that name.
// Names are unique per user, not globally.
var ErrDuplicateFolder = errors.New("duplicate folder")

// folderNameConstraint is the unique index backing "one folder name per user".
const folderNameConstraint = "folders_user_id_name_key"

func duplicateFolderName(err error) bool {
	return isUniqueViolation(err, folderNameConstraint)
}

// Folder groups a user's subscriptions. Folders are flat:
// an OPML file with nested outlines is folded into a single name on import.
type Folder struct {
	ID        int64     `json:"id"`
	UserID    uuid.UUID `json:"-"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// FolderModel gives access to the folders table.
type FolderModel struct {
	DB *sql.DB
}

// GetOrCreate returns the user's folder by name, creating it if needed.
func (m FolderModel) GetOrCreate(ctx context.Context, userID uuid.UUID, name string) (*Folder, error) {
	query := `
		INSERT INTO folders (user_id, name)
		VALUES ($1, $2)
		ON CONFLICT (user_id, name) DO UPDATE SET name = folders.name
		RETURNING id, user_id, name, created_at`

	var folder Folder

	err := m.DB.QueryRowContext(ctx, query, userID, name).Scan(
		&folder.ID,
		&folder.UserID,
		&folder.Name,
		&folder.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &folder, nil
}

// Get returns one of the user's folders. Scoping every read and write to
// user_id is what keeps one user out of another's folders.
func (m FolderModel) Get(ctx context.Context, userID uuid.UUID, id int64) (*Folder, error) {
	query := `
		SELECT id, user_id, name, created_at
		FROM folders
		WHERE id = $1 AND user_id = $2`

	var folder Folder

	err := m.DB.QueryRowContext(ctx, query, id, userID).Scan(
		&folder.ID,
		&folder.UserID,
		&folder.Name,
		&folder.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}

	return &folder, nil
}

// Insert creates a folder and sets its generated ID and CreatedAt. Returns
// ErrDuplicateFolder if the user already has one by that name. Use GetOrCreate
// instead where an existing folder should be reused rather than reported.
func (m FolderModel) Insert(ctx context.Context, folder *Folder) error {
	query := `
		INSERT INTO folders (user_id, name)
		VALUES ($1, $2)
		RETURNING id, created_at`

	err := m.DB.QueryRowContext(ctx, query, folder.UserID, folder.Name).Scan(
		&folder.ID,
		&folder.CreatedAt,
	)
	if err != nil {
		if duplicateFolderName(err) {
			return ErrDuplicateFolder
		}
		return err
	}

	return nil
}

// Update renames a folder. Returns ErrDuplicateFolder if the new name is taken,
// and ErrRecordNotFound if the folder does not exist or belongs to someone else
// — the two are not distinguished.
func (m FolderModel) Update(ctx context.Context, userID uuid.UUID, id int64, name string) error {
	query := `
		UPDATE folders
		SET name = $1
		WHERE id = $2 AND user_id = $3`

	result, err := m.DB.ExecContext(ctx, query, name, id, userID)
	if err != nil {
		if duplicateFolderName(err) {
			return ErrDuplicateFolder
		}
		return err
	}

	return checkOneRow(result)
}

// Delete removes a folder. The subscriptions it held are not touched: the
// foreign key is ON DELETE SET NULL, so they fall back to unfiled.
func (m FolderModel) Delete(ctx context.Context, userID uuid.UUID, id int64) error {
	query := `DELETE FROM folders WHERE id = $1 AND user_id = $2`

	result, err := m.DB.ExecContext(ctx, query, id, userID)
	if err != nil {
		return err
	}

	return checkOneRow(result)
}

// GetAllForUser returns the user's folders, alphabetically.
func (m FolderModel) GetAllForUser(ctx context.Context, userID uuid.UUID) ([]*Folder, error) {
	query := `
		SELECT id, user_id, name, created_at
		FROM folders
		WHERE user_id = $1
		ORDER BY name`

	rows, err := m.DB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	folders := []*Folder{}

	for rows.Next() {
		var folder Folder
		if err := rows.Scan(&folder.ID, &folder.UserID, &folder.Name, &folder.CreatedAt); err != nil {
			return nil, err
		}
		folders = append(folders, &folder)
	}

	return folders, rows.Err()
}
