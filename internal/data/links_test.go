package data

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func boolPtr(b bool) *bool       { return &b }
func int64Ptr(n int64) *int64    { return &n }
func floatPtr(f float64) *float64 { return &f }
func intPtr(n int) *int          { return &n }

func TestBuildLinkFiltersWhere(t *testing.T) {
	userID := uuid.New()

	tests := []struct {
		name      string
		filters   LinkFilters
		wantWhere []string
		wantArgs  []any
	}{
		{
			name:      "no filters defaults to non-archived",
			filters:   LinkFilters{},
			wantWhere: []string{"l.user_id = $1", "l.archived_at IS NULL"},
			wantArgs:  []any{userID},
		},
		{
			name:      "archived only",
			filters:   LinkFilters{Archived: true},
			wantWhere: []string{"l.user_id = $1", "l.archived_at IS NOT NULL"},
			wantArgs:  []any{userID},
		},
		{
			name:    "all filters keep placeholders in sync with args",
			filters: LinkFilters{IsRead: boolPtr(false), IsStarred: boolPtr(true), FeedID: int64Ptr(42)},
			wantWhere: []string{
				"l.user_id = $1",
				"l.archived_at IS NULL",
				"l.is_read = $2",
				"l.is_starred = $3",
				"l.feed_id = $4",
			},
			wantArgs: []any{userID, false, true, int64(42)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			where, args := buildLinkFiltersWhere(userID, tt.filters)
			if !reflect.DeepEqual(where, tt.wantWhere) {
				t.Errorf("where = %v, want %v", where, tt.wantWhere)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("args = %v, want %v", args, tt.wantArgs)
			}
		})
	}
}

func TestBuildLinkUpdateSet(t *testing.T) {
	tests := []struct {
		name     string
		update   LinkUpdate
		wantSet  []string
		wantArgs []any
	}{
		{
			name:     "empty update produces no clauses",
			update:   LinkUpdate{},
			wantSet:  nil,
			wantArgs: nil,
		},
		{
			name:     "read only",
			update:   LinkUpdate{IsRead: boolPtr(true)},
			wantSet:  []string{"is_read = $1"},
			wantArgs: []any{true},
		},
		{
			name:     "archive sets archived_at without an arg",
			update:   LinkUpdate{Archived: boolPtr(true)},
			wantSet:  []string{"archived_at = NOW()"},
			wantArgs: nil,
		},
		{
			name:     "unarchive clears archived_at",
			update:   LinkUpdate{Archived: boolPtr(false)},
			wantSet:  []string{"archived_at = NULL"},
			wantArgs: nil,
		},
		{
			name: "all fields keep placeholders in sync with args",
			update: LinkUpdate{
				IsRead:                     boolPtr(false),
				IsStarred:                  boolPtr(true),
				Archived:                   boolPtr(true),
				ReadingProgress:            floatPtr(0.5),
				ReadingProgressAnchorIndex: intPtr(3),
			},
			wantSet: []string{
				"is_read = $1",
				"is_starred = $2",
				"reading_progress = $3",
				"reading_progress_anchor_index = $4",
				"archived_at = NOW()",
			},
			wantArgs: []any{false, true, 0.5, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set, args := buildLinkUpdateSet(tt.update)
			if !reflect.DeepEqual(set, tt.wantSet) {
				t.Errorf("set = %v, want %v", set, tt.wantSet)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("args = %v, want %v", args, tt.wantArgs)
			}
		})
	}
}

func TestLinkUpdateIsEmpty(t *testing.T) {
	if !(LinkUpdate{}).IsEmpty() {
		t.Error("zero LinkUpdate should be empty")
	}
	if (LinkUpdate{IsStarred: boolPtr(true)}).IsEmpty() {
		t.Error("LinkUpdate with a field set should not be empty")
	}
}
