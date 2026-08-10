package data

import (
	"database/sql"
)

// Create a Models struct which wraps the ArticleModel. We'll add other models to this,
// like a UserModel and PermissionModel, as our build progresses.
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

// For ease of use, we also add a New() method which returns a Models struct containing
// the initialized Models.
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
