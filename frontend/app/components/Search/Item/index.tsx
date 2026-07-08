import { Link } from "react-router";
import { Clock, Rss, Star } from "lucide-react";
import type { SearchResultGroup } from "~/mocks/resultSearch";

type ItemProps = {
    item: SearchResultGroup;
};

const Item = ({ item }: ItemProps) => (
    <div className="">
        <div className="flex items-center py-3 md:pt-3 pt-6">
            <div className="h6">{item.title}</div>
            {item.date && (
                <div className="ml-5 caption1 text-[#6C7275]/75">
                    {item.date}
                </div>
            )}
        </div>
        <div className="md:-mx-5 mx-0">
            {item.list.map((x) => (
                <Link
                    className="group relative flex items-center pl-5 py-5 pr-24 rounded-xl transition-colors hover:bg-[#E8ECEF]/50 md:!bg-transparent md:py-4 md:pl-0 md:pr-18 md:mb-2 md:last:mb-0 dark:hover:bg-[#232627] dark:md:hover:bg-transparent"
                    key={x.id}
                    to={`/app/read/${x.slug}`}
                >
                    <div className="relative shrink-0 flex items-center justify-center w-12 h-12 rounded-xl bg-[#F3F5F7] dark:bg-[#232627] overflow-hidden">
                        {x.imageUrl ? (
                            <img
                                className="w-full h-full object-cover"
                                src={x.imageUrl}
                                alt=""
                                loading="lazy"
                            />
                        ) : (
                            <Rss className="w-5 h-5 text-[#6C7275]/60" />
                        )}
                    </div>
                    <div className="w-[calc(100%-3rem)] pl-5">
                        <div
                            className={`mb-1 flex items-center gap-2 truncate base1 font-semibold ${
                                x.isRead ? "text-[#6C7275]" : ""
                            }`}
                        >
                            <span className="truncate">{x.title}</span>
                            {x.isStarred && (
                                <Star className="shrink-0 w-4 h-4 fill-[#FFB800] text-[#FFB800]" />
                            )}
                        </div>
                        <div className="truncate caption1 text-[#6C7275]/75">
                            {x.description}
                        </div>
                        <div className="flex items-center gap-3 mt-1.5 caption2 text-[#6C7275]/60">
                            <span className="flex items-center gap-1">
                                <Rss className="w-3.5 h-3.5" />
                                {x.feedTitle}
                            </span>
                            {x.readingTimeMinutes > 0 && (
                                <span className="flex items-center gap-1">
                                    <Clock className="w-3.5 h-3.5" />
                                    {x.readingTimeMinutes} min
                                </span>
                            )}
                        </div>
                    </div>
                    <div className="absolute top-1/2 right-5 -translate-y-1/2 caption1 text-[#6C7275]/50 group-hover:hidden md:right-0">
                        {x.savedAt}
                    </div>
                    <div className="absolute top-1/2 right-5 -translate-y-1/2 px-2 rounded bg-[#FEFEFE] caption1 font-semibold text-[#6C7275] hidden group-hover:block md:right-0 dark:bg-[#343839] dark:text-[#E8ECEF]">
                        Read
                    </div>
                </Link>
            ))}
        </div>
    </div>
);

export default Item;
