package readability

// Unit tests for preprocessing rules.
//
// Each test case uses a minimal HTML snippet to verify that a specific cleanup
// rule produces the expected result. These tests serve as living documentation:
// they describe *what* each rule removes and *why*.
//
// To add a new rule:
//  1. Add a case here describing the intended behavior (name + input + contain/exclude).
//  2. Implement the rule in preprocess.go.
//  3. Run the tests to confirm the rule works in isolation before adding full-page
//     golden files under test/test-pages/.

import (
	"strings"
	"testing"
)

type preprocessCase struct {
	name    string
	input   string
	contain []string // strings that MUST appear in the markdown output
	exclude []string // strings that MUST NOT appear in the markdown output
}

// preprocessCases documents every active preprocessing rule.
// Cases are grouped by site or feature area.
var preprocessCases = []preprocessCase{

	// -------------------------------------------------------------------------
	// General — lazy images
	// -------------------------------------------------------------------------
	{
		name: "remplace data-src par src sur les images lazy-load",
		input: `<div>
			<img data-src="https://example.com/photo.jpg" src="data:image/svg+xml,%3Csvg/%3E" alt="photo"/>
		</div>`,
		contain: []string{"photo.jpg"},
	},

	// -------------------------------------------------------------------------
	// General — éléments de navigation/accessibilité
	// -------------------------------------------------------------------------
	{
		name: "retire les éléments tabindex=-1 (skip links, overlays cachés)",
		input: `<div>
			<p>Contenu visible.</p>
			<div tabindex="-1"><p>Navigation cachée</p></div>
		</div>`,
		contain: []string{"Contenu visible."},
		exclude: []string{"Navigation cachée"},
	},

	// -------------------------------------------------------------------------
	// General — blocs RGPD / tracking
	// -------------------------------------------------------------------------
	{
		name: "retire les éléments data-vendor (bandeaux RGPD)",
		input: `<div>
			<p>Contenu de l'article.</p>
			<div data-vendor="didomi"><p>Gérer mes préférences</p></div>
		</div>`,
		contain: []string{"Contenu de l'article."},
		exclude: []string{"Gérer mes préférences"},
	},
	{
		name: "retire les éléments data-tracking",
		input: `<div>
			<p>Contenu de l'article.</p>
			<div data-tracking="newsletter"><p>Abonnez-vous</p></div>
		</div>`,
		contain: []string{"Contenu de l'article."},
		exclude: []string{"Abonnez-vous"},
	},

	// -------------------------------------------------------------------------
	// General — images dupliquées dans <picture>
	// -------------------------------------------------------------------------
	{
		name: "retire les balises <picture> (wrappers lazy-load avec img dupliqué)",
		input: `<div>
			<picture>
				<source srcset="photo.webp" type="image/webp"/>
			</picture>
			<img src="photo.jpg" alt="photo"/>
		</div>`,
		contain: []string{"photo.jpg"},
		exclude: []string{"photo.webp"},
	},

	// -------------------------------------------------------------------------
	// General — widgets de contenu lié
	// -------------------------------------------------------------------------
	{
		name: `retire les blocs "à lire aussi" et variantes`,
		input: `<div>
			<p>Contenu de l'article.</p>
			<div><p>À lire aussi</p><a href="/autre-article">Un autre article</a></div>
		</div>`,
		contain: []string{"Contenu de l'article."},
		exclude: []string{"À lire aussi", "Un autre article"},
	},
	{
		name: `retire les blocs "pour aller plus loin"`,
		input: `<div>
			<p>Contenu de l'article.</p>
			<div><p>Pour aller plus loin</p><a href="/guide">Le guide complet</a></div>
		</div>`,
		contain: []string{"Contenu de l'article."},
		exclude: []string{"Pour aller plus loin", "Le guide complet"},
	},

	// -------------------------------------------------------------------------
	// Numerama
	// -------------------------------------------------------------------------
	{
		name: "numerama: retire div#installPwaDiv (invitation à installer l'appli)",
		input: `<div>
			<p>Contenu de l'article.</p>
			<div id="installPwaDiv" data-nosnippet="">
				<p>Ajoutez Numerama à votre écran d'accueil</p>
			</div>
		</div>`,
		contain: []string{"Contenu de l'article."},
		exclude: []string{"Ajoutez Numerama"},
	},
	{
		name: "numerama: retire div#download (widget de téléchargement d'application)",
		input: `<div>
			<p>Contenu de l'article.</p>
			<div id="download">
				<h2>Revolut</h2>
				<p>Télécharger gratuitement</p>
			</div>
		</div>`,
		contain: []string{"Contenu de l'article."},
		exclude: []string{"Revolut", "Télécharger gratuitement"},
	},
	{
		name: "numerama: retire <hr> + section embedded-tag-* et tout ce qui suit",
		// Le <hr> précède toujours les tags thématiques en bas d'article.
		// On coupe à partir du <hr> pour ne garder que le texte de l'article.
		input: `<div>
			<p>Contenu de l'article.</p>
			<hr/>
			<div id="embedded-tag-jeux-video">
				<p>Tous nos articles sur les jeux vidéo</p>
			</div>
		</div>`,
		contain: []string{"Contenu de l'article."},
		exclude: []string{"Tous nos articles", "jeux-video"},
	},

	// -------------------------------------------------------------------------
	// Le Monde
	// -------------------------------------------------------------------------
	{
		name: "le monde: retire la modale de partage par lien cadeau",
		input: `<div>
			<p>Contenu de l'article.</p>
			<div id="js-modal-gifted-url"><p>Partager cet article</p></div>
		</div>`,
		contain: []string{"Contenu de l'article."},
		exclude: []string{"Partager cet article"},
	},
	{
		name: "le monde: retire le bandeau de plafonnement paywall (js-capping)",
		input: `<div>
			<p>Contenu de l'article.</p>
			<section id="js-capping"><p>Il vous reste X articles gratuits</p></section>
		</div>`,
		contain: []string{"Contenu de l'article."},
		exclude: []string{"articles gratuits"},
	},
}

func TestPreprocessRules(t *testing.T) {
	r := NewReadability()

	for _, tc := range preprocessCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := r.HTMLToMarkdown(tc.input)
			if err != nil {
				t.Fatalf("HTMLToMarkdown error: %v", err)
			}

			for _, want := range tc.contain {
				if !strings.Contains(got, want) {
					t.Errorf("manquant dans la sortie: %q\n\nSortie:\n%s", want, got)
				}
			}
			for _, notWant := range tc.exclude {
				if strings.Contains(got, notWant) {
					t.Errorf("présent dans la sortie (devrait être absent): %q\n\nSortie:\n%s", notWant, got)
				}
			}
		})
	}
}
