package service

import (
	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/jsonlog"
)

// Services regroupe toute la logique métier de ton application.
// Si demain tu ajoutes un service d'envoi d'email ou de statistiques, tu l'ajouteras ici.
type Services struct {
	FeedService *FeedService

	// models is kept for the housekeeping the scheduler does outside any one
	// service — pruning expired tokens, for instance.
	models data.Models
}

// NewServices initialise tous les services en leur injectant les modèles (accès
// DB), le logger et la configuration de fetch.
func NewServices(models data.Models, logger *jsonlog.Logger, fetchCfg FetchConfig) Services {
	return Services{
		FeedService: NewFeedService(models, logger, fetchCfg),
		models:      models,
	}
}
