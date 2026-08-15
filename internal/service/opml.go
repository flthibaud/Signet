package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/jsonlog"
	"github.com/flthibaud/signet/internal/opml"
	"github.com/google/uuid"
)

// opmlImportWorkers bounds how many feeds are subscribed to at once.
const opmlImportWorkers = 4

// finishTimeout bounds the closing writes, which run on a context detached from
// cancellation so a shutdown still records how far the import got.
const finishTimeout = 5 * time.Second

// The interfaces below are sized to what the import actually calls, so the
// worker can be tested with fakes. data.FolderModel, data.OPMLImportModel,
// data.FeedModel and *SubscriptionService satisfy them as they are.

type folderStore interface {
	GetOrCreate(ctx context.Context, userID uuid.UUID, name string) (*data.Folder, error)
}

type importStore interface {
	Insert(ctx context.Context, imp *data.OPMLImport) error
	AppendResult(ctx context.Context, id uuid.UUID, result data.OPMLImportResult) error
	MarkFinished(ctx context.Context, id uuid.UUID, status string) error
}

type feedMarker interface {
	MarkDueForSync(ctx context.Context, ids []int64) error
}

type subscriber interface {
	Subscribe(ctx context.Context, userID uuid.UUID, feedURL string, opts SubscribeOptions) (*data.Subscription, error)
}

// SyncTrigger asks the scheduler to run a sync pass now instead of waiting for
// its next tick. The scheduler is built after the services, so it is injected
// afterwards through SetSyncTrigger.
type SyncTrigger interface {
	TriggerSync()
}

// OPMLService runs subscription-list imports.
type OPMLService struct {
	subs    subscriber
	folders folderStore
	imports importStore
	feeds   feedMarker
	logger  *jsonlog.Logger

	// syncTrigger is optional: without it the imported feeds simply wait for
	// the scheduler's next tick.
	syncTrigger SyncTrigger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewOPMLService builds the service with a context of its own, since an import
// runs in the background well past the upload request that started it. Pair it
// with Shutdown. The scheduler is wired in afterwards through SetSyncTrigger.
func NewOPMLService(models data.Models, subs subscriber, logger *jsonlog.Logger) *OPMLService {
	ctx, cancel := context.WithCancel(context.Background())

	return &OPMLService{
		subs:    subs,
		folders: models.Folders,
		imports: models.OPMLImports,
		feeds:   models.Feeds,
		logger:  logger,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// SetSyncTrigger wires the scheduler in once it exists.
func (s *OPMLService) SetSyncTrigger(trigger SyncTrigger) {
	s.syncTrigger = trigger
}

// StartImport records a job for the given entries and starts working on it in
// the background, returning as soon as the job exists.
func (s *OPMLService) StartImport(ctx context.Context, userID uuid.UUID, entries []opml.Entry) (*data.OPMLImport, error) {
	imp := &data.OPMLImport{UserID: userID, Total: len(entries)}

	if err := s.imports.Insert(ctx, imp); err != nil {
		return nil, err
	}

	s.wg.Go(func() {
		s.run(imp.ID, userID, entries)
	})

	return imp, nil
}

// run subscribes to every entry, then hands the feeds to the scheduler.
func (s *OPMLService) run(importID, userID uuid.UUID, entries []opml.Entry) {
	folders := s.resolveFolders(userID, entries)

	work := make(chan opml.Entry)
	var mu sync.Mutex
	var feedIDs []int64

	var workers sync.WaitGroup
	for range opmlImportWorkers {
		workers.Go(func() {
			for entry := range work {
				feedID, result := s.importEntry(userID, entry, folders[entry.Folder])

				if s.ctx.Err() != nil {
					return
				}

				if err := s.imports.AppendResult(s.ctx, importID, result); err != nil {
					s.logger.PrintError(err, map[string]string{
						"context":   "recording an OPML import result",
						"import_id": importID.String(),
					})
				}

				if feedID != 0 {
					mu.Lock()
					feedIDs = append(feedIDs, feedID)
					mu.Unlock()
				}
			}
		})
	}

dispatch:
	for _, entry := range entries {
		select {
		case work <- entry:
		case <-s.ctx.Done():
			break dispatch
		}
	}
	close(work)
	workers.Wait()

	if s.ctx.Err() != nil {
		s.logger.PrintInfo("OPML import cancelled at shutdown", map[string]string{
			"import_id": importID.String(),
		})
		s.finish(importID, data.OPMLImportInterrupted)
		return
	}

	if err := s.feeds.MarkDueForSync(s.ctx, feedIDs); err != nil {
		s.logger.PrintError(err, map[string]string{
			"context":   "marking imported feeds due for sync",
			"import_id": importID.String(),
		})
	} else if s.syncTrigger != nil && len(feedIDs) > 0 {
		s.syncTrigger.TriggerSync()
	}

	s.finish(importID, data.OPMLImportCompleted)
}

// importEntry subscribes to one feed and turns the outcome into a report line.
// It returns the feed id on success, zero otherwise.
func (s *OPMLService) importEntry(userID uuid.UUID, entry opml.Entry, folderID *int64) (int64, data.OPMLImportResult) {
	result := data.OPMLImportResult{URL: entry.XMLURL, Title: entry.Title}

	sub, err := s.subs.Subscribe(s.ctx, userID, entry.XMLURL, SubscribeOptions{
		FolderID:    folderID,
		DeferImport: true,
	})

	switch {
	case err == nil:
		result.Status = data.OPMLEntryImported
		return sub.FeedID, result

	case errors.Is(err, ErrAlreadySubscribed):
		result.Status = data.OPMLEntrySkipped
		result.Reason = "already subscribed"

	case errors.Is(err, ErrInvalidFeed):
		result.Status = data.OPMLEntryFailed
		result.Reason = "not a valid RSS feed"

	case errors.Is(err, ErrFeedNotFound):
		result.Status = data.OPMLEntryFailed
		result.Reason = "could not be reached"

	default:
		result.Status = data.OPMLEntryFailed
		result.Reason = "import failed"
		if !cancelledByShutdown(s.ctx, err) {
			s.logger.PrintError(err, map[string]string{
				"context": "subscribing during an OPML import",
				"url":     entry.XMLURL,
			})
		}
	}

	return 0, result
}

// resolveFolders creates the file's folders up front, sequentially, and returns
// a lookup the workers can read without locking.
func (s *OPMLService) resolveFolders(userID uuid.UUID, entries []opml.Entry) map[string]*int64 {
	folders := map[string]*int64{}

	for _, entry := range entries {
		if entry.Folder == "" {
			continue
		}
		if _, done := folders[entry.Folder]; done {
			continue
		}

		folder, err := s.folders.GetOrCreate(s.ctx, userID, entry.Folder)
		if err != nil {
			s.logger.PrintError(err, map[string]string{
				"context": "creating a folder during an OPML import",
				"folder":  entry.Folder,
			})
			folders[entry.Folder] = nil
			continue
		}

		folders[entry.Folder] = &folder.ID
	}

	return folders
}

// finish closes the job on a context detached from cancellation: the write that
// records how an interrupted import ended must survive the cancellation that
// interrupted it.
func (s *OPMLService) finish(importID uuid.UUID, status string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ctx), finishTimeout)
	defer cancel()

	if err := s.imports.MarkFinished(ctx, importID, status); err != nil {
		s.logger.PrintError(err, map[string]string{
			"context":   "closing an OPML import",
			"import_id": importID.String(),
			"status":    status,
		})
	}
}

// Shutdown cancels the imports in flight and waits for them to record where
// they stopped.
func (s *OPMLService) Shutdown() {
	s.cancel()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(shutdownGrace):
		s.logger.PrintError(errors.New("timed out waiting for OPML imports to stop"), nil)
	}
}
