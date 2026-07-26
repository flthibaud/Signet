# Fetch anti-bot — deux chemins + fallback navigateur

## Contexte

Deux types de fetch cohabitent, avec des besoins opposés :

- **Polling RSS** (`ImportArticlesForSubscribers`) : fetche les *flux* toutes les
  15 min, tous les feeds, avec ETag/If-Modified-Since. Fréquent, rarement bloqué
  → doit rester rock-solid sur le client **stdlib**. On n'y touche pas.
- **Scraping d'article** (`fetchWithReadability`) : extrait le contenu de la page.
  C'est *lui* qui se prend les anti-bot (Cloudflare, DataDome…), qui :
  - **filtrent passivement sur le fingerprint TLS (JA3/JA4)** → un client Go
    standard est reconnu comme non-navigateur et bloqué (403), même avec un bon
    User-Agent ;
  - ou renvoient un **challenge JavaScript** qu'il faut exécuter côté client.

**Symptômes observés :** statut `403` avec `Server: cloudflare`, ou titre
retourné `"Just a moment..."` / `"Client Challenge"`.

La bonne structure n'est donc **pas une cascade GET→retry** (double fetch inutile :
le surcoût d'un handshake impersoné est marginal vs une seconde requête complète),
mais **deux chemins de fetch distincts** + un seul vrai fallback navigateur.

---

## Architecture

```
Polling RSS  ──────────────────────────────► client stdlib (inchangé)

Scraping article · fetchWithReadability(url)
  │
  ├─ client TLS-impersonating (in-process)                → défaut du scraping
  │     tls-client, fingerprint TLS + HTTP/2 de Chrome
  │     (retombe sur stdlib si désactivé ou si le transport casse)
  │     │
  │     └─ bloqué / challenge détecté
  │           │
  ├─ retry stdlib (1 requête, gratuit)                    → WAF anti-imposteur
  │     certains sites bloquent le fingerprint Chrome mais servent un client nu
  │     │
  │     └─ toujours bloqué : challenge JS actif
  │           │
  ├─ sidecar navigateur anti-détect                       → challenge JS actif
  │     Byparr (Camoufox) via le contrat REST FlareSolverr
  │
  └─ Fallback final : contenu RSS de l'item (comportement actuel)
```

Chaque brique est **désactivable par config**. Sans sidecar déployé et avec
l'impersonation coupée, le scraping repart sur la stdlib puis se rabat sur le
contenu RSS — le comportement d'origine.

Le code correspondant :

| Fichier | Rôle |
|---|---|
| `internal/service/pagefetch.go` | `pageFetcher` + les transports stdlib et TLS imperso |
| `internal/service/challenge.go` | détection de challenge (statut, headers, marqueurs HTML) |
| `internal/service/solver.go` | client REST du sidecar + sérialisation + circuit breaker |
| `internal/service/scrape.go` | l'échelle d'escalade (`fetchPage`), budget de solves, mémoire par domaine |

---

## Client TLS-impersonating (défaut du scraping, in-process)

Règle l'essentiel des blocages Cloudflare (filtrage passif du fingerprint TLS)
**sans sidecar**, on reste sur le principe 1-binaire. C'est le client **par
défaut de `fetchWithReadability`** : le surcoût du handshake est marginal et sans
effet de bord sur les sites normaux, donc inutile de faire un GET standard
d'abord.

Dépendance : [`github.com/bogdanfinn/tls-client`](https://github.com/bogdanfinn/tls-client)
(profil `Chrome_146`), qui couvre à la fois le fingerprint TLS et les *settings
frames* HTTP/2.

### Le JA3 ne suffit pas : headers et cohérence

Trois pièges, tous traités dans `pagefetch.go` :

1. **L'ordre des headers fait partie du fingerprint.** Cloudflare le regarde à
   côté du JA3. Une requête ne portant qu'un `User-Agent` est signalée quel que
   soit le handshake → on envoie le jeu complet d'un Chrome en navigation
   top-level (`sec-ch-ua*`, `sec-fetch-*`, `accept`, `accept-language`,
   `upgrade-insecure-requests`), dans l'ordre de Chrome, via `HeaderOrderKey`.
2. **UA et fingerprint doivent décrire le même navigateur.** Annoncer Chrome/120
   avec un handshake Chrome 146 est *en soi* un signal. `browserProfile`,
   `browserUserAgent` et `browserSecChUA` se bumpent ensemble — et la constante
   `UserAgent` du polling RSS pointe sur la même valeur.
3. **La décompression dépend du protocole.** On pose `accept-encoding` à la main
   (le retirer serait un tell, et il faut y mettre `zstd` puisque Chrome
   l'annonce). Or le chemin HTTP/2 de fhttp décompresse **mais laisse
   `Content-Encoding` en place**, tandis que le chemin HTTP/1 le supprime :
   décompresser inconditionnellement double-décode et casse avec
   `gzip: invalid header` / `brotli: RESERVED`. On se fie donc à
   `resp.Uncompressed`, seul signal fiable.

### Garder la stdlib atteignable

tls-client remplace le transport stdlib (lib tierce sur un chemin critique). Trois
garde-fous :

- `TLS_IMPERSONATE_ENABLED=false` repasse tout le scraping sur la stdlib ;
- sur **erreur de transport** (pas sur un statut HTTP), `fetchPage` retente une
  fois avec la stdlib plutôt que de perdre l'article ;
- sur **blocage**, idem — voir juste en dessous.

Le polling RSS, lui, reste **toujours** sur la stdlib.

### L'impersonation peut empirer les choses

Contre-intuitif mais mesuré : certains WAF matchent **l'ordre de headers exact de
Chrome** et le bloquent — « un bot qui se fait passer pour un navigateur » — tout
en servant sans broncher un client qui n'en a pas l'air.

Cas reproductible sur `zonebourse.com` : 403 avec l'ordre Chrome, 200 dès que
`accept-encoding` n'est plus collé juste avant `accept-language`. Ni la valeur des
headers ni la langue n'entrent en jeu, seulement la position — c'est bien la
signature Chrome qui est visée.

On ne tord donc **pas** l'ordre des headers pour contourner un site : ça
dégraderait le fingerprint partout ailleurs. La réponse est structurelle — quand
le fetch imperso est bloqué, `fetchPage` retente une fois avec la stdlib, *avant*
d'envisager le sidecar :

- c'est une requête, contre plusieurs secondes de navigateur ;
- ça ne consomme pas de budget de solve ;
- si ça passe, la marque « host protégé » est effacée, donc les articles suivants
  de ce site repartent sur le chemin normal.

Pour un vrai site Cloudflare, la stdlib sera bloquée aussi et on escalade au
palier suivant : le coût est une requête perdue, uniquement sur le chemin déjà en
échec.

### Exception : `CreateFromURL`

L'ajout d'un feed par l'utilisateur est une action *interactive* : un 403
Cloudflare y est un échec visible. Le fetch reste stdlib, mais sur 403/503/429 on
retente une fois avec le client imperso (`retryFeedFetch`). Le double fetch est
acceptable ici — c'est rare et manuel, contrairement au poll de 15 min.

---

## Sidecar navigateur anti-détect — seul vrai fallback

Uniquement pour les sites qui exigent l'**exécution d'un challenge JS**, quand le
client TLS imperso ne suffit pas. Un headless « léger » type Lightpanda ne
convient pas : ces moteurs sont optimisés pour la vitesse, pas pour tromper un
anti-bot, et se font détecter.

### Quelle implémentation

On code contre le **contrat REST de FlareSolverr** (`POST /v1` avec un `cmd`),
devenu un standard de fait, pas contre une implémentation :

```
POST http://127.0.0.1:8191/v1
{ "cmd": "request.get", "url": "<article>", "maxTimeout": 60000 }
→ { "status": "ok", "solution": { "response": "<html rendu>" } }
```

- **Byparr** — recommandé. Adossé à **Camoufox** (Firefox patché
  anti-fingerprinting), activement maintenu, même port et même API que
  FlareSolverr.
- **FlareSolverr** — l'original, mais adossé à undetected-chromedriver et non à
  Camoufox, sans logique pour produire un `cf-turnstile-response` : il ne passe
  plus les Managed Challenges modernes, et le projet n'est plus vraiment
  maintenu. À éviter pour un nouveau déploiement.
- **FlareBypasser** — autre implémentation du même contrat.

Changer de sidecar = changer `SOLVER_URL`, aucun code à toucher.

### Sidecar

C'est un navigateur complet (~centaines de Mo) : à réserver au strict
nécessaire, d'où sa position en dernier palier. `docker-compose.solver.yml`
fournit le service, **bindé sur loopback** — cet endpoint fetche des URL
arbitraires sur demande et ne doit jamais être exposé.

```bash
docker compose -f docker-compose.solver.yml up -d
```

### Concurrence et garde-fous

Le fetcher tourne en pool de workers, le sidecar pilote **un** navigateur. Trois
protections dans `solver.go` / `scrape.go` :

- **Sérialisation** : sémaphore à 1 solve à la fois. Envoyer tout le pool sur le
  sidecar ne fait que produire des timeouts.
- **Circuit breaker** : 3 échecs consécutifs → on arrête d'appeler le sidecar
  pendant 5 min. Sinon un sidecar mort transforme chaque article en timeout. Une
  annulation de contexte (notre propre deadline) ne compte pas comme un échec.
- **Budget par run de feed** (`SOLVER_MAX_PER_FEED`, défaut 5). C'est le point
  important : `feedProcessTimeout` borne **tout** le feed à 8 min, et un solve
  peut prendre une minute. Sans plafond, une poignée d'articles protégés affame
  les items restants — et comme un item raté empêche l'ETag d'avancer, le tick
  suivant refait tout, en boucle. Budget épuisé → on retombe direct sur le
  contenu RSS.
- Le rate-limit par domaine existant (`scrapeLimiter`, 1 req/s) s'applique avant
  toute l'échelle.

### Mémoire par domaine

Un host qui a servi un challenge est mémorisé 1 h (`challengedHosts`) : ses
autres articles vont **directement** au sidecar au lieu de brûler un fetch
imperso condamné à échouer. Un fetch propre sur ce host efface la marque, donc un
site qui lève sa protection se récupère tout seul.

### Cookies et sessions (non implémenté)

La réponse du sidecar contient aussi `solution.cookies` (dont `cf_clearance`) et
`solution.userAgent`. Tentant, mais un `cf_clearance` est lié à l'**IP** *et* au
fingerprint TLS/UA de celui qui l'a obtenu : le rejouer depuis le client Go
marche mal en pratique. La vraie optimisation serait les **sessions**
(`sessions.create` puis `session: <id>`), qui gardent l'onglet chaud et
raccourcissent nettement les solves suivants. À faire si le volume le justifie.

---

## Détection du challenge

Point de branchement : `detectChallenge()`, appelé par `fetchPage` sur la
réponse, avant readability.

Ce qui manquait avant : **le scraping ne regardait jamais le statut HTTP**. Le
body d'un 403 partait tel quel dans readability, et le challenge ne se
manifestait qu'après coup, sous forme d'un article intitulé « Just a moment... »
— d'où la détection par titre planquée dans `createArticleFromItem`. Elle est
maintenant centralisée, et un challenge non résolu devient une **erreur**, ce qui
déclenche naturellement le fallback RSS.

Trois familles de signaux, par ordre de fiabilité décroissante :

1. **Headers** — `cf-mitigated`, ou un statut bloquant (403/503/429) avec
   `Server: cloudflare` / `cf-ray`, `x-datadome`, cookies `datadome` /
   `incap_ses` / `visid_incap`.
2. **Marqueurs de runtime dans le HTML** — `/cdn-cgi/challenge-platform/`,
   `__cf_chl`, `cf-browser-verification`, `challenges.cloudflare.com/turnstile`,
   `geo.captcha-delivery.com`, `_Incapsula_Resource`. Ce sont des chemins
   d'assets : ils ne peuvent pas apparaître dans de la prose éditoriale, donc ils
   sont concluants même sur un 200 (Cloudflare sert certains interstitiels en
   200).
3. **Titres** — `just a moment`, `un instant`, `client challenge`, `attention
   required!`, `checking your browser`, `vérification`, `access denied`… Ces
   titres sont **localisés** et changent souvent : cette liste ne sera jamais
   exhaustive, elle n'est là qu'en filet. Elle n'est retenue que sur un statut
   bloquant ou une page < 100 Ko, pour ne pas signaler un article qui *cite* la
   phrase.

Enfin, un statut bloquant sans marqueur identifiable reste un blocage : on
escalade quand même, c'est tout ce qui reste à essayer.

---

## Configuration

Variables d'environnement (voir `.env.example`) :

| Variable | Défaut | Description |
|---|---|---|
| `TLS_IMPERSONATE_ENABLED` | `true` | Client TLS imperso pour le scraping (sinon stdlib) |
| `SOLVER_URL` | `` | Endpoint REST du sidecar ; vide = escalade navigateur désactivée |
| `SOLVER_TIMEOUT` | `60s` | Timeout d'un solve navigateur (`maxTimeout` envoyé au sidecar) |
| `SOLVER_MAX_PER_FEED` | `5` | Plafond de solves navigateur par run de feed |

Le polling RSS n'est jamais concerné : toujours stdlib.

---

## Flux complet

1. Scheduler → `ImportArticlesForSubscribers()` : polling du flux en **stdlib**,
   et ouverture d'un budget de solves pour le run.
2. Pour chaque nouvel item → `fetchWithReadability(url)` → rate-limit par domaine
   → `fetchPage()`.
3. Host déjà connu comme protégé + sidecar dispo → sidecar directement.
4. Sinon fetch avec le client **TLS imperso** (stdlib si désactivé ; retry stdlib
   sur erreur de transport).
5. `detectChallenge()` sur la réponse. Si challenge : retry **stdlib** (gratuit,
   hors budget) — s'il passe, la marque host est effacée et on repart.
6. Toujours bloqué + `SOLVER_URL` défini + budget disponible → escalade sidecar,
   HTML rendu.
7. HTML final → pipeline readability → markdown.
8. Tout échoue (challenge non résolu, statut d'erreur, options désactivées) →
   fallback contenu RSS de l'item.

---

## Observabilité

`logFetch` trace les événements intéressants — escalade vers le sidecar, solve
réussi ou raté, retry stdlib, budget épuisé, feed récupéré par le client imperso
— avec l'URL, le host et le signal détecté. Le chemin nominal reste silencieux
pour ne pas noyer le log. C'est ce qui permet de répondre à « est-ce que le
sidecar sert vraiment ? » avant de décider de le garder.

---

## Références

- [Byparr](https://github.com/ThePhaseless/Byparr) — sidecar Camoufox, API FlareSolverr
- [Camoufox](https://github.com/daijro/camoufox) — Firefox anti-détection
- [FlareSolverr](https://github.com/FlareSolverr/FlareSolverr) — l'original, contrat REST de référence
- [tls-client](https://github.com/bogdanfinn/tls-client) — impersonation TLS/HTTP2 en Go
- [utls](https://github.com/refraction-networking/utls) — TLS fingerprints custom (dépendance sous-jacente)
