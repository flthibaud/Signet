package data

import (
	"database/sql"
)

// Models is the data layer's single entry point: one field per entity, each a
// model wrapping the same *sql.DB. The application struct holds one of these,
// so a handler reaches any table through app.models.
type Models struct {
	Users         UserModel
	Feeds         FeedModel
	Subscriptions SubscriptionModel
	Folders       FolderModel
	Links         LinkModel
	Articles      ArticleModel
	Tokens        TokenModel
	OPMLImports   OPMLImportModel
}

// NewModels returns the set of models, all sharing db and therefore its
// connection pool.
func NewModels(db *sql.DB) Models {
	return Models{
		Users:         UserModel{DB: db},
		Feeds:         FeedModel{DB: db},
		Subscriptions: SubscriptionModel{DB: db},
		Folders:       FolderModel{DB: db},
		Links:         LinkModel{DB: db},
		Articles:      ArticleModel{DB: db},
		Tokens:        TokenModel{DB: db},
		OPMLImports:   OPMLImportModel{DB: db},
	}
}
