# Charge les variables d'environnement depuis le fichier .env s'il existe
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

# Variables par défaut
MIGRATION_PATH=./migrations

.PHONY: migrate-up migrate-down migrate-create

# Commande pour migrer UP
migrate-up:
	migrate -path=$(MIGRATION_PATH) -database="$(DATABASE_URI)" up

# Commande pour migrer DOWN (annuler la dernière)
migrate-down:
	migrate -path=$(MIGRATION_PATH) -database="$(DATABASE_URI)" down 1

# Helper pour créer une nouvelle migration
# Utilisation : make migrate-create name=add_users_table
migrate-create:
	migrate create -ext sql -dir $(MIGRATION_PATH) -seq $(name)

# Commande pour forcer une version (en cas d'erreur "Dirty database")
# Utilisation : make migrate-force version=1
migrate-force:
	migrate -path=$(MIGRATION_PATH) -database="$(DATABASE_URI)" force $(version)

# Commande pour afficher la version actuelle de la base de données
migrate-version:
	migrate -path=$(MIGRATION_PATH) -database="$(DATABASE_URI)" version

# Commande pour aller à une version spécifique de la base de données
# Utilisation : make migrate-goto version=2
migrate-goto:
	migrate -path=$(MIGRATION_PATH) -database="$(DATABASE_URI)" goto $(version)

# Commande pour Supprimer TOUTES les tables et redémarrer les migrations
reset-db:
	migrate -path=$(MIGRATION_PATH) -database="$(DATABASE_URI)" drop -f
	migrate -path=$(MIGRATION_PATH) -database="$(DATABASE_URI)" up