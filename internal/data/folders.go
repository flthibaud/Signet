package data

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// Folder groups a user's subscriptions. Folders are flat:
// an OPML file with nested outlines is folded into a single name on import.
type Folder struct {
	ID        int64     `json:"id"`
	UserID    uuid.UUID `json:"-"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

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
