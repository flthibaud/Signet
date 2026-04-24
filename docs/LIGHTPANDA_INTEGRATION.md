# Intégration Lightpanda — Fetcher avec fallback headless browser

## Contexte

Le fetcher actuel effectue un simple HTTP GET avec un User-Agent Chrome pour récupérer le contenu des articles. Cette approche fonctionne pour la plupart des sites, mais certains sites (ex: Le Monde) renvoient désormais une page de **challenge JavaScript** qui nécessite l'exécution de JS côté client pour charger le vrai contenu.

**Symptôme observé :**
- Titre retourné : `"Client Challenge"`
- Contenu : une page d'erreur indiquant que JS est requis

La solution est d'intégrer [Lightpanda](https://lightpanda.io) comme fallback headless browser léger.

---

## Architecture cible

```
fetchWithReadability(url)
  │
  ├── HTTP GET simple (approche actuelle, rapide)
  │     │
  │     ├── Succès → readability → markdown ✓
  │     │
  │     └── Challenge détecté (titre = "Client Challenge", "Just a moment...", etc.)
  │               │
  │               └── fetchWithBrowser(url)
  │                     │
  │                     ├── CDP via chromedp → RemoteAllocator → Lightpanda
  │                     ├── Navigation + attente du rendu JS
  │                     └── HTML rendu → readability → markdown ✓
  │
  └── Fallback final : contenu RSS de l'item
```

---

## Pourquoi Lightpanda ?

| Critère | Chromium headless | Lightpanda |
|---|---|---|
| Mémoire | ~300-500 MB | ~20-30 MB (16x moins) |
| Vitesse de démarrage | ~2-3s | <200ms (9x plus rapide) |
| Exécution JS | Oui | Oui |
| Interface | CDP (WebSocket) | CDP (WebSocket) — compatible |
| Dépendance binaire | Chrome/Chromium | Lightpanda (binaire Zig) |

---

## Implémentation

### Prérequis

Lightpanda doit tourner comme processus séparé (sidecar) :

```bash
# Démarrage du serveur CDP
lightpanda serve --host 127.0.0.1 --port 9222
```

En production, gérer le processus via Docker ou un service système. En développement, lancer manuellement ou via `docker-compose`.

### Dépendance Go

Ajouter `chromedp` au projet :

```bash
go get github.com/chromedp/chromedp
```

Utiliser `RemoteAllocator` pour pointer vers Lightpanda au lieu de Chrome :

```go
allocCtx, cancel := chromedp.NewRemoteAllocator(
    context.Background(),
    "ws://127.0.0.1:9222",
)
defer cancel()
```

### Contrainte : 1 connexion par processus

Lightpanda ne supporte qu'**une seule connexion CDP active à la fois** (1 contexte, 1 page). Le fetcher utilisant un worker pool concurrent, les requêtes headless doivent être sérialisées via un mutex ou une queue.

```go
// Exemple : mutex global pour les requêtes Lightpanda
var lightpandaMu sync.Mutex

func (s *FeedService) fetchWithBrowser(url string) (*readabilityResult, error) {
    lightpandaMu.Lock()
    defer lightpandaMu.Unlock()
    // ... requête CDP
}
```

### Logique de détection des challenges

Dans `fetchWithReadability()`, les titres suivants déclenchent le fallback browser :

```go
challengeTitles := []string{
    "Just a moment...",  // Cloudflare IUAM
    "Client Challenge",  // Le Monde / autres
}
```

---

## Configuration

Variables d'environnement à ajouter :

| Variable | Défaut | Description |
|---|---|---|
| `LIGHTPANDA_URL` | `ws://127.0.0.1:9222` | URL CDP du serveur Lightpanda |
| `LIGHTPANDA_ENABLED` | `false` | Active le fallback headless |

Si `LIGHTPANDA_ENABLED=false`, le fallback se rabat sur le contenu RSS de l'item (comportement actuel).

---

## Flux complet avec Lightpanda activé

1. Scheduler déclenche `ImportArticlesForSubscribers()`
2. Pour chaque item RSS → `fetchWithReadability(articleURL)`
3. HTTP GET → si challenge détecté → `fetchWithBrowser(articleURL)`
4. `fetchWithBrowser` : connecte à Lightpanda via CDP, navigue vers l'URL, attend le rendu JS, récupère le HTML du DOM
5. Le HTML rendu passe dans le pipeline readability habituel → markdown
6. Si Lightpanda échoue ou est désactivé → fallback contenu RSS

---

## Références

- [Lightpanda GitHub](https://github.com/lightpanda-io/browser)
- [Demo Go avec chromedp + Lightpanda](https://github.com/lightpanda-io/demo)
- [chromedp RemoteAllocator](https://pkg.go.dev/github.com/chromedp/chromedp#NewRemoteAllocator)
