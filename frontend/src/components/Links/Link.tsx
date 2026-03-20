import { useState, useEffect } from "react";
import { Clock, Rss } from "lucide-react";

interface LinkData {
  title: string;
  description: string;
  image_url: string;
  saved_at: string;
  feed_title: string;
  reading_time_minutes: number;
  published_at: string;
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

  useEffect(() => {
    fetch("/v1/links?page=1")
      .then((res) => res.json())
      .then((data) => setLinks(data.links ?? []));
  }, []);

  return (
    <div className="flex flex-col divide-y divide-gray-200 dark:divide-gray-700">
      {links.map((link, i) => (
        <article
          key={i}
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
    </div>
  );
};
