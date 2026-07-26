package data

import "strings"

const SimpleTextSearchConfig = "simple_ua"

// textSearchConfigs maps a BCP-47 primary subtag to the unaccent-enabled
// PostgreSQL text search configuration created by migration 000009.
//
// Only languages PostgreSQL ships a snowball stemmer for appear here. Anything
// else — notably Japanese, Chinese and Thai, which the default parser cannot
// tokenize at all because they have no word separators — resolves to
// SimpleTextSearchConfig.
var textSearchConfigs = map[string]string{
	"ar": "arabic_ua",
	"hy": "armenian_ua",
	"eu": "basque_ua",
	"ca": "catalan_ua",
	"da": "danish_ua",
	"nl": "dutch_ua",
	"en": "english_ua",
	"fi": "finnish_ua",
	"fr": "french_ua",
	"de": "german_ua",
	"el": "greek_ua",
	"hi": "hindi_ua",
	"hu": "hungarian_ua",
	"id": "indonesian_ua",
	"ga": "irish_ua",
	"it": "italian_ua",
	"lt": "lithuanian_ua",
	"ne": "nepali_ua",
	"no": "norwegian_ua",
	"nb": "norwegian_ua",
	"nn": "norwegian_ua",
	"pt": "portuguese_ua",
	"ro": "romanian_ua",
	"ru": "russian_ua",
	"sr": "serbian_ua",
	"es": "spanish_ua",
	"sv": "swedish_ua",
	"ta": "tamil_ua",
	"tr": "turkish_ua",
	"yi": "yiddish_ua",
}

// ResolveTextSearchConfig maps a language tag ("fr", "fr-FR", "en_US") to a
// PostgreSQL text search configuration name, falling back to
// SimpleTextSearchConfig for anything unknown or empty.
//
// The result is always one of a fixed set of identifiers, never caller-supplied
// text, which is what makes it safe to interpolate into a query — and it has to
// be interpolated rather than bound, so the planner can constant-fold the
// tsquery and still reach the GIN index.
func ResolveTextSearchConfig(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return SimpleTextSearchConfig
	}

	// "fr-FR", "fr_FR" and "fr" all reduce to "fr".
	if i := strings.IndexAny(tag, "-_"); i > 0 {
		tag = tag[:i]
	}

	if config, ok := textSearchConfigs[strings.ToLower(tag)]; ok {
		return config
	}
	return SimpleTextSearchConfig
}
