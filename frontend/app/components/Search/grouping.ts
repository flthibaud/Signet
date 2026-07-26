import type { SearchResult, SearchResultGroup } from "~/types/search";

const DAY_MS = 24 * 60 * 60 * 1000;

/** Midnight on the day `date` falls on, in the viewer's timezone. */
function startOfDay(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate());
}

/**
 * The "Date" filter's options. `since` is resolved client-side because only the
 * browser knows the user's timezone, and "today" is a timezone-dependent
 * boundary; the API takes an absolute RFC3339 instant.
 *
 * It bounds the article's publication date, not when the link was created: an
 * RSS import stamps everything it pulls with the same saved_at, so a fresh
 * subscription would file a three-week-old article under "Today".
 */
export type DateFilter = {
  id: string;
  title: string;
  since: (now: Date) => string | undefined;
};

export const dateFilters: DateFilter[] = [
  { id: "any", title: "Any time", since: () => undefined },
  { id: "today", title: "Today", since: (now) => startOfDay(now).toISOString() },
  {
    id: "week",
    title: "Last week",
    since: (now) => new Date(startOfDay(now).getTime() - 7 * DAY_MS).toISOString(),
  },
  {
    id: "month",
    title: "Last 30 days",
    since: (now) => new Date(startOfDay(now).getTime() - 30 * DAY_MS).toISOString(),
  },
];

const relative = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });
const shortDate = new Intl.DateTimeFormat(undefined, {
  weekday: "short",
  day: "numeric",
  month: "short",
});

/** "3 minutes ago" / "2 days ago", from an RFC3339 timestamp. */
export function formatRelativeDate(iso: string, now: Date = new Date()): string {
  const elapsed = now.getTime() - new Date(iso).getTime();
  if (Number.isNaN(elapsed)) return "";

  const units: [Intl.RelativeTimeFormatUnit, number][] = [
    ["year", 365 * DAY_MS],
    ["month", 30 * DAY_MS],
    ["day", DAY_MS],
    ["hour", 60 * 60 * 1000],
    ["minute", 60 * 1000],
  ];

  for (const [unit, ms] of units) {
    if (elapsed >= ms) return relative.format(-Math.floor(elapsed / ms), unit);
  }
  return "just now";
}

/**
 * Date buckets, ordered newest first. `maxAgeDays` is inclusive and the
 * previous entry's value is the exclusive lower bound, so they tile the whole
 * range without gaps.
 */
const buckets = [
  { id: "today", title: "Today", maxAgeDays: 0 },
  { id: "week", title: "Last 7 days", maxAgeDays: 7 },
  { id: "month", title: "Last 30 days", maxAgeDays: 30 },
  { id: "older", title: "Earlier", maxAgeDays: Infinity },
];

/**
 * Turns a flat result list into the groups the UI renders.
 *
 * With a query the API orders by relevance, so date buckets would interleave
 * meaninglessly — everything goes into one group instead. Without a query the
 * API orders by recency, which is exactly what the buckets describe.
 */
export function groupResults(
  results: SearchResult[],
  query: string,
  now: Date = new Date(),
): SearchResultGroup[] {
  if (results.length === 0) return [];

  if (query) {
    return [
      {
        id: "results",
        title: "Results",
        date: `${results.length} ${results.length === 1 ? "article" : "articles"}`,
        list: results,
      },
    ];
  }

  const todayStart = startOfDay(now).getTime();

  // Whole-day difference, so an article published late yesterday lands in
  // "Last 7 days" rather than "Today". Buckets follow publication date to match
  // both the date filter and the timestamp each row displays.
  const ageInDays = (result: SearchResult) =>
    Math.round(
      (todayStart - startOfDay(new Date(result.published_at)).getTime()) / DAY_MS,
    );

  return buckets
    .map((bucket, index) => {
      const minAgeDays = index === 0 ? -Infinity : buckets[index - 1].maxAgeDays;
      return {
        id: bucket.id,
        title: bucket.title,
        date: bucket.id === "today" ? shortDate.format(now) : undefined,
        list: results.filter((result) => {
          const age = ageInDays(result);
          return age > minAgeDays && age <= bucket.maxAgeDays;
        }),
      };
    })
    .filter((group) => group.list.length > 0);
}
