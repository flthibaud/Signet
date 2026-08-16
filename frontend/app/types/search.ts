export interface SearchResult {
  id: number;
  slug: string;
  title: string;
  snippet: string;
  image_url?: string;
  feed_id: number | null;
  feed_title?: string;
  reading_time_minutes: number;
  is_read: boolean;
  is_starred: boolean;
  /** ISO timestamp, or null when the link is not archived. */
  archived_at: string | null;
  saved_at: string;
  published_at: string;
  rank: number;
}

export interface SearchResponse {
  results: SearchResult[];
  metadata: {
    query: string;
    current_page: number;
    page_size: number;
    /**
     * Whether a further page exists. The API deliberately does not return an
     * exact total: counting every match is the most expensive part of a broad
     * search, and this is the only part of it a caller needs.
     */
    has_more: boolean;
  };
}

export interface SearchResultGroup {
  id: string;
  title: string;
  date?: string;
  list: SearchResult[];
}

/** Must match `HighlightStart` / `HighlightEnd` in internal/data/search.go. */
export const HIGHLIGHT_START = "[[hl]]";
export const HIGHLIGHT_END = "[[/hl]]";
