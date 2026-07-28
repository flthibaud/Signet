package data

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

var (
	testDBOnce sync.Once
	testDBConn *sql.DB
	testDBErr  error
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("neither TEST_DATABASE_URL nor DATABASE_URL is set")
	}

	testDBOnce.Do(func() {
		testDBConn, testDBErr = sql.Open("postgres", dsn)
		if testDBErr != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		testDBErr = testDBConn.PingContext(ctx)
	})
	if testDBErr != nil {
		t.Skipf("database unreachable: %v", testDBErr)
	}

	return testDBConn
}

func exec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// seedUser inserts a user with a unique identity and removes it, along with
// everything cascading off it, when the test ends.
func seedUser(t *testing.T, db *sql.DB) uuid.UUID {
	t.Helper()

	id := uuid.New()
	exec(t, db,
		`INSERT INTO users (id, username, email, password_hash) VALUES ($1, $2, $3, $4)`,
		id, "test-"+id.String(), "test-"+id.String()+"@example.test", []byte("x"))

	t.Cleanup(func() {
		exec(t, db, `DELETE FROM users WHERE id = $1`, id)
	})

	return id
}

type articleFixture struct {
	url         string
	title       string
	description string
	author      string
	imageURL    string
	textContent string
	publishedAt *time.Time
}

// seedArticle inserts an article under a hash unique to this run, so a test
// never collides with a real row or with a parallel test.
func seedArticle(t *testing.T, db *sql.DB, f articleFixture) int64 {
	t.Helper()

	hash := "test-" + uuid.NewString()
	if f.url == "" {
		f.url = "https://example.test/" + hash
	}
	if f.title == "" {
		f.title = "Test article"
	}

	var id int64
	err := db.QueryRowContext(context.Background(), `
		INSERT INTO articles (url, hash, title, description, author, image_url, text_content, published_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`,
		f.url, hash, f.title, f.description, f.author, f.imageURL, f.textContent, f.publishedAt,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed article: %v", err)
	}

	t.Cleanup(func() {
		exec(t, db, `DELETE FROM articles WHERE id = $1`, id)
	})

	return id
}

func seedArticleWithNulls(t *testing.T, db *sql.DB) int64 {
	t.Helper()

	hash := "test-" + uuid.NewString()

	var id int64
	err := db.QueryRowContext(context.Background(), `
		INSERT INTO articles (url, hash, title, description, author, image_url, text_content)
		VALUES ($1, $2, 'Untitled', NULL, NULL, NULL, '')
		RETURNING id`,
		"https://example.test/"+hash, hash,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed article with nulls: %v", err)
	}

	t.Cleanup(func() {
		exec(t, db, `DELETE FROM articles WHERE id = $1`, id)
	})

	return id
}

type linkFixture struct {
	feedID          *int64
	isRead          bool
	isStarred       bool
	readingProgress float64
	anchorIndex     int
	archived        bool
	savedAt         *time.Time
}

func seedLink(t *testing.T, db *sql.DB, userID uuid.UUID, articleID int64, f linkFixture) string {
	t.Helper()

	slug := "test-" + uuid.NewString()
	savedAt := time.Now()
	if f.savedAt != nil {
		savedAt = *f.savedAt
	}

	var archivedAt any
	if f.archived {
		archivedAt = savedAt
	}

	exec(t, db, `
		INSERT INTO links (user_id, article_id, feed_id, slug, is_read, is_starred,
			reading_progress, reading_progress_anchor_index, archived_at, saved_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		userID, articleID, f.feedID, slug, f.isRead, f.isStarred,
		f.readingProgress, f.anchorIndex, archivedAt, savedAt)

	return slug
}

func seedFeed(t *testing.T, db *sql.DB, title, imageURL string) int64 {
	t.Helper()

	var id int64
	err := db.QueryRowContext(context.Background(), `
		INSERT INTO feeds (url, original_title, image_url)
		VALUES ($1, $2, $3)
		RETURNING id`,
		fmt.Sprintf("https://example.test/feed/%s.xml", uuid.NewString()), title, imageURL,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed feed: %v", err)
	}

	t.Cleanup(func() {
		exec(t, db, `DELETE FROM feeds WHERE id = $1`, id)
	})

	return id
}

func timePtr(tm time.Time) *time.Time { return &tm }
