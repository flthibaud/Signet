import { useState, useEffect } from "react";

interface LinkData {
  title: string;
  description: string;
  image_url: string;
  saved_at: string;
  feed_title: string;
  reading_time_minutes: number;
}

export const Link = () => {
  const [links, setLinks] = useState<LinkData[]>([]);

  useEffect(() => {
    fetch("/v1/links?page=1")
      .then((res) => res.json())
      .then((data) => setLinks(data.links ?? []));
  }, []);

  return (
    <>
      {links.map((link, i) => (
        <div key={i} className="flex gap-4 mb-6 last:mb-0 cursor-pointer">
          <img src={link.image_url} alt={link.title} className="w-24 h-24 rounded-sm" />
          <div className="flex flex-col gap-4">
            <div className="flex flex-row items-center justify-between">
              <h3 className="text-xl font-medium text-gray-900 dark:text-white">
                {link.title}
              </h3>
              <span className="text-sm font-medium text-gray-900 dark:text-white">
                {new Date(link.saved_at).toLocaleDateString("fr-FR", {
                  day: "2-digit",
                  month: "long",
                  year: "numeric",
                })}
              </span>
            </div>
            <p className="text-base text-ellipsis text-gray-500 dark:text-gray-400 whitespace-nowrap overflow-hidden w-200">
              {link.description}
            </p>
            <div className="flex gap-2">
              <span className="text-sm text-gray-500 dark:text-gray-400">
                {link.feed_title}
              </span>
              <span className="text-sm text-gray-500 dark:text-gray-400">
                {" "}- {Math.round(link.reading_time_minutes)} minutes
              </span>
              <span className="text-sm text-gray-500 dark:text-gray-400">
                {" "}- tags
              </span>
            </div>
          </div>
        </div>
      ))}
    </>
  );
};
