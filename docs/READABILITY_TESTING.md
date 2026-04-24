# Tests du service Readability

Ce document décrit le système de tests du package `internal/readability`, qui convertit du HTML en Markdown. Il s'adresse à quelqu'un qui découvre le projet et veut ajouter un site, corriger une règle, ou comprendre pourquoi un test échoue.

---

## Vue d'ensemble

Le service dispose de deux niveaux de tests complémentaires :

| Niveau | Fichier | Ce qu'il teste |
|---|---|---|
| **Règles unitaires** | `preprocess_test.go` | Chaque règle de nettoyage DOM en isolation, avec un snippet HTML minimal |
| **Intégration (golden files)** | `readability_test.go` | Pipeline complet HTML → Markdown sur de vraies pages capturées |

La règle générale : **on écrit d'abord un test unitaire pour documenter le comportement attendu d'une règle, puis on ajoute éventuellement une page complète pour la régression globale.**

---

## Tests unitaires — règles de preprocessing

### Fichier : `preprocess_test.go`

Chaque règle de nettoyage DOM est documentée par un cas de test minimal. La structure est simple :

```go
{
    name: "numerama: retire div#download (widget de téléchargement d'application)",
    input: `<div>
        <p>Contenu de l'article.</p>
        <div id="download">
            <h2>Revolut</h2>
            <p>Télécharger gratuitement</p>
        </div>
    </div>`,
    contain: []string{"Contenu de l'article."},   // doit être présent dans la sortie
    exclude: []string{"Revolut", "Télécharger"},  // ne doit pas apparaître
},
```

Les cas sont regroupés par périmètre dans `preprocessCases` :
- **General** — lazy images, RGPD, accessibilité, contenu lié
- **Numerama** — PWA install, widget download, section embedded-tag
- **Le Monde** — modale cadeau, bandeau paywall

### Lancer uniquement ces tests

```bash
go test ./internal/readability/... -run TestPreprocessRules -v
```

### Ajouter une règle de preprocessing

1. **Écrire le cas de test en premier** dans `preprocessCases` (`preprocess_test.go`) :
   - Choisir un `name` explicite (périmètre + élément + raison)
   - Écrire le snippet HTML **minimal** qui déclenche la règle
   - Renseigner `contain` (contenu article préservé) et `exclude` (éléments à retirer)

2. **Vérifier que le test échoue** : `go test -run TestPreprocessRules`

3. **Implémenter la règle** dans `preprocess.go` :
   - Si c'est un sélecteur simple (ID, attribut, tag) → ajouter une entrée dans `removalRules`
   - Si c'est une logique positionnelle (ex. couper à partir d'un `<hr>`) → ajouter une fonction dédiée appelée dans `preprocessDOM`

4. **Vérifier que le test passe** : `go test -run TestPreprocessRules`

---

## Tests d'intégration — golden files

### Principe

Chaque site a un dossier sous `internal/readability/test/test-pages/<site>/` contenant :

```
test-pages/
└── numerama/
    ├── url.txt                  # URL source (pour référence)
    ├── source.html              # page capturée (fragment readability)
    ├── expected.md              # sortie Markdown attendue (golden file)
    ├── source-variant1.html     # variante de page (layout différent)
    ├── expected-variant1.md     # golden file correspondant
    └── ...
```

Le test lit chaque `source*.html`, applique le pipeline complet, et compare avec le `expected*.md` correspondant. Une divergence = échec avec affichage de la première ligne différente.

### Nommage des variantes

| Fichier source | Fichier expected | Nom du sous-test |
|---|---|---|
| `source.html` | `expected.md` | `default` |
| `source-variant1.html` | `expected-variant1.md` | `variant1` |
| `source-variant2.html` | `expected-variant2.md` | `variant2` |

### Lancer les tests d'intégration

```bash
# Tous les sites
go test ./internal/readability/... -run TestHTMLToMarkdown -v

# Un seul site
go test ./internal/readability/... -run TestHTMLToMarkdown/numerama -v

# Une variante précise
go test ./internal/readability/... -run TestHTMLToMarkdown/numerama/variant2 -v
```

### Ajouter un site

1. Créer le dossier : `internal/readability/test/test-pages/<site>/`
2. Y déposer `url.txt` (l'URL) et `source.html` (le HTML du fragment article, extrait via l'outil de capture)
3. Générer le golden file :
   ```bash
   go test ./internal/readability/... -run TestHTMLToMarkdown/<site> -update
   ```
4. **Relire `expected.md`** pour vérifier que la sortie est propre (pas de blocs pub, pas de nav, pas de PWA install…)
5. Committer `source.html` et `expected.md` ensemble

### Ajouter une variante à un site existant

Même démarche : déposer `source-variantN.html`, puis :

```bash
go test ./internal/readability/... -run TestHTMLToMarkdown/<site>/variantN -update
```

### Mettre à jour les goldens après un changement de règle

Quand une règle de preprocessing change et que la sortie évolue **intentionnellement** :

```bash
go test ./internal/readability/... -update
```

Cela régénère **tous** les `expected*.md`. Relire le diff git avant de committer pour s'assurer que les changements sont bien voulus.

### Déboguer un échec

En cas d'échec, le test écrit automatiquement la sortie réelle dans un fichier `_actual.md` à côté du golden file :

```
expected-variant2.md       ← référence
expected-variant2_actual.md ← sortie réelle lors de l'échec
```

Comparer les deux avec un diff :

```bash
diff internal/readability/test/test-pages/numerama/expected-variant2.md \
     internal/readability/test/test-pages/numerama/expected-variant2_actual.md
```

---

## Fichiers clés

| Fichier | Rôle |
|---|---|
| `preprocess.go` | Règles de nettoyage DOM (suppression d'éléments, fix lazy images, etc.) |
| `preprocess_test.go` | Tests unitaires de chaque règle, avec snippets HTML minimaux |
| `readability_test.go` | Tests d'intégration golden files, avec flag `-update` |
| `test/test-pages/<site>/` | Pages capturées + goldens par site |

---

## Décisions de conception

**Pourquoi deux niveaux de tests ?**
Les tests unitaires documentent chaque règle et se lisent comme des specs. Les golden files détectent les régressions sur de vraies pages avec tous leurs cas limites. Les deux se complètent.

**Pourquoi des snippets HTML minimaux ?**
Un snippet de 10 lignes est plus lisible qu'une page de 1000 lignes pour comprendre ce qu'une règle fait. Il se maintient aussi indépendamment des changements de structure du site source.

**Pourquoi des variantes plutôt qu'un seul fichier par site ?**
Numerama, comme beaucoup de sites, a plusieurs mises en page selon le type d'article (article standard, fiche application, critique de série…). Les variantes permettent de couvrir ces layouts différents sans dupliquer les règles.

**Pourquoi un flag `-update` plutôt qu'un script séparé ?**
Le flag est intégré au workflow `go test` standard et évite de maintenir un outil externe. Il régénère exactement ce que le test compare, sans risque de désynchronisation.
