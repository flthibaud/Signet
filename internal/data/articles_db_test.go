package data

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
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
