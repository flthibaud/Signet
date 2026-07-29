import { Fragment } from "react";
import { Link } from "react-router";
import { Clock, Rss, Star } from "lucide-react";

import {
  HIGHLIGHT_END,
  HIGHLIGHT_START,
  type SearchResultGroup,
} from "~/types/search";
import { formatRelativeDate } from "../grouping";

/**
 * Renders a snippet's `[[hl]]…[[/hl]]` runs as <mark> elements.
 *
 * The markers are split out into React nodes rather than injected as HTML, so
 * article text can never reach the DOM as markup.
 */
function highlight(snippet: string) {
  return snippet.split(HIGHLIGHT_START).map((chunk, index) => {
    if (index === 0) return <Fragment key={index}>{chunk}</Fragment>;

    const [marked, ...rest] = chunk.split(HIGHLIGHT_END);
    return (
      <Fragment key={index}>
        <mark className="bg-transparent font-semibold text-n-7 dark:text-n-1">
          {marked}
        </mark>
        {rest.join(HIGHLIGHT_END)}
      </Fragment>
    );
  });
}

type ItemProps = {
  item: SearchResultGroup;
};

const Item = ({ item }: ItemProps) => (
  <div className="">
    <div className="flex items-center py-3 md:pt-3 pt-6">
      <div className="h6">{item.title}</div>
      {item.date && (
        <div className="ml-5 caption1 text-n-4/75">{item.date}</div>
      )}
    </div>
    <div className="md:-mx-5 mx-0">
      {item.list.map((x) => (
        <Link
          className="group relative flex items-center pl-5 py-5 pr-24 rounded-xl transition-colors hover:bg-n-3/50 max-md:bg-transparent! max-md:py-4 max-md:pl-0 max-md:pr-18 max-md:mb-2 max-md:last:mb-0 dark:hover:bg-n-6 dark:max-md:hover:bg-transparent"
          key={x.id}
          to={`/app/read/${x.slug}`}
        >
          <div className="relative shrink-0 flex items-center justify-center w-12 h-12 rounded-xl bg-n-2 dark:bg-n-6 overflow-hidden">
            {x.image_url ? (
              <img
                className="w-full h-full object-cover"
                src={x.image_url}
                alt=""
                loading="lazy"
              />
            ) : (
              <Rss className="w-5 h-5 text-n-4/60" />
            )}
          </div>
          <div className="w-[calc(100%-3rem)] pl-5">
            <div
              className={`mb-1 flex items-center gap-2 truncate base1 font-semibold ${
                x.is_read ? "text-n-4" : ""
              }`}
            >
              <span className="truncate">{x.title}</span>
              {x.is_starred && (
                <Star className="shrink-0 w-4 h-4 fill-[#FFB800] text-[#FFB800]" />
              )}
            </div>
            {x.snippet && (
              <div className="truncate caption1 text-n-4/75">
                {highlight(x.snippet)}
              </div>
            )}
            <div className="flex items-center gap-3 mt-1.5 caption2 text-n-4/60">
              {x.feed_title && (
                <span className="flex items-center gap-1">
                  <Rss className="w-3.5 h-3.5" />
                  {x.feed_title}
                </span>
              )}
              {x.reading_time_minutes > 0 && (
                <span className="flex items-center gap-1">
                  <Clock className="w-3.5 h-3.5" />
                  {Math.round(x.reading_time_minutes)} min
                </span>
              )}
            </div>
          </div>
          <div className="absolute top-1/2 right-5 -translate-y-1/2 caption1 text-n-4/50 group-hover:hidden max-md:right-0">
            {formatRelativeDate(x.published_at)}
          </div>
          <div className="absolute top-1/2 right-5 -translate-y-1/2 px-2 rounded bg-n-1 caption1 font-semibold text-n-4 hidden group-hover:block max-md:right-0 dark:bg-n-5 dark:text-n-3">
            Read
          </div>
        </Link>
      ))}
    </div>
  </div>
);

export default Item;
