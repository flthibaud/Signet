package data

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestOPMLImportLifecycle walks a job from creation to completion, which is
// also what the polling endpoint reads at each step.
func TestOPMLImportLifecycle(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	model := OPMLImportModel{DB: db}
	userID := seedUser(t, db)

	imp := &OPMLImport{UserID: userID, Total: 3}
	if err := model.Insert(ctx, imp); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if imp.Status != OPMLImportRunning {
		t.Errorf("status = %q, want %q", imp.Status, OPMLImportRunning)
	}

	results := []OPMLImportResult{
		{URL: "https://a.test/feed", Title: "A", Status: OPMLEntryImported},
		{URL: "https://b.test/feed", Title: "B", Status: OPMLEntrySkipped, Reason: "already subscribed"},
		{URL: "https://c.test/feed", Title: "C", Status: OPMLEntryFailed, Reason: "unreachable"},
	}
	for _, r := range results {
		if err := model.AppendResult(ctx, imp.ID, r); err != nil {
			t.Fatalf("AppendResult(%s): %v", r.Status, err)
		}
	}

	got, err := model.GetLatestForUser(ctx, userID)
	if err != nil {
		t.Fatalf("GetLatestForUser: %v", err)
	}

	if got.Processed != 3 || got.Imported != 1 || got.Skipped != 1 || got.Failed != 1 {
		t.Errorf("counters = processed %d / imported %d / skipped %d / failed %d, want 3/1/1/1",
			got.Processed, got.Imported, got.Skipped, got.Failed)
	}
	if len(got.Results) != 3 {
		t.Fatalf("got %d results, want 3", len(got.Results))
	}
	// Order is the order of work, and the reason survives the round trip
	// through JSONB — it is the only thing telling the user what went wrong.
	if got.Results[2].Reason != "unreachable" || got.Results[2].URL != "https://c.test/feed" {
		t.Errorf("unexpected last result: %+v", got.Results[2])
	}
	if got.FinishedAt != nil {
		t.Error("a running job should have no finished_at")
	}

	if err := model.MarkFinished(ctx, imp.ID, OPMLImportCompleted); err != nil {
		t.Fatalf("MarkFinished: %v", err)
	}

	got, err = model.GetLatestForUser(ctx, userID)
	if err != nil {
		t.Fatalf("GetLatestForUser after finishing: %v", err)
	}
	if got.Status != OPMLImportCompleted || got.FinishedAt == nil {
		t.Errorf("status = %q, finished_at = %v", got.Status, got.FinishedAt)
	}
}

// TestOPMLImportInsertReplaces pins how the table stays bounded: one row per
// user, the previous import making way for the new one.
func TestOPMLImportInsertReplaces(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	model := OPMLImportModel{DB: db}
	userID := seedUser(t, db)
	other := seedUser(t, db)

	first := &OPMLImport{UserID: userID, Total: 1}
	if err := model.Insert(ctx, first); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	theirs := &OPMLImport{UserID: other, Total: 7}
	if err := model.Insert(ctx, theirs); err != nil {
		t.Fatalf("Insert for another user: %v", err)
	}

	second := &OPMLImport{UserID: userID, Total: 42}
	if err := model.Insert(ctx, second); err != nil {
		t.Fatalf("Insert (second): %v", err)
	}

	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM opml_imports WHERE user_id = $1`, userID).Scan(&count)
	if err != nil {
		t.Fatalf("counting imports: %v", err)
	}
	if count != 1 {
		t.Errorf("got %d imports for the user, want 1", count)
	}

	latest, err := model.GetLatestForUser(ctx, userID)
	if err != nil {
		t.Fatalf("GetLatestForUser: %v", err)
	}
	if latest.ID != second.ID || latest.Total != 42 {
		t.Errorf("latest = %+v, want the second import", latest)
	}

	// Replacing one user's import must leave everyone else's alone.
	if _, err := model.GetLatestForUser(ctx, other); err != nil {
		t.Errorf("another user's import was deleted: %v", err)
	}
}

func TestOPMLImportGetLatestNone(t *testing.T) {
	db := testDB(t)

	model := OPMLImportModel{DB: db}
	userID := seedUser(t, db)

	_, err := model.GetLatestForUser(context.Background(), userID)
	if !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("err = %v, want ErrRecordNotFound", err)
	}
}

// TestOPMLImportFailStale covers the case a crash leaves behind: a job nothing
// is working on any more must stop reporting itself as running.
func TestOPMLImportFailStale(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	model := OPMLImportModel{DB: db}
	userID := seedUser(t, db)

	imp := &OPMLImport{UserID: userID, Total: 5}
	if err := model.Insert(ctx, imp); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	// A fresh job is not stale.
	if _, err := model.FailStale(ctx, time.Hour); err != nil {
		t.Fatalf("FailStale: %v", err)
	}
	got, err := model.GetLatestForUser(ctx, userID)
	if err != nil {
		t.Fatalf("GetLatestForUser: %v", err)
	}
	if got.Status != OPMLImportRunning {
		t.Fatalf("a fresh job was swept: status = %q", got.Status)
	}

	// The row cannot be aged by hand — update_opml_imports_modtime resets
	// updated_at on every UPDATE, which is precisely what makes it a reliable
	// heartbeat. So the threshold is moved instead: a negative one puts the
	// cutoff in the future, making every running job stale.
	swept, err := model.FailStale(ctx, -time.Second)
	if err != nil {
		t.Fatalf("FailStale: %v", err)
	}
	if swept == 0 {
		t.Fatal("FailStale reported no job swept")
	}

	got, err = model.GetLatestForUser(ctx, userID)
	if err != nil {
		t.Fatalf("GetLatestForUser: %v", err)
	}
	if got.Status != OPMLImportInterrupted {
		t.Errorf("status = %q, want %q", got.Status, OPMLImportInterrupted)
	}
	if got.FinishedAt == nil {
		t.Error("an interrupted job should carry finished_at")
	}
}
