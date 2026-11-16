// Package service holds the business logic between the HTTP handlers and the
// data layer. Five concerns live here, and they are more independent than one
// package suggests:
//
//   - fetcher.go polls RSS/Atom, parses it (gofeed) and turns items into
//     articles, extracting page content through internal/readability;
//   - the anti-bot ladder for that extraction — pagefetch.go picks a transport,
//     challenge.go recognises an interstitial, solver.go drives the optional
//     browser sidecar, scrape.go decides when to escalate and how often;
//   - scheduler.go is the worker pool that runs the sync periodically;
//   - subscriptions.go owns the subscribe sequence;
//   - opml.go imports a subscription list as a background job.
//
// Only RSS polling and article scraping differ in their outbound client: polling
// always uses the stdlib one, scraping climbs the ladder. Both dial through
// internal/safedial, because both fetch URLs a user supplied.
//
// Background work (the Scheduler, OPMLService) holds a context of its own rather
// than a request's, since the work outlives the response; each exposes a way to
// cancel it (Stop, Shutdown).
package service

import (
	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/jsonlog"
)

// Services aggregates the business logic layer, the way data.Models aggregates
// the data layer: main builds one and hands it to the handlers.
type Services struct {
	FeedService         *FeedService
	SubscriptionService *SubscriptionService
	OPMLService         *OPMLService
	Scheduler           *Scheduler
}

// NewServices builds every service and the scheduler, injecting the models, the
// logger and their configuration. The scheduler is built but not started.
func NewServices(models data.Models, logger *jsonlog.Logger, fetchCfg FetchConfig, schedCfg SchedulerConfig) Services {
	// The order is a dependency chain, not a preference: the scheduler syncs
	// through FeedService, SubscriptionService creates feeds with FeedService and
	// wakes the scheduler, and OPMLService subscribes through
	// SubscriptionService.
	feedService := NewFeedService(models, logger, fetchCfg)
	scheduler := NewScheduler(feedService, models, logger, schedCfg)
	subscriptionService := NewSubscriptionService(models, feedService, scheduler, logger)
	opmlService := NewOPMLService(models, subscriptionService, logger)

	return Services{
		FeedService:         feedService,
		SubscriptionService: subscriptionService,
		OPMLService:         opmlService,
		Scheduler:           scheduler,
	}
}
