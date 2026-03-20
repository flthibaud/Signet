import { useState, useEffect, useRef, useCallback } from "react";
import { Clock, Rss, Loader2 } from "lucide-react";

interface LinkData {
  id: number;
  title: string;
  description: string;
  image_url: string;
  saved_at: string;
  feed_title: string;
  reading_time_minutes: number;
  published_at: string;
}

interface Metadata {
  current_page: number;
  page_size: number;
  total_records: number;
  total_pages: number;
}

const formatDate = (dateStr: string) => {
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

  if (diffDays === 0) return "Aujourd'hui";
  if (diffDays === 1) return "Hier";
  if (diffDays < 7) return `Il y a ${diffDays} jours`;

  return date.toLocaleDateString("fr-FR", {
    day: "numeric",
    month: "short",
    year: date.getFullYear() !== now.getFullYear() ? "numeric" : undefined,
  });
};

export const Link = () => {
  const [links, setLinks] = useState<LinkData[]>([]);
  const [metadata, setMetadata] = useState<Metadata | null>(null);
  const [loading, setLoading] = useState(false);
  const loaderRef = useRef<HTMLDivElement>(null);
  const pageRef = useRef(1);

  const hasMore = metadata ? metadata.current_page < metadata.total_pages : true;

  const fetchLinks = useCallback(async (page: number) => {
    setLoading(true);
    try {
      const res = await fetch(`/v1/links?page=${page}`);
      const data = await res.json();
      setLinks((prev) => page === 1 ? (data.links ?? []) : [...prev, ...(data.links ?? [])]);
      setMetadata(data.metadata);
      pageRef.current = page;
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchLinks(1);
  }, [fetchLinks]);

  useEffect(() => {
    if (!loaderRef.current) return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && !loading && hasMore) {
          fetchLinks(pageRef.current + 1);
        }
      },
      { rootMargin: "200px" },
    );

    observer.observe(loaderRef.current);
    return () => observer.disconnect();
  }, [loading, hasMore, fetchLinks]);

  return (
    <div className="flex flex-col divide-y divide-gray-200 dark:divide-gray-700">
      {links.map((link) => (
        <article
          key={link.id}
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
                {formatDate(link.saved_at)}
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
            </div>
          </div>
        </article>
      ))}

      <div ref={loaderRef} className="py-6 flex justify-center">
        {loading && <Loader2 size={20} className="animate-spin text-gray-400" />}
      </div>
    </div>
  );
};
