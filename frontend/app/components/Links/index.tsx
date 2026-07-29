import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import {
  Archive,
  ArchiveRestore,
  CheckCheck,
  Clock,
  Loader2,
  Rss,
  Star,
  Undo2,
  X,
} from "lucide-react";
import type { LinksResponse } from "~/types/link";
import { formatDate } from "~/utils/formatDate";
import { Link, useSearchParams } from "react-router";
import { apiFetch } from "~/lib/api";
import { useSubscriptions } from "~/lib/feeds";
import { useUpdateLink } from "~/lib/links";

const FILTERS = [
  { id: "all", label: "All", params: "" },
  { id: "unread", label: "Unread", params: "&is_read=false" },
  { id: "starred", label: "Starred", params: "&is_starred=true" },
  { id: "archived", label: "Archived", params: "&archived=true" },
] as const;

type FilterId = (typeof FILTERS)[number]["id"];

// The list scrolls inside its own container, so the browser's native scroll
// restoration can't help when navigating to an article and back. Position and
// active filter are kept in sessionStorage and restored on mount.
const SCROLL_KEY = "links:scroll";
const FILTER_KEY = "links:filter";

function savedFilter(): FilterId {
  const saved = sessionStorage.getItem(FILTER_KEY);
  return FILTERS.some((f) => f.id === saved) ? (saved as FilterId) : "all";
}

type Scope = {
  key: string;
  params: string;
  feedId: number | null;
  folderId: number | null;
};

function readScope(searchParams: URLSearchParams): Scope {
  const feedId = Number(searchParams.get("feed_id")) || null;
  const folderId = Number(searchParams.get("folder_id")) || null;

  if (feedId) {
    return { key: `feed:${feedId}`, params: `&feed_id=${feedId}`, feedId, folderId: null };
  }
  if (folderId) {
    return { key: `folder:${folderId}`, params: `&folder_id=${folderId}`, feedId: null, folderId };
  }
  return { key: "all", params: "", feedId: null, folderId: null };
}

function useScopeLabel(scope: Scope): string | null {
  const { data } = useSubscriptions({ enabled: scope.key !== "all" });

  if (scope.key === "all") return null;

  const subscriptions = data?.subscriptions ?? [];

  if (scope.feedId !== null) {
    const match = subscriptions.find((s) => s.feed.id === scope.feedId);
    return match?.custom_title || match?.feed.title || "This feed";
  }

  const match = subscriptions.find((s) => s.folder?.id === scope.folderId);
  return match?.folder?.name ?? "This folder";
}

export const Links = () => {
  const loaderRef = useRef<HTMLDivElement>(null);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const scrollPosRef = useRef(0);
  const scrollRestoredRef = useRef<string | null>(null);
  const updateLink = useUpdateLink();

  const [filter, setFilter] = useState<FilterId>(savedFilter);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [isBulkPending, setIsBulkPending] = useState(false);

  const filterParams = FILTERS.find((f) => f.id === filter)?.params ?? "";
  const showingArchived = filter === "archived";

  const [searchParams] = useSearchParams();
  const scope = readScope(searchParams);
  const scopeLabel = useScopeLabel(scope);
  const scrollKey = `${SCROLL_KEY}:${scope.key}`;

  const { data, fetchNextPage, hasNextPage, isFetchingNextPage } =
    useInfiniteQuery<LinksResponse>({
      queryKey: ["links", filter, scope.key],
      queryFn: ({ pageParam }) =>
        apiFetch<LinksResponse>(
          `/v1/links?page=${pageParam}${filterParams}${scope.params}`,
        ),
      initialPageParam: 1,
      getNextPageParam: (lastPage) =>
        lastPage.metadata.has_more
          ? lastPage.metadata.current_page + 1
          : undefined,
    });

  const links = data?.pages.flatMap((p) => p.links ?? []) ?? [];

  const allSelected = links.length > 0 && selected.size === links.length;
  const someSelected = selected.size > 0 && !allSelected;

  const changeFilter = (id: FilterId) => {
    setFilter(id);
    setSelected(new Set());
    sessionStorage.setItem(FILTER_KEY, id);
    // A new filter means a new list: start back at the top.
    scrollPosRef.current = 0;
    sessionStorage.removeItem(scrollKey);
    scrollContainerRef.current?.scrollTo({ top: 0 });
  };

  useEffect(() => {
    const el = scrollContainerRef.current;
    if (!el) return;

    const onScroll = () => {
      scrollPosRef.current = el.scrollTop;
    };

    el.addEventListener("scroll", onScroll, { passive: true });
    return () => {
      el.removeEventListener("scroll", onScroll);
      sessionStorage.setItem(scrollKey, String(scrollPosRef.current));
    };
  }, [scrollKey]);

  useLayoutEffect(() => {
    const el = scrollContainerRef.current;
    if (!el || scrollRestoredRef.current === scrollKey || links.length === 0) {
      return;
    }
    scrollRestoredRef.current = scrollKey;

    const saved = Number(sessionStorage.getItem(scrollKey) ?? 0);
    el.scrollTo({ top: saved, behavior: "instant" });
    scrollPosRef.current = el.scrollTop;
  }, [links.length, scrollKey]);

  const toggleSelected = (slug: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(slug)) {
        next.delete(slug);
      } else {
        next.add(slug);
      }
      return next;
    });
  };

  const toggleSelectAll = () => {
    setSelected(allSelected ? new Set() : new Set(links.map((l) => l.slug)));
  };

  const applyBulk = async (patch: {
    isRead?: boolean;
    archived?: boolean;
    readingProgress?: number;
  }) => {
    setIsBulkPending(true);
    try {
      await Promise.all(
        [...selected].map((slug) => updateLink.mutateAsync({ slug, ...patch })),
      );
      setSelected(new Set());
    } finally {
      setIsBulkPending(false);
    }
  };

  useEffect(() => {
    if (!loaderRef.current) return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasNextPage && !isFetchingNextPage) {
          fetchNextPage();
        }
      },
      { rootMargin: "200px" },
    );

    observer.observe(loaderRef.current);
    return () => observer.disconnect();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const bulkButtonClass =
    "flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-xs font-medium text-gray-600 dark:text-gray-300 transition-colors hover:bg-gray-100 dark:hover:bg-gray-700 disabled:opacity-50";

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between gap-4 px-10 py-4 border-b border-gray-200 dark:border-gray-700 shrink-0 md:px-4">
        <div className="flex items-center gap-4 min-w-0">
          <input
            type="checkbox"
            title="Select all"
            className="w-4 h-4 shrink-0 rounded border-gray-300 dark:border-gray-600 accent-blue-600 cursor-pointer"
            checked={allSelected}
            ref={(el) => {
              if (el) el.indeterminate = someSelected;
            }}
            onChange={toggleSelectAll}
          />

          <div className="flex items-center gap-1 overflow-x-auto">
            {FILTERS.map((f) => (
              <button
                key={f.id}
                type="button"
                onClick={() => changeFilter(f.id)}
                className={`px-3 py-1.5 rounded-full text-xs font-medium whitespace-nowrap transition-colors ${
                  filter === f.id
                    ? "bg-gray-900 text-white dark:bg-white dark:text-gray-900"
                    : "text-gray-500 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700"
                }`}
              >
                {f.label}
              </button>
            ))}
          </div>
        </div>

        {scopeLabel && selected.size === 0 && (
          <Link
            to="/app"
            title="Show every feed"
            className="flex items-center gap-1.5 shrink-0 px-3 py-1.5 rounded-full text-xs font-medium bg-primary-1/10 text-primary-1 transition-colors hover:bg-primary-1/20"
          >
            <span className="max-w-40 truncate">{scopeLabel}</span>
            <X size={14} />
          </Link>
        )}

        {selected.size > 0 && (
          <div className="flex items-center gap-1 shrink-0">
            <span className="mr-2 text-xs text-gray-400 dark:text-gray-500 whitespace-nowrap">
              {selected.size} selected
            </span>
            <button
              type="button"
              className={bulkButtonClass}
              disabled={isBulkPending}
              onClick={() => applyBulk({ isRead: true })}
            >
              <CheckCheck size={14} />
              Mark read
            </button>
            <button
              type="button"
              className={bulkButtonClass}
              disabled={isBulkPending}
              onClick={() => applyBulk({ isRead: false, readingProgress: 0 })}
            >
              <Undo2 size={14} />
              Mark unread
            </button>
            <button
              type="button"
              className={bulkButtonClass}
              disabled={isBulkPending}
              onClick={() => applyBulk({ archived: !showingArchived })}
            >
              {showingArchived ? (
                <ArchiveRestore size={14} />
              ) : (
                <Archive size={14} />
              )}
              {showingArchived ? "Unarchive" : "Archive"}
            </button>
          </div>
        )}
      </div>

      <div
        ref={scrollContainerRef}
        className="grow px-6 py-6 overflow-y-auto scroll-smooth md:px-4 md:pb-6"
      >
        <div className="flex flex-col divide-y divide-gray-200 dark:divide-gray-700">
          {links.map((link) => {
            const progress = link.is_read
              ? 100
              : Math.round((link.reading_progress ?? 0) * 100);

            return (
              <div
                key={link.id}
                className="flex items-start gap-3 py-4 px-2 -mx-2 rounded-lg transition-colors hover:bg-gray-50 dark:hover:bg-gray-700/30"
              >
                <input
                  type="checkbox"
                  className="mt-1.5 w-4 h-4 shrink-0 rounded border-gray-300 dark:border-gray-600 accent-blue-600 cursor-pointer"
                  checked={selected.has(link.slug)}
                  onChange={() => toggleSelected(link.slug)}
                />

                <Link to={`/app/read/${link.slug}`} className="flex-1 min-w-0">
                  <article className="flex gap-4 cursor-pointer">
                    {link.image_url && (
                      <img
                        src={link.image_url}
                        alt=""
                        className="w-28 h-20 rounded-lg object-cover shrink-0 bg-gray-100 dark:bg-gray-700"
                        loading="lazy"
                        onError={(e) => {
                          (e.target as HTMLImageElement).style.display =
                            "none";
                        }}
                      />
                    )}

                    <div className="flex-1 min-w-0 flex flex-col gap-1.5">
                      <div className="flex items-start justify-between gap-4">
                        <h3
                          className={`text-base font-semibold leading-snug line-clamp-2 ${
                            link.is_read
                              ? "text-gray-400 dark:text-gray-500"
                              : "text-gray-900 dark:text-white"
                          }`}
                        >
                          {link.title}
                        </h3>
                        <time className="text-xs text-gray-400 dark:text-gray-500 whitespace-nowrap mt-0.5 shrink-0">
                          {formatDate(link.published_at)}
                        </time>
                      </div>

                      {link.description && (
                        <p className="text-sm text-gray-500 dark:text-gray-400 line-clamp-2 leading-relaxed">
                          {link.description}
                        </p>
                      )}

                      <div className="flex items-center gap-3 mt-1 text-xs text-gray-400 dark:text-gray-500">
                        {link.feed_title && (
                          <span className="flex items-center gap-1">
                            <Rss size={12} />
                            {link.feed_title}
                          </span>
                        )}
                        {link.reading_time_minutes > 0 && (
                          <span className="flex items-center gap-1">
                            <Clock size={12} />
                            {Math.round(link.reading_time_minutes)} min
                          </span>
                        )}

                        <span className="ml-auto flex items-center gap-1">
                          <button
                            type="button"
                            title={link.is_starred ? "Unstar" : "Star"}
                            className="p-1.5 rounded-md transition-colors hover:bg-gray-200 dark:hover:bg-gray-600/50"
                            onClick={(e) => {
                              e.preventDefault();
                              e.stopPropagation();
                              updateLink.mutate({
                                slug: link.slug,
                                isStarred: !link.is_starred,
                              });
                            }}
                          >
                            <Star
                              size={16}
                              className={
                                link.is_starred
                                  ? "text-amber-400 fill-amber-400"
                                  : "text-gray-400 dark:text-gray-500"
                              }
                            />
                          </button>
                          <button
                            type="button"
                            title={showingArchived ? "Unarchive" : "Archive"}
                            className="p-1.5 rounded-md transition-colors hover:bg-gray-200 dark:hover:bg-gray-600/50"
                            onClick={(e) => {
                              e.preventDefault();
                              e.stopPropagation();
                              updateLink.mutate({
                                slug: link.slug,
                                archived: !showingArchived,
                              });
                            }}
                          >
                            {showingArchived ? (
                              <ArchiveRestore
                                size={16}
                                className="text-gray-400 dark:text-gray-500"
                              />
                            ) : (
                              <Archive
                                size={16}
                                className="text-gray-400 dark:text-gray-500"
                              />
                            )}
                          </button>
                        </span>
                      </div>

                      {progress > 0 && (
                        <div className="mt-1.5 h-1 w-full rounded-full bg-gray-100 dark:bg-gray-700 overflow-hidden">
                          <div
                            className={`h-full rounded-full transition-[width] duration-300 ${
                              link.is_read ? "bg-emerald-500" : "bg-blue-500"
                            }`}
                            style={{ width: `${progress}%` }}
                          />
                        </div>
                      )}
                    </div>
                  </article>
                </Link>
              </div>
            );
          })}

          {links.length === 0 && !isFetchingNextPage && (
            <p className="py-12 text-center text-sm text-gray-400 dark:text-gray-500">
              Nothing here yet.
            </p>
          )}

          <div ref={loaderRef} className="py-6 flex justify-center">
            {isFetchingNextPage && (
              <Loader2 size={20} className="animate-spin text-gray-400" />
            )}
          </div>
        </div>
      </div>
    </div>
  );
};
