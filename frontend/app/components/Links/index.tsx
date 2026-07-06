import { useEffect, useRef } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { Archive, Clock, Rss, Loader2, Star } from "lucide-react";
import type { LinksResponse } from "~/types/link";
import { formatDate } from "~/utils/formatDate";
import { Link } from "react-router";
import { apiFetch } from "~/lib/api";
import { useUpdateLink } from "~/lib/links";

export const Links = () => {
  const loaderRef = useRef<HTMLDivElement>(null);
  const updateLink = useUpdateLink();

  const { data, fetchNextPage, hasNextPage, isFetchingNextPage } =
    useInfiniteQuery<LinksResponse>({
      queryKey: ["links"],
      queryFn: ({ pageParam }) =>
        apiFetch<LinksResponse>(`/v1/links?page=${pageParam}`),
      initialPageParam: 1,
      getNextPageParam: (lastPage) =>
        lastPage.metadata.current_page < lastPage.metadata.total_pages
          ? lastPage.metadata.current_page + 1
          : undefined,
    });

  const links = data?.pages.flatMap((p) => p.links ?? []) ?? [];

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

  return (
    <div className="flex flex-col divide-y divide-gray-200 dark:divide-gray-700">
      {links.map((link) => (
        <Link to={`/app/read/${link.slug}`} className="flex-1" key={link.id}>
          <article
            className="flex gap-4 py-4 px-2 -mx-2 rounded-lg cursor-pointer transition-colors hover:bg-gray-50 dark:hover:bg-gray-700/30"
          >
            {link.image_url && (
              <img
                src={link.image_url}
                alt=""
                className="w-28 h-20 rounded-lg object-cover shrink-0 bg-gray-100 dark:bg-gray-700"
                loading="lazy"
                onError={(e) => {
                  (e.target as HTMLImageElement).style.display = "none";
                }}
              />
            )}

            <div className="flex-1 min-w-0 flex flex-col gap-1.5">
              <div className="flex items-start justify-between gap-4">
                <h3 className="text-base font-semibold text-gray-900 dark:text-white leading-snug line-clamp-2">
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
                    title="Archive"
                    className="p-1.5 rounded-md transition-colors hover:bg-gray-200 dark:hover:bg-gray-600/50"
                    onClick={(e) => {
                      e.preventDefault();
                      e.stopPropagation();
                      updateLink.mutate({ slug: link.slug, archived: true });
                    }}
                  >
                    <Archive
                      size={16}
                      className="text-gray-400 dark:text-gray-500"
                    />
                  </button>
                </span>
              </div>
            </div>
          </article>
        </Link>
      ))}

      <div ref={loaderRef} className="py-6 flex justify-center">
        {isFetchingNextPage && (
          <Loader2 size={20} className="animate-spin text-gray-400" />
        )}
      </div>
    </div>
  );
};
