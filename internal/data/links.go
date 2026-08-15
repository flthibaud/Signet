package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Link is a user's saved copy of an article. ArchivedAt is a pointer because
// links.archived_at is nullable and "not archived" has to serialize as null —
// a zero time.Time would send back "0001-01-01T00:00:00Z" instead.
//
// ReadingProgressAnchorIndex uses the same JSON name as the PATCH body field
// that writes it, so a client can round-trip what it sent.
type Link struct {
	ID                         int64      `json:"id"`
	UserID                     uuid.UUID  `json:"-"`
	ArticleID                  int64      `json:"-"`
	Slug                       string     `json:"slug"`
	ArticleUrl                 string     `json:"article_url"`
	IsRead                     bool       `json:"is_read"`
	IsStarred                  bool       `json:"is_starred"`
	ReadingProgress            float64    `json:"reading_progress"`
	ReadingProgressAnchorIndex int        `json:"reading_progress_anchor_index"`
	FeedID                     *int64     `json:"feed_id"`
	CreatedAt                  time.Time  `json:"created_at"`
	SavedAt                    time.Time  `json:"saved_at"`
	ArchivedAt                 *time.Time `json:"archived_at"`
	UpdatedAt                  time.Time  `json:"updated_at"`
}

// LinkWithArticle combines a user's link state with the article content.
type LinkWithArticle struct {
	Link
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Author      string    `json:"author"`
	ImageURL    string    `json:"image_url,omitempty"`
	ReadingTime float64   `json:"reading_time_minutes"`
	FeedTitle   *string   `json:"feed_title,omitempty"`
	PublishedAt time.Time `json:"published_at"`
}

// LinkDetail is used for the single article detail view, includes text content.
type LinkDetail struct {
	Link
	Title       string    `json:"title"`
	Author      string    `json:"author"`
	ImageURL    string    `json:"image_url,omitempty"`
	ReadingTime float64   `json:"reading_time_minutes"`
	TextContent string    `json:"text_content"`
	FeedTitle   *string   `json:"feed_title,omitempty"`
	PublishedAt time.Time `json:"published_at"`
}

type LinkModel struct {
	DB *sql.DB
}

type LinkFilters struct {
	IsRead    *bool
	IsStarred *bool
	Archived  *bool
	FeedID    *int64
	FolderID  *int64
}

// buildLinkFiltersWhere returns the WHERE clauses and their args for f.
// The first placeholder is $1 (userID); filter args start at $2.
func buildLinkFiltersWhere(userID uuid.UUID, f LinkFilters) ([]string, []any) {
	where := []string{"l.user_id = $1"}
	args := []any{userID}

	if f.Archived != nil {
		if *f.Archived {
			where = append(where, "l.archived_at IS NOT NULL")
		} else {
			where = append(where, "l.archived_at IS NULL")
		}
	}
	if f.IsRead != nil {
		args = append(args, *f.IsRead)
		where = append(where, fmt.Sprintf("l.is_read = $%d", len(args)))
	}
	if f.IsStarred != nil {
		args = append(args, *f.IsStarred)
		where = append(where, fmt.Sprintf("l.is_starred = $%d", len(args)))
	}
	if f.FeedID != nil {
		args = append(args, *f.FeedID)
		where = append(where, fmt.Sprintf("l.feed_id = $%d", len(args)))
	}

	if f.FolderID != nil {
		args = append(args, *f.FolderID)
		where = append(where, fmt.Sprintf(
			"l.feed_id IN (SELECT s.feed_id FROM subscriptions s WHERE s.user_id = $1 AND s.folder_id = $%d)",
			len(args)))
	}

	return where, args
}

// ListForUser returns one page of the user's links, plus whether a further
// page exists.
func (m LinkModel) ListForUser(ctx context.Context, userID uuid.UUID, filters LinkFilters, limit, offset int) ([]*LinkWithArticle, bool, error) {
	where, args := buildLinkFiltersWhere(userID, filters)

	// #nosec G201 -- the only interpolated values are placeholder indices
	// derived from len(args) and the clause strings buildLinkFiltersWhere
	// assembles from hardcoded column names. Every user-supplied value travels
	// in args as a bind parameter.
	query := fmt.Sprintf(`
		SELECT l.id, l.article_id, l.slug, l.feed_id, l.is_read, l.is_starred, COALESCE(l.reading_progress, 0),
			l.reading_progress_anchor_index,
			l.saved_at, l.created_at, l.updated_at, l.archived_at,
			a.url,
			a.title, COALESCE(a.description, ''), COALESCE(a.author, ''),
			COALESCE(NULLIF(a.image_url, ''), f.image_url, '') AS image_url,
			a.reading_time_minutes, l.published_at,
			f.original_title
		FROM links l
		JOIN articles a ON l.article_id = a.id
		LEFT JOIN feeds f ON l.feed_id = f.id
		WHERE %s
		ORDER BY l.published_at DESC, l.id DESC
		LIMIT $%d OFFSET $%d`,
		strings.Join(where, " AND "), len(args)+1, len(args)+2)

	args = append(args, limit+1, offset)

	rows, err := m.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var links []*LinkWithArticle

	for rows.Next() {
		var link LinkWithArticle
		err := rows.Scan(
			&link.ID,
			&link.ArticleID,
			&link.Slug,
			&link.FeedID,
			&link.IsRead,
			&link.IsStarred,
			&link.ReadingProgress,
			&link.ReadingProgressAnchorIndex,
			&link.SavedAt,
			&link.CreatedAt,
			&link.UpdatedAt,
			&link.ArchivedAt,
			&link.ArticleUrl,
			&link.Title,
			&link.Description,
			&link.Author,
			&link.ImageURL,
			&link.ReadingTime,
			&link.PublishedAt,
			&link.FeedTitle,
		)
		if err != nil {
			return nil, false, err
		}
		link.UserID = userID
		links = append(links, &link)
	}

	if err = rows.Err(); err != nil {
		return nil, false, err
	}

	hasMore := len(links) > limit
	if hasMore {
		links = links[:limit]
	}

	return links, hasMore, nil
}

// GetBySlug returns one of the user's links, article body included.
func (m LinkModel) GetBySlug(ctx context.Context, slug string, userID uuid.UUID) (*LinkDetail, error) {
	query := `
		SELECT l.id, l.article_id, l.slug, l.feed_id, l.is_read, l.is_starred, COALESCE(l.reading_progress, 0),
			l.reading_progress_anchor_index,
			l.saved_at, l.created_at, l.updated_at, l.archived_at,
			a.url,
			a.title, COALESCE(a.author, ''),
			COALESCE(NULLIF(a.image_url, ''), f.image_url, '') AS image_url,
			a.reading_time_minutes,
			a.text_content, l.published_at,
			f.original_title
		FROM links l
		JOIN articles a ON l.article_id = a.id
		LEFT JOIN feeds f ON l.feed_id = f.id
		WHERE l.slug = $1 AND l.user_id = $2`

	var link LinkDetail
	err := m.DB.QueryRowContext(ctx, query, slug, userID).Scan(
		&link.ID,
		&link.ArticleID,
		&link.Slug,
		&link.FeedID,
		&link.IsRead,
		&link.IsStarred,
		&link.ReadingProgress,
		&link.ReadingProgressAnchorIndex,
		&link.SavedAt,
		&link.CreatedAt,
		&link.UpdatedAt,
		&link.ArchivedAt,
		&link.ArticleUrl,
		&link.Title,
		&link.Author,
		&link.ImageURL,
		&link.ReadingTime,
		&link.TextContent,
		&link.PublishedAt,
		&link.FeedTitle,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}

	link.UserID = userID
	return &link, nil
}

func (m LinkModel) Exists(ctx context.Context, userID uuid.UUID, articleID int64) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM links
			WHERE user_id = $1 AND article_id = $2
		)`

	var exists bool
	err := m.DB.QueryRowContext(ctx, query, userID, articleID).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, nil
}

// BulkInsertForArticle creates links for multiple users in a single query.
// Uses ON CONFLICT DO NOTHING to skip users who already have this article.
//
// The user ids travel as one array parameter rather than as a generated VALUES
// list. A row per user would have cost four placeholders each, and Postgres caps
// a statement at 65535 of them — a feed passing ~16k subscribers would have
// started failing the insert outright, losing the article for every subscriber
// at once rather than degrading.
func (m LinkModel) BulkInsertForArticle(ctx context.Context, userIDs []uuid.UUID, articleID int64, feedID int64, baseSlug string) error {
	if len(userIDs) == 0 {
		return nil
	}

	// pq has no array encoder for uuid.UUID; the text form is cast back on the
	// other side by the ::uuid[] annotation.
	ids := make([]string, len(userIDs))
	for i, uid := range userIDs {
		ids[i] = uid.String()
	}

	// Each user has its own slug namespace (UNIQUE(user_id, slug)), so every
	// subscriber gets the same baseSlug.
	query := `
		INSERT INTO links (user_id, article_id, feed_id, slug, saved_at, updated_at)
		SELECT u.id, $2, $3, $4, NOW(), NOW()
		FROM unnest($1::uuid[]) AS u(id)
		ON CONFLICT (user_id, article_id) DO NOTHING`

	_, err := m.DB.ExecContext(ctx, query, pq.Array(ids), articleID, feedID, baseSlug)
	return err
}

type LinkUpdate struct {
	IsRead                     *bool
	IsStarred                  *bool
	Archived                   *bool
	ReadingProgress            *float64
	ReadingProgressAnchorIndex *int
}

func (u LinkUpdate) IsEmpty() bool {
	return u.IsRead == nil && u.IsStarred == nil && u.Archived == nil &&
		u.ReadingProgress == nil && u.ReadingProgressAnchorIndex == nil
}

// buildLinkUpdateSet returns the SET clauses and their args for u,
// with placeholders numbered from $1.
func buildLinkUpdateSet(u LinkUpdate) ([]string, []any) {
	var set []string
	var args []any

	add := func(column string, val any) {
		args = append(args, val)
		set = append(set, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	if u.IsRead != nil {
		add("is_read", *u.IsRead)
	}
	if u.IsStarred != nil {
		add("is_starred", *u.IsStarred)
	}
	if u.ReadingProgress != nil {
		add("reading_progress", *u.ReadingProgress)
	}
	if u.ReadingProgressAnchorIndex != nil {
		add("reading_progress_anchor_index", *u.ReadingProgressAnchorIndex)
	}
	if u.Archived != nil {
		if *u.Archived {
			set = append(set, "archived_at = NOW()")
		} else {
			set = append(set, "archived_at = NULL")
		}
	}

	return set, args
}

// Update applies a partial update to a user's link, identified by slug.
// It returns ErrRecordNotFound if no matching link exists for that user.
func (m LinkModel) Update(ctx context.Context, userID uuid.UUID, slug string, upd LinkUpdate) error {
	set, args := buildLinkUpdateSet(upd)
	if len(set) == 0 {
		return nil
	}

	// #nosec G201 -- buildLinkUpdateSet emits hardcoded column names against
	// placeholder indices; the values it sets are bound through args.
	query := fmt.Sprintf(`
		UPDATE links
		SET %s, updated_at = NOW()
		WHERE slug = $%d AND user_id = $%d`,
		strings.Join(set, ", "), len(args)+1, len(args)+2)
	args = append(args, slug, userID)

	result, err := m.DB.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrRecordNotFound
	}

	return nil
}
