package data

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// searchRankCandidates caps how many matching links get scored by a relevance
// search. Ranking is O(matches) — there is no way to know the top 20 without
// scoring everything — so on a large library a broad query would otherwise walk
// tens of thousands of rows to return one page. Bounding the candidates to the
// most recently published matches keeps the cost flat; the cost is that a very
// relevant but older article can fall outside a very broad search.
//
// It also caps how deep pagination can go, which is why it is well above any
// page size a caller can ask for.
const searchRankCandidates = 1000

// headlineOptions configures ts_headline. The markers are deliberately not
// HTML: the API returns the snippet as data and the frontend splits on them to
// render the highlight, so no caller ever has to inject raw markup.
const (
	HighlightStart = "[[hl]]"
	HighlightEnd   = "[[/hl]]"

	headlineOptions = "StartSel=" + HighlightStart + ", StopSel=" + HighlightEnd +
		", MaxWords=32, MinWords=12, MaxFragments=1, FragmentDelimiter= … "
)

type SearchResult struct {
	ID          int64      `json:"id"`
	Slug        string     `json:"slug"`
	Title       string     `json:"title"`
	Snippet     string     `json:"snippet"`
	ImageURL    string     `json:"image_url,omitempty"`
	FeedID      *int64     `json:"feed_id"`
	FeedTitle   *string    `json:"feed_title,omitempty"`
	ReadingTime float64    `json:"reading_time_minutes"`
	IsRead      bool       `json:"is_read"`
	IsStarred   bool       `json:"is_starred"`
	ArchivedAt  *time.Time `json:"archived_at"`
	SavedAt     time.Time  `json:"saved_at"`
	PublishedAt time.Time  `json:"published_at"`
	Rank        float64    `json:"rank"`
}

// SearchFilters describes a library search. An empty Query is valid and means
// "no full-text restriction", which turns the search into a plain reverse
// chronological listing — what the UI shows before the user types anything.
type SearchFilters struct {
	LinkFilters
	Query string
	// Language is the text search configuration to stem the *query* with,
	// derived from the user's locale. Empty falls back to the neutral config.
	// Use ResolveTextSearchConfig to produce it — the value is interpolated
	// into SQL, so it must come from that fixed set.
	Language string
	// Since bounds results by publication date, not by when the link was
	// created. An RSS import stamps every article it pulls with the same
	// saved_at, so filtering on that would put a three-week-old article in
	// "today" for anyone who just subscribed.
	Since *time.Time
}

// tsqueryExpr builds the SQL expression turning the $n placeholder into a
// tsquery under a single text search configuration.
//
// The ':*' suffix appended to websearch_to_tsquery's text output turns the last
// lexeme into a prefix match, so search-as-you-type finds "postgres" while the
// user is still typing "postg". websearch_to_tsquery renders as a quoted lexeme
// list ('foo' & 'bar'), so appending the marker only ever affects the trailing
// term. A query made solely of stopwords renders as the empty string, which is
// not a valid tsquery; it collapses to NULL instead, and NULL never matches.
// config is interpolated into the query text rather than bound, so it is run
// through safeTextSearchConfig here — at the point of interpolation — instead of
// being trusted from the caller.
func tsqueryExpr(n int, config string) string {
	parsed := fmt.Sprintf("websearch_to_tsquery('%s', $%d)", safeTextSearchConfig(config), n)
	return fmt.Sprintf(
		"(CASE WHEN %s::text = '' THEN NULL::tsquery ELSE (%s::text || ':*')::tsquery END)",
		parsed, parsed,
	)
}

// searchQueryExpr builds the tsquery matched against articles.tsv.
//
// Articles are indexed twice — once under their own language, once under the
// neutral config — so the query is parsed both ways and OR'd. The neutral half
// matches whatever the user literally typed, in any language; the stemmed half
// adds the morphology of *their* locale, which is what lets a French user
// searching "chats" find a document that only contains "chat".
//
// The COALESCE matters: a term that is a stopword in one config renders as an
// empty tsquery there, and `tsquery || NULL` is NULL — without it, searching
// "le" in French would wipe out the neutral half that would have matched.
func searchQueryExpr(n int, language string) string {
	neutral := tsqueryExpr(n, SimpleTextSearchConfig)

	// Resolved up front so an unrecognized value collapses to the neutral half
	// here, rather than producing a redundant `neutral || neutral`.
	language = safeTextSearchConfig(language)
	if language == SimpleTextSearchConfig {
		return neutral
	}

	stemmed := tsqueryExpr(n, language)
	return fmt.Sprintf("COALESCE(%s || %s, %s, %s)", stemmed, neutral, stemmed, neutral)
}

// Search returns the user's links matching filters, ranked by relevance when a
// query is present and by recency otherwise.
//
// The bool reports whether further results exist beyond this page. It replaces
// an exact total on purpose: a count(*) OVER() has to walk every match just to
// produce a number, which is the single most expensive part of a broad search,
// and the only thing any caller needs is whether to offer a next page.
func (m LinkModel) Search(ctx context.Context, userID uuid.UUID, filters SearchFilters, limit, offset int) ([]*SearchResult, bool, error) {
	where, args := buildLinkFiltersWhere(userID, filters.LinkFilters)

	if filters.Since != nil {
		args = append(args, *filters.Since)
		where = append(where, fmt.Sprintf("l.published_at >= $%d", len(args)))
	}

	var query string

	if filters.Query == "" {
		// Nothing to rank or highlight, so order by publication date — the same
		// order the article river uses, and the one the UI labels. It reads
		// links.published_at rather than the article's own column so the sort
		// comes straight off idx_links_user_published and PostgreSQL can stop as
		// soon as it has a page.
		query = fmt.Sprintf(`
			SELECT l.id, l.slug, l.feed_id, l.is_read, l.is_starred, l.archived_at,
				l.saved_at,
				a.title,
				COALESCE(NULLIF(a.image_url, ''), f.image_url, '') AS image_url,
				a.reading_time_minutes,
				l.published_at,
				f.original_title,
				0::float8 AS rank,
				COALESCE(a.description, '') AS snippet
			FROM links l
			JOIN articles a ON l.article_id = a.id
			LEFT JOIN feeds f ON l.feed_id = f.id
			WHERE %s
			ORDER BY l.published_at DESC, l.id DESC
			LIMIT $%d OFFSET $%d`,
			strings.Join(where, " AND "), len(args)+1, len(args)+2)
	} else {
		args = append(args, filters.Query)
		tsq := searchQueryExpr(len(args), filters.Language)
		where = append(where, fmt.Sprintf("a.tsv @@ %s", tsq))

		// Relevance ordering cannot stop early: scoring the top N means scoring
		// every match. The CTE bounds that work by taking the
		// searchRankCandidates most recently published matches and ranking only
		// those. The trade-off is that a highly relevant but older match drops
		// out of a very broad search on a very large library.
		//
		// MATERIALIZED is required: PostgreSQL would otherwise inline this
		// single-reference CTE and hoist the outer ORDER BY back over the whole
		// match set, undoing the bound.
		//
		// Normalization 32 divides the score by (rank + 1), keeping it in [0,1)
		// so it stays comparable across documents of different lengths.
		//
		// Highlighting uses the *article's* config, not the searcher's:
		// ts_headline re-tokenizes the document, so it has to stem it the same
		// way the tsvector did or the matched lexemes won't line up. The headline
		// covers description + body, so a match in either gets marked, and a
		// title-only match still falls back to the description — the same preview
		// the unsearched list shows.
		query = fmt.Sprintf(`
			WITH candidates AS MATERIALIZED (
				SELECT l.id, l.article_id, l.slug, l.feed_id, l.is_read, l.is_starred,
					l.archived_at, l.saved_at, l.published_at
				FROM links l
				JOIN articles a ON l.article_id = a.id
				WHERE %s
				ORDER BY l.published_at DESC, l.id DESC
				LIMIT %d
			)
			SELECT c.id, c.slug, c.feed_id, c.is_read, c.is_starred, c.archived_at,
				c.saved_at,
				a.title,
				COALESCE(NULLIF(a.image_url, ''), f.image_url, '') AS image_url,
				a.reading_time_minutes,
				c.published_at,
				f.original_title,
				ts_rank_cd(a.tsv, %s, 32) AS rank,
				ts_headline(a.language, concat_ws(' ', NULLIF(a.description, ''), a.text_content), %s, '%s') AS snippet
			FROM candidates c
			JOIN articles a ON c.article_id = a.id
			LEFT JOIN feeds f ON c.feed_id = f.id
			ORDER BY rank DESC, c.published_at DESC, c.id DESC
			LIMIT $%d OFFSET $%d`,
			strings.Join(where, " AND "), searchRankCandidates,
			tsq, tsq, headlineOptions, len(args)+1, len(args)+2)
	}

	// One row beyond the page tells us whether a next page exists, without
	// counting anything.
	args = append(args, limit+1, offset)

	rows, err := m.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	results := []*SearchResult{}

	for rows.Next() {
		var r SearchResult
		err := rows.Scan(
			&r.ID,
			&r.Slug,
			&r.FeedID,
			&r.IsRead,
			&r.IsStarred,
			&r.ArchivedAt,
			&r.SavedAt,
			&r.Title,
			&r.ImageURL,
			&r.ReadingTime,
			&r.PublishedAt,
			&r.FeedTitle,
			&r.Rank,
			&r.Snippet,
		)
		if err != nil {
			return nil, false, err
		}
		results = append(results, &r)
	}

	if err = rows.Err(); err != nil {
		return nil, false, err
	}

	hasMore := len(results) > limit
	if hasMore {
		results = results[:limit]
	}

	return results, hasMore, nil
}
