package data

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// The reading time is the fetcher's to compute; the database must store what it
// is given. A trigger used to overwrite it with an uncapped formula, which made a
// very long article fail chk_reading_time on every sync.
func TestArticleInsertKeepsReadingTime(t *testing.T) {
	db := testDB(t)
	m := ArticleModel{DB: db}

	for _, tc := range []struct {
		name        string
		text        string
		readingTime float64
	}{
		{"short", "three words here", 0.015},
		{"past the old trigger's ceiling", strings.Repeat("word ", 120_000), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			article := &Article{
				Url:         "https://example.test/" + uuid.NewString(),
				Hash:        "test-" + uuid.NewString(),
				Title:       "Test article",
				TextContent: tc.text,
				ReadingTime: tc.readingTime,
				Language:    SimpleTextSearchConfig,
				PublishedAt: time.Now(),
			}
			if err := m.Insert(context.Background(), article); err != nil {
				t.Fatalf("insert: %v", err)
			}
			t.Cleanup(func() {
				exec(t, db, `DELETE FROM articles WHERE id = $1`, article.ID)
			})

			var stored float64
			err := db.QueryRowContext(context.Background(),
				`SELECT reading_time_minutes FROM articles WHERE id = $1`, article.ID).Scan(&stored)
			if err != nil {
				t.Fatal(err)
			}
			// reading_time_minutes is a REAL, so compare at float32 precision.
			if float32(stored) != float32(tc.readingTime) {
				t.Errorf("stored reading time = %v, want %v", stored, tc.readingTime)
			}
		})
	}
}

func TestArticleIDByHash(t *testing.T) {
	db := testDB(t)
	m := ArticleModel{DB: db}
	ctx := context.Background()

	id := seedArticle(t, db, articleFixture{})
	var hash string
	if err := db.QueryRowContext(ctx, `SELECT hash FROM articles WHERE id = $1`, id).Scan(&hash); err != nil {
		t.Fatal(err)
	}

	got, err := m.IDByHash(ctx, hash)
	if err != nil {
		t.Fatalf("IDByHash: %v", err)
	}
	if got != id {
		t.Errorf("IDByHash = %d, want %d", got, id)
	}

	if _, err := m.IDByHash(ctx, "test-"+uuid.NewString()); !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("unknown hash: err = %v, want ErrRecordNotFound", err)
	}
}

// The feed sync tells an item the database will always refuse from one that may
// go through next time, and only lets the first one advance the feed's ETag.
func TestArticleInsertReportsUnstorableData(t *testing.T) {
	db := testDB(t)
	m := ArticleModel{DB: db}

	for _, tc := range []struct {
		name   string
		mutate func(a *Article)
	}{
		{"check constraint (class 23)", func(a *Article) { a.ReadingTime = 501 }},
		{"NUL byte in a text column (class 22)", func(a *Article) { a.Title = "bad \x00 title" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			article := &Article{
				Url:         "https://example.test/" + uuid.NewString(),
				Hash:        "test-" + uuid.NewString(),
				Title:       "Test article",
				Language:    SimpleTextSearchConfig,
				PublishedAt: time.Now(),
			}
			tc.mutate(article)

			err := m.Insert(context.Background(), article)
			if !errors.Is(err, ErrUnstorable) {
				t.Fatalf("err = %v, want it to wrap ErrUnstorable", err)
			}
			var pqErr *pq.Error
			if !errors.As(err, &pqErr) {
				t.Errorf("err = %v, want the driver's error still reachable", err)
			}
		})
	}
}
