package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Statuses an import job goes through. There is no "pending": the job is
// created by the goroutine that immediately starts working on it.
const (
	OPMLImportRunning     = "running"
	OPMLImportCompleted   = "completed"
	OPMLImportInterrupted = "interrupted"
)

// Per-entry outcomes, as recorded in OPMLImport.Results.
const (
	OPMLEntryImported = "imported"
	OPMLEntrySkipped  = "skipped"
	OPMLEntryFailed   = "failed"
)

// OPMLImportResult is one line of the report shown to the user.
type OPMLImportResult struct {
	URL    string `json:"url"`
	Title  string `json:"title,omitempty"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// OPMLImport tracks one import job. Importing 200 feeds means 200 HTTP fetches,
// far too long for a request, so the work runs detached and its progress lives
// here — which also means it survives a page reload.
type OPMLImport struct {
	ID         uuid.UUID          `json:"id"`
	UserID     uuid.UUID          `json:"-"`
	Status     string             `json:"status"`
	Total      int                `json:"total"`
	Processed  int                `json:"processed"`
	Imported   int                `json:"imported"`
	Skipped    int                `json:"skipped"`
	Failed     int                `json:"failed"`
	Results    []OPMLImportResult `json:"results"`
	CreatedAt  time.Time          `json:"created_at"`
	UpdatedAt  time.Time          `json:"updated_at"`
	FinishedAt *time.Time         `json:"finished_at"`
}

type OPMLImportModel struct {
	DB *sql.DB
}

// Insert records a new job, replacing the user's previous one.
//
// Keeping only the latest bounds the table by the number of accounts without
// any background purge. The row has to outlive the job itself — it carries the
// report the user reads once the import is done — so it cannot simply be
// deleted on completion.
func (m OPMLImportModel) Insert(ctx context.Context, imp *OPMLImport) error {
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM opml_imports WHERE user_id = $1`, imp.UserID); err != nil {
		return err
	}

	query := `
		INSERT INTO opml_imports (user_id, status, total)
		VALUES ($1, $2, $3)
		RETURNING id, status, created_at, updated_at`

	err = tx.QueryRowContext(ctx, query, imp.UserID, OPMLImportRunning, imp.Total).Scan(
		&imp.ID,
		&imp.Status,
		&imp.CreatedAt,
		&imp.UpdatedAt,
	)
	if err != nil {
		return err
	}

	imp.Results = []OPMLImportResult{}

	return tx.Commit()
}

// GetLatestForUser returns the user's most recent import, or
// ErrRecordNotFound. It backs both the polling and the "what happened last
// time" view after a reload, which is why no lookup by id is needed.
func (m OPMLImportModel) GetLatestForUser(ctx context.Context, userID uuid.UUID) (*OPMLImport, error) {
	query := `
		SELECT id, user_id, status, total, processed, imported, skipped, failed,
		       results, created_at, updated_at, finished_at
		FROM opml_imports
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 1`

	var imp OPMLImport
	var results []byte

	err := m.DB.QueryRowContext(ctx, query, userID).Scan(
		&imp.ID,
		&imp.UserID,
		&imp.Status,
		&imp.Total,
		&imp.Processed,
		&imp.Imported,
		&imp.Skipped,
		&imp.Failed,
		&results,
		&imp.CreatedAt,
		&imp.UpdatedAt,
		&imp.FinishedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}

	if err := json.Unmarshal(results, &imp.Results); err != nil {
		return nil, err
	}

	return &imp, nil
}

// AppendResult records one entry's outcome and moves the counters, in a single
// statement so progress is always internally consistent.
//
// The UPDATE also refreshes updated_at through the table's trigger, which is
// what FailStale reads as a heartbeat.
func (m OPMLImportModel) AppendResult(ctx context.Context, id uuid.UUID, result OPMLImportResult) error {
	encoded, err := json.Marshal([]OPMLImportResult{result})
	if err != nil {
		return err
	}

	query := `
		UPDATE opml_imports
		SET results   = results || $2::jsonb,
		    processed = processed + 1,
		    imported  = imported + CASE WHEN $3 = 'imported' THEN 1 ELSE 0 END,
		    skipped   = skipped  + CASE WHEN $3 = 'skipped'  THEN 1 ELSE 0 END,
		    failed    = failed   + CASE WHEN $3 = 'failed'   THEN 1 ELSE 0 END
		WHERE id = $1`

	_, err = m.DB.ExecContext(ctx, query, id, encoded, result.Status)
	return err
}

// MarkFinished closes a job.
func (m OPMLImportModel) MarkFinished(ctx context.Context, id uuid.UUID, status string) error {
	query := `
		UPDATE opml_imports
		SET status = $2, finished_at = NOW()
		WHERE id = $1`

	_, err := m.DB.ExecContext(ctx, query, id, status)
	return err
}

// FailStale closes jobs whose worker died without saying so — a crash, a
// SIGKILL, a container replaced mid-import. Without it they would poll as
// "running" forever.
func (m OPMLImportModel) FailStale(ctx context.Context, olderThan time.Duration) (int64, error) {
	query := `
		UPDATE opml_imports
		SET status = $1, finished_at = NOW()
		WHERE status = $2
		  AND updated_at < NOW() - make_interval(secs => $3::double precision)`

	result, err := m.DB.ExecContext(ctx, query, OPMLImportInterrupted, OPMLImportRunning, olderThan.Seconds())
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}
