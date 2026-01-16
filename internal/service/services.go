package service

import (
	"github.com/flthibaud/omnivore-go/internal/data"
)

// Services regroupe toute la logique métier de ton application.
// Si demain tu ajoutes un service d'envoi d'email ou de statistiques, tu l'ajouteras ici.
type Services struct {
	FeedImporter *FeedImporter
}

// NewServices initialise tous les services en leur injectant les modèles (accès DB).
func NewServices(models data.Models) Services {
	return Services{
		FeedImporter: NewFeedImporter(models),
	}
}
