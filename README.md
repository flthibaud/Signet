# Signet

[![CI](https://github.com/flthibaud/Signet/actions/workflows/ci.yml/badge.svg)](https://github.com/flthibaud/Signet/actions/workflows/ci.yml)

Une application **read-it-later** self-hosted avec support natif des flux **RSS/Atom**, écrite en **Go** (API) et **React Router v7** (frontend).

## À propos

Ce projet est **inspiré d'[Omnivore](https://github.com/omnivore-app/omnivore)**, un excellent lecteur read-it-later open-source que j'utilisais au quotidien mais qui n'est **plus maintenu** depuis sa fermeture fin 2024 (suite au rachat par ElevenLabs).

L'objectif est de proposer une alternative légère, self-hostable, avec un seul binaire Go (frontend embarqué via `go:embed`) et une base PostgreSQL. L'accent est mis sur :

- **La simplicité de déploiement** : un binaire, une base de données, c'est tout
- **Le support RSS natif** : les flux RSS/Atom sont des citoyens de première classe
- **La déduplication** : un article sauvé par *N* utilisateurs n'est stocké qu'une seule fois
- **La lecture** : extraction du contenu via readability, sanitization, temps de lecture estimé

![feeds display on homepage](docs/resources/feeds-homepage.png "Feeds")

![feeds details](docs/resources/feed-details.png "Feeds Details")

## Fonctionnalités

- Inscription / connexion par email, avec token Bearer **ou** cookie httpOnly
- Abonnement à des flux RSS/Atom, import des articles dès l'abonnement
- **Synchronisation en tâche de fond** : pool de workers, toutes les 15 min par défaut (`SCHEDULER_INTERVAL`), avec ETag / `If-Modified-Since` et rate limit par domaine
- **Extraction du contenu** via readability puis conversion en markdown, avec repli sur la description RSS
- **Fetch anti-bot** : fingerprint TLS de navigateur par défaut, sidecar navigateur optionnel pour les challenges JS ([docs/ANTIBOT_FETCHING.md](docs/ANTIBOT_FETCHING.md))
- **Déduplication** : l'article est stocké une fois, `links` porte l'état par utilisateur
- État de lecture par utilisateur : lu, favori, archivé, progression, slug unique
- **Recherche full-text** multilingue et insensible aux accents, avec filtres par flux et par date
- Rate limiting par IP sur l'API (`/v1/*` uniquement, désactivé par défaut)

## Démarrage rapide

Le plus court chemin, tout en conteneurs :

```bash
cp .env.example .env
docker compose up -d
```

L'image applicative est tirée depuis GHCR (`ghcr.io/flthibaud/signet`), publiée par la CI : `latest` suit `master`, `dev` suit `develop`. Rien n'est compilé sur place, donc ce fichier fonctionne tel quel sur un NAS ou un VPS sans toolchain. Pour un déploiement durable, épingler un tag de version dans `SIGNET_TAG` plutôt que de suivre `latest` : tirer une image plus récente applique ses migrations au redémarrage suivant, et une migration ne se rejoue pas à l'envers.

L'application écoute sur <http://localhost:8000>. Le binaire embarque ses migrations SQL et met la base à niveau lui-même au démarrage : il n'y a pas d'étape de migration à lancer avant, ni de service dédié dans le compose. Une base vide suffit.

Le conteneur applicatif tourne sous un utilisateur non privilégié, avec un système de fichiers racine en lecture seule (seul `/tmp` est un tmpfs) : rien n'est écrit sur disque, tout l'état vit dans Postgres. Son healthcheck interroge `/v1/readiness`, qui échoue si la base est injoignable.

### Utiliser sa propre base PostgreSQL

Le service `db` du compose est là pour dépanner : identifiants fixes, aucun port publié, joignable par la seule app. Pour brancher une base existante — managée, ou déjà présente — il suffit de renseigner `DATABASE_URL` dans le `.env` et de supprimer le service `db` du compose, plus rien d'autre n'y fait référence :

```bash
DATABASE_URL="postgres://user:motdepasse@monhote:5432/signet?sslmode=require"
```

C'est le seul réglage de base de données : il n'y a pas de variables d'hôte, d'utilisateur ou de mot de passe séparées à accorder entre elles.

```bash
docker compose logs -f app
docker compose pull && docker compose up -d   # mise à jour
docker compose down          # ajouter -v pour supprimer aussi le volume Postgres
```

Pour faire tourner le compose sur une image construite depuis le working tree — tester une modif avant de la pousser — il suffit de la tagger sous le même nom :

```bash
docker build -t ghcr.io/flthibaud/signet:local .
SIGNET_TAG=local docker compose up -d
```

Le sidecar navigateur pour l'anti-bot est **optionnel** et derrière un profil, car il embarque un navigateur complet :

```bash
echo 'SOLVER_URL=http://solver:8191/v1' >> .env
docker compose --profile solver up -d
```

## Configuration

Toutes les variables ont un défaut sain : seule `DATABASE_URL` est obligatoire. Le binaire refuse de démarrer sur une valeur invalide plutôt que de retomber silencieusement sur zéro.

| Variable | Défaut | Description |
|---|---|---|
| `DATABASE_URL` | — | **Requis.** DSN PostgreSQL |
| `DATABASE_MAX_OPEN_CONNS` | `25` | Connexions ouvertes max (`0` = illimité) |
| `DATABASE_MAX_IDLE_CONNS` | `25` | Connexions inactives conservées |
| `DATABASE_MAX_IDLE_TIME` | `15m` | Durée avant fermeture d'une connexion inactive |
| `AUTO_MIGRATE` | `true` | Applique les migrations en attente au démarrage |
| `PORT` | `8000` | Port d'écoute HTTP |
| `ENV` | `` (`production` via compose) | Nom de l'environnement ; `production` active le flag `Secure` sur le cookie d'auth |
| `RATE_LIMITER_ENABLED` | `false` | Rate limiting par IP sur `/v1/*` |
| `RATE_LIMITER_RPS` | `5` | Requêtes par seconde et par IP |
| `RATE_LIMITER_BURST` | `10` | Taille du seau |
| `SCHEDULER_INTERVAL` | `15m` | Période de synchronisation des flux |
| `SCHEDULER_WORKERS` | `5` | Workers de synchronisation concurrents |
| `SCHEDULER_BATCH_SIZE` | `50` | Flux traités par tick |
| `TLS_IMPERSONATE_ENABLED` | `true` | Fingerprint TLS de navigateur pour le scraping d'articles |
| `SOLVER_URL` | `` | Sidecar navigateur (contrat FlareSolverr) ; vide = désactivé |
| `SOLVER_TIMEOUT` | `60s` | Budget d'un solve navigateur |
| `SOLVER_MAX_PER_FEED` | `5` | Plafond de solves par run de flux |

Le rate limiter est **désactivé par défaut** : un self-hoster servant quelques utilisateurs de confiance n'en a pas besoin. Il ne s'applique qu'à `/v1/*`.

## Développement

### Devcontainer

Le projet fournit un [devcontainer](.devcontainer/devcontainer.json) (VSCode / GitHub Codespaces) avec Go, Node.js/pnpm et PostgreSQL préconfigurés :

> *VSCode → `Dev Containers: Reopen in Container`*

### En local

Prérequis : Go 1.25+, Node.js 22+ avec pnpm, PostgreSQL 14+. Le CLI [golang-migrate](https://github.com/golang-migrate/migrate) n'est nécessaire que pour les cibles `make migrate-*` ci-dessous — créer une migration, faire un rollback, forcer une version. Le serveur, lui, applique les migrations tout seul.

```bash
go run ./cmd/api                      # migre puis sert l'API + le frontend embarqué, port 8000

cd frontend && pnpm install && pnpm dev   # dev server Vite, port 5173, HMR
```

En développement, travailler sur <http://localhost:5173> : le dev server Vite proxifie `/v1` vers l'API Go (`VITE_API_TARGET`, défaut `http://localhost:8000`). Le navigateur voit donc tout sur une même origine, ce qui fait fonctionner les cookies d'auth sans configuration CORS.

Le binaire Go sert le SPA **depuis le build embarqué** : pour voir des changements frontend sur le port 8000, il faut relancer `pnpm build` puis rebuilder le binaire.

```bash
make migrate-create name=add_x        # nouvelle paire up/down
make migrate-down                     # rollback de la dernière migration
make migrate-version                  # version actuelle
make migrate-force version=N          # sortir d'un état « Dirty database »
make reset-db                         # drop + re-run complet
go test ./...                         # tests Go
cd frontend && pnpm typecheck         # typegen react-router + tsc
```

Le binaire et le CLI partagent la même table `schema_migrations` : les deux peuvent être mélangés sans risque de désaccord sur la version en place. Si une migration casse en cours de route, la base est marquée *dirty* et tout démarrage suivant échoue avec la marche à suivre — corriger le schéma à la main, puis `make migrate-force version=N`.

### Build de production

```bash
cd frontend && pnpm build && cd ..    # obligatoire : alimente le go:embed
go build -o bin/api ./cmd/api
```

## Architecture

- **Go 1.25** + [`httprouter`](https://github.com/julienschmidt/httprouter), PostgreSQL via [`lib/pq`](https://github.com/lib/pq)
- [`gofeed`](https://github.com/mmcdole/gofeed) (RSS/Atom), [`go-readability`](https://codeberg.org/readeck/go-readability) (extraction), [`bluemonday`](https://github.com/microcosm-cc/bluemonday) (sanitization), [`tls-client`](https://github.com/bogdanfinn/tls-client) (fingerprint navigateur)
- **React 19** + **React Router v7** en **mode SPA** (`ssr: false`), **TailwindCSS v4**, **TanStack Query**, **react-hook-form** + **zod**
- Schéma PostgreSQL avec triggers et `citext`. La recherche full-text s'appuie sur un `tsvector` multilingue et insensible aux accents, maintenu par colonne générée et pondéré titre/description/contenu, exposé par `GET /v1/search`.

Détails dans [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Endpoints

| Méthode | Route | Auth | Description |
|---|---|---|---|
| `GET` | `/v1/healthcheck` | Non | Liveness — le process répond (ne touche aucune dépendance) |
| `GET` | `/v1/readiness` | Non | Readiness — `200` si la base est joignable, `503` sinon |
| `POST` | `/v1/users` | Non | Inscription |
| `POST` | `/v1/tokens/authentication` | Non | Connexion |
| `DELETE` | `/v1/tokens/authentication` | Oui | Déconnexion |
| `GET` | `/v1/users/me` | Oui | Utilisateur courant |
| `GET` | `/v1/subscriptions` | Oui | Liste des abonnements |
| `POST` | `/v1/subscriptions` | Oui | S'abonner à un flux |
| `DELETE` | `/v1/subscriptions/:id` | Oui | Se désabonner (conserve les articles) |
| `GET` | `/v1/links` | Oui | Articles, paginés — filtres `is_read`, `is_starred`, `archived`, `feed_id` |
| `GET` | `/v1/links/:slug` | Oui | Détail d'un article |
| `PATCH` | `/v1/links/:slug` | Oui | Mettre à jour l'état de lecture |
| `GET` | `/v1/search` | Oui | Recherche full-text — voir ci-dessous |

### Recherche full-text

`GET /v1/search` interroge la bibliothèque de l'utilisateur via l'index `tsvector`
d'`articles` (titre pondéré A, description B, contenu C).

| Paramètre | Description |
|---|---|
| `q` | Requête (syntaxe `websearch_to_tsquery` : `"phrase exacte"`, `-exclusion`, `or`). Le dernier terme est traité comme un préfixe, pour la recherche au fil de la frappe. Vide = les articles récents, triés par date de publication. Entre 2 et 200 caractères. |
| `lang` | Locale du chercheur (`fr-FR`, `en`…), pour le stemming de la requête. À défaut, l'en-tête `Accept-Language` ; à défaut, configuration neutre |
| `feed_id`, `is_read`, `is_starred` | Mêmes filtres que `/v1/links` |
| `archived` | Tri-état : absent = toute la bibliothèque (archivés inclus), contrairement à `/v1/links` |
| `since` | Borne inférieure RFC3339 sur la **date de publication** — le client la calcule dans son fuseau |
| `page`, `page_size` | Pagination (défaut 20, max 100) |

La réponse ne renvoie **pas** de total exact, mais un booléen `has_more`. Compter
tous les résultats obligerait Postgres à les parcourir entièrement pour produire
un nombre dont seule la présence d'une page suivante est utile.

Le tri par pertinence ne peut pas s'arrêter tôt : connaître les 20 meilleurs
impose de scorer tous les résultats. Le classement ne porte donc que sur les
**1000 correspondances publiées le plus récemment** (`searchRankCandidates`), bornage qui
rend le coût constant quelle que soit la taille de la bibliothèque. En
contrepartie, un article très pertinent mais plus ancien que ce seuil peut sortir
d'une recherche très large — et la pagination ne va pas au-delà de 1000
résultats.

Les dates affichées et filtrées sont celles de **publication**, jamais celles
d'import : un abonnement à un flux horodate tous les articles récupérés à la même
seconde, ce qui classerait un article vieux de trois semaines dans « aujourd'hui ».
`links.published_at` duplique la colonne d'`articles` pour que ce tri passe par un
index (migration `000010`).

Chaque résultat porte un `snippet` produit par `ts_headline`, où les termes trouvés
sont encadrés par `[[hl]]` / `[[/hl]]`. Ce sont des marqueurs texte, jamais du HTML :
le frontend les découpe pour rendre un `<mark>` sans injecter de balise.

### Multilingue et accents

Chaque article porte sa propre configuration de recherche (`articles.language`),
déduite du `<language>` du flux à l'import. Son `tsv` est **hybride** : le texte
est indexé deux fois, une fois avec la langue de l'article, une fois avec la
configuration neutre `simple_ua`. Côté requête, les deux moitiés sont interrogées
et combinées par `|`.

C'est ce qui évite d'avoir à deviner la langue de la requête : la moitié neutre
matche ce que l'utilisateur a littéralement tapé, quelle que soit la langue, et la
moitié stemmée ajoute la morphologie de **sa** locale. Un francophone qui cherche
« objets connectes » trouve ainsi un article contenant « objet connecté » — sans
`lang`, la même requête ne renvoie rien.

Toutes les configurations passent par `unaccent` (suffixe `_ua`), donc « hebergee »
trouve « hébergée » et « Zuge » trouve « Züge », dans les deux sens.

> **Limite connue — CJK.** Le parser de Postgres découpe sur les espaces : en
> japonais et en chinois, une phrase entière devient un seul lexème et la
> recherche partielle ne trouve rien. Ces langues retombent sur `simple_ua`, ce
> qui ne les répare pas. Les corriger demanderait des bigrammes applicatifs ou
> PGroonga (indisponible sur Postgres standard). Le coréen, l'arabe, le russe et
> toutes les langues à séparateurs fonctionnent normalement.

Les erreurs suivent une enveloppe unique : `{"error": ...}`, une chaîne pour les erreurs génériques, un objet `{champ: message}` pour les 422 de validation.

## Structure du projet

```
signet/
├── cmd/api/            # Handlers HTTP, middleware, routes
├── internal/
│   ├── data/           # Couche d'accès aux données (seule à faire du SQL)
│   ├── service/        # Fetcher RSS, scraping anti-bot, scheduler
│   ├── readability/    # Extraction de contenu → markdown
│   ├── validator/      # Validation des entrées
│   └── jsonlog/        # Logging JSON structuré
├── frontend/           # Application React Router (SPA)
├── migrations/         # Migrations SQL (golang-migrate)
├── docs/               # Documentation technique
├── frontend.go         # go:embed du build frontend
├── migrations.go       # go:embed des migrations SQL
├── docker-compose.yml  # app + PostgreSQL + sidecar optionnel
├── Dockerfile          # build multi-stages
└── Makefile            # helpers de migration
```

## Documentation

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — Architecture détaillée
- [docs/RSS_SYNC.md](docs/RSS_SYNC.md) — Flux de synchronisation RSS
- [docs/ANTIBOT_FETCHING.md](docs/ANTIBOT_FETCHING.md) — Fetch anti-bot (TLS imperso, sidecar navigateur)
- [docs/READABILITY_TESTING.md](docs/READABILITY_TESTING.md) — Tests du parser readability
- [docs/schema.sql](docs/schema.sql) — Schéma complet de la BDD

## Licence

Signet est distribué sous licence **[GNU AGPL-3.0-or-later](LICENSE)**.

Copyright (C) 2026 Florian Thibaud

Concrètement : vous êtes libre d'utiliser, modifier et redistribuer Signet, y
compris pour vos propres besoins ou ceux de votre organisation. En contrepartie,
si vous distribuez une version modifiée **ou si vous la proposez à des tiers via
un réseau** (hébergement, SaaS), vous devez en publier le code source sous la
même licence.

L'auto-hébergement pour soi, sa famille ou son équipe n'impose rien : cette
clause vise la mise à disposition d'un service à des utilisateurs tiers.

Le nom et le logo **Signet** ne sont pas couverts par la licence — voir
[NOTICE.md](NOTICE.md).
