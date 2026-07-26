import { useMemo, useState } from "react";
import { Search as SearchIcon, Rss, Clock, LoaderCircle } from "lucide-react";

import Select from "~/components/Select";
import Item from "./Item";
import { dateFilters, groupResults, type DateFilter } from "./grouping";
import useDebouncedValue from "~/hooks/useDebouncedValue";
import { MIN_SEARCH_LENGTH, useSearch } from "~/lib/search";
import { useSubscriptions } from "~/lib/feeds";

type SourceOption = {
  id: string;
  title: string;
  /** Undefined on the "All feeds" entry, which applies no filter. */
  feedId?: number;
};

const allSources: SourceOption = { id: "all", title: "All feeds" };

const Search = () => {
  const [search, setSearch] = useState<string>("");
  const [source, setSource] = useState<SourceOption>(allSources);
  const [date, setDate] = useState<DateFilter>(dateFilters[0]);

  // The query the API actually sees. An empty one is meaningful: it returns
  // the most recently saved articles, which is what the modal shows on open —
  // and it is also what a half-typed word falls back to, since the API rejects
  // anything shorter than MIN_SEARCH_LENGTH.
  const typed = useDebouncedValue(search.trim());
  const query = typed.length >= MIN_SEARCH_LENGTH ? typed : "";

  const { data: subscriptions } = useSubscriptions();
  const sources = useMemo<SourceOption[]>(
    () => [
      allSources,
      ...(subscriptions?.subscriptions ?? []).map((subscription) => ({
        id: String(subscription.feed.id),
        title: subscription.custom_title ?? subscription.feed.title,
        feedId: subscription.feed.id,
      })),
    ],
    [subscriptions],
  );

  // Resolved when the filter changes rather than on every render, so typing
  // doesn't churn the query key with a drifting timestamp.
  const since = useMemo(() => date.since(new Date()), [date]);

  const { data, isPending, isError, error } = useSearch({
    q: query,
    feedId: source.feedId,
    since,
    pageSize: 30,
  });

  const groups = useMemo(
    () => groupResults(data?.results ?? [], query),
    [data, query],
  );

  return (
    <form
      className="flex flex-col min-h-0 h-full"
      onSubmit={(e) => e.preventDefault()}
    >
      {/* Fixed header: search field */}
      <div className="shrink-0 relative border-b border-[#E8ECEF] dark:border-[#232627]">
        <button
          className="group absolute top-7 left-10 outline-none md:hidden"
          type="submit"
        >
          <SearchIcon className="w-8 h-8 fill-[#6C7275]/50 transition-colors group-hover:fill-[#141718] dark:group-hover:fill-[#E8ECEF]" />
        </button>
        <input
          className="w-full h-22 pl-24 pr-5 bg-transparent border-none outline-none h5 text-[#141718] placeholder:text-[#6C7275]/50 md:h-18 md:pl-18 dark:text-[#FEFEFE]"
          type="text"
          name="search"
          placeholder="Search"
          autoFocus
          autoComplete="off"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
      </div>
      {/* Fixed filters row */}
      <div className="shrink-0 pt-5 px-10 pb-5 md:px-6 md:pb-4">
        <div className="md:flex block space-y-4 md:space-y-0">
          <Select
            className="md:w-[10.31rem] md:mr-3 w-full mr-0"
            classButton="h-11 rounded-full shadow-[inset_0_0_0_0.0625rem_#DADBDC] caption1 dark:shadow-[inset_0_0_0_0.0625rem_#2A2E2F] dark:bg-transparent"
            classOptions="min-w-full"
            classIcon="w-5 h-5 text-[#6C7275]/50"
            classArrow="dark:text-[#6C7275]"
            icon={Rss}
            placeholder="Source"
            items={sources}
            value={source}
            onChange={setSource}
          />
          <Select
            className="md:w-[10.31rem] w-full mr-0"
            classButton="h-11 rounded-full shadow-[inset_0_0_0_0.0625rem_#DADBDC] caption1 dark:shadow-[inset_0_0_0_0.0625rem_#2A2E2F] dark:bg-transparent"
            classOptions="min-w-full"
            classIcon="w-5 h-5 text-[#6C7275]/50"
            classArrow="dark:text-[#6C7275]"
            icon={Clock}
            placeholder="Date"
            items={dateFilters}
            value={date}
            onChange={setDate}
          />
        </div>
      </div>
      {/* Scrollable results: only this region scrolls */}
      <div className="grow min-h-0 overflow-y-auto px-10 pb-6 md:px-6 scrollbar-none">
        {isPending ? (
          <div className="flex justify-center py-10 text-[#6C7275]/50">
            <LoaderCircle className="w-6 h-6 animate-spin" />
          </div>
        ) : isError ? (
          <div className="py-10 text-center caption1 text-[#6C7275]">
            {error instanceof Error
              ? error.message
              : "Search is unavailable right now."}
          </div>
        ) : groups.length === 0 ? (
          <div className="py-10 text-center caption1 text-[#6C7275]">
            {query
              ? `No articles match “${query}”.`
              : typed
                ? `Keep typing — searches need at least ${MIN_SEARCH_LENGTH} characters.`
                : "Nothing saved yet. Subscribe to a feed to get started."}
          </div>
        ) : (
          groups.map((group) => <Item item={group} key={group.id} />)
        )}
      </div>
    </form>
  );
};

export default Search;
