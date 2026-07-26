import { useState } from "react";
import Select from "~/components/Select";
import Item from "./Item";
import type { SearchResultGroup } from "~/mocks/resultSearch";

import { Search as SearchIcon, Rss, Clock } from "lucide-react";

const sources = [
    {
        id: "all",
        title: "All feeds",
    },
    {
        id: "0",
        title: "Ness Labs",
    },
    {
        id: "1",
        title: "The Verge",
    },
    {
        id: "2",
        title: "Julia Evans",
    },
];

const dates = [
    {
        id: "0",
        title: "Today",
    },
    {
        id: "1",
        title: "Last week",
    },
    {
        id: "2",
        title: "Last 30 days",
    },
];

type SearchProps = {
    items: SearchResultGroup[];
};

const Search = ({ items }: SearchProps) => {
    const [search, setSearch] = useState<string>("");
    const [source, setSource] = useState<any>();
    const [date, setDate] = useState<any>();

    return (
        <form
            className="flex flex-col min-h-0 h-full"
            action=""
            onSubmit={() => console.log("Submit")}
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
                    value={search}
                    onChange={(e: any) => setSearch(e.target.value)}
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
                        items={dates}
                        value={date}
                        onChange={setDate}
                    />
                </div>
            </div>
            {/* Scrollable results: only this region scrolls */}
            <div className="grow min-h-0 overflow-y-auto px-10 pb-6 md:px-6 scrollbar-none">
                {items.map((x) => (
                    <Item item={x} key={x.id} />
                ))}
            </div>
        </form>
    );
};

export default Search;
