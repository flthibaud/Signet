import { keepPreviousData, useQuery } from "@tanstack/react-query";

import { apiFetch } from "./api";
import type { SearchResponse } from "~/types/search";

export const MIN_SEARCH_LENGTH = 2;

export type SearchParams = {
  /** Empty means "no full-text restriction": the API returns recent saves. */
  q: string;
  feedId?: number;
  since?: string;
  pageSize?: number;
};

function queryLanguage(): string | undefined {
  if (typeof navigator === "undefined") return undefined;
  return navigator.language || undefined;
}

function buildSearchQuery({ q, feedId, since, pageSize }: SearchParams) {
  const params = new URLSearchParams();
  if (q) params.set("q", q);
  if (feedId !== undefined) params.set("feed_id", String(feedId));
  if (since) params.set("since", since);
  if (pageSize !== undefined) params.set("page_size", String(pageSize));

  const lang = queryLanguage();
  if (q && lang) params.set("lang", lang);

  return params.toString();
}

/**
 * Full-text search across the current user's library.
 *
 * Results are kept while a new query is in flight so the list doesn't blank out
 * between keystrokes.
 */
export function useSearch(params: SearchParams) {
  const queryString = buildSearchQuery(params);

  return useQuery({
    queryKey: ["search", queryString],
    queryFn: () => apiFetch<SearchResponse>(`/v1/search?${queryString}`),
    placeholderData: keepPreviousData,
    staleTime: 30_000,
  });
}
