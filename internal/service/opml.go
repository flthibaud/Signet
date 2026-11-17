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

// shutdownGrace bounds how long Shutdown waits for cancelled imports to return.
// It only has to cover the unwinding after cancellation, not the import itself.
const shutdownGrace = 5 * time.Second

// The interfaces below are sized to what the import actually calls, so the
// worker can be tested with fakes. data.FolderModel, data.OPMLImportModel and
// *SubscriptionService satisfy them as they are.

type folderStore interface {
	GetOrCreate(ctx context.Context, userID uuid.UUID, name string) (*data.Folder, error)
}

type importStore interface {
	Insert(ctx context.Context, imp *data.OPMLImport) error
	AppendResult(ctx context.Context, id uuid.UUID, result data.OPMLImportResult) error
	MarkFinished(ctx context.Context, id uuid.UUID, status string) error
}

type subscriber interface {
	Subscribe(ctx context.Context, userID uuid.UUID, feedURL string, opts SubscribeOptions) (*data.Subscription, error)
}

// OPMLService runs subscription-list imports.
type OPMLService struct {
	subs    subscriber
	folders folderStore
	imports importStore
	logger  *jsonlog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewOPMLService builds the service with a context of its own, since an import
// runs in the background well past the upload request that started it. Pair it
// with Shutdown.
func NewOPMLService(models data.Models, subs subscriber, logger *jsonlog.Logger) *OPMLService {
	ctx, cancel := context.WithCancel(context.Background())

	return &OPMLService{
		subs:    subs,
		folders: models.Folders,
		imports: models.OPMLImports,
		logger:  logger,
		ctx:     ctx,
		cancel:  cancel,
	}
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

// run subscribes to every entry. Each subscription wakes the scheduler, which
// starts syncing the first feeds while the later ones are still being added;
// its worker pool and per-domain limiter bound that, however long the file.
func (s *OPMLService) run(importID, userID uuid.UUID, entries []opml.Entry) {
	folders := s.resolveFolders(userID, entries)

	work := make(chan opml.Entry)

	var workers sync.WaitGroup
	for range opmlImportWorkers {
		workers.Go(func() {
			for entry := range work {
				result := s.importEntry(userID, entry, folders[entry.Folder])

				if s.ctx.Err() != nil {
					return
				}

				if err := s.imports.AppendResult(s.ctx, importID, result); err != nil {
					s.logger.PrintError(err, map[string]string{
						"context":   "recording an OPML import result",
						"import_id": importID.String(),
					})
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

	s.finish(importID, data.OPMLImportCompleted)
}

// importEntry subscribes to one feed and turns the outcome into a report line.
func (s *OPMLService) importEntry(userID uuid.UUID, entry opml.Entry, folderID *int64) data.OPMLImportResult {
	result := data.OPMLImportResult{URL: entry.XMLURL, Title: entry.Title}

	_, err := s.subs.Subscribe(s.ctx, userID, entry.XMLURL, SubscribeOptions{FolderID: folderID})

	switch {
	case err == nil:
		result.Status = data.OPMLEntryImported

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

	return result
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
