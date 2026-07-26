// Mock data for the search UI. Shaped after the `Link` domain model
// (see app/types/link.ts): a saved article carries a feed source, a reading
// time, read/starred state, and a slug used to build its /app/read/:slug URL.

export type SearchResultItem = {
    id: string;
    slug: string;
    title: string;
    description: string;
    feedTitle: string;
    readingTimeMinutes: number;
    savedAt: string;
    imageUrl?: string;
    isRead: boolean;
    isStarred: boolean;
};

export type SearchResultGroup = {
    id: string;
    title: string;
    date?: string;
    list: SearchResultItem[];
};

export const resultSearch: SearchResultGroup[] = [
    {
        id: "0",
        title: "Today",
        date: "Thu 16 Feb",
        list: [
            {
                id: "0",
                slug: "the-hidden-cost-of-context-switching",
                title: "The hidden cost of context switching",
                description:
                    "Every time you jump between tasks your brain pays a tax. A look at why deep work is so hard to protect and what actually helps.",
                feedTitle: "Ness Labs",
                readingTimeMinutes: 6,
                savedAt: "1m ago",
                isRead: false,
                isStarred: true,
            },
            {
                id: "1",
                slug: "building-a-self-hosted-rss-reader",
                title: "Building a self-hosted RSS reader in a weekend",
                description:
                    "A practical walkthrough of parsing feeds, deduplicating articles, and extracting readable content from messy HTML.",
                feedTitle: "Julia Evans",
                readingTimeMinutes: 12,
                savedAt: "18m ago",
                isRead: false,
                isStarred: false,
            },
            {
                id: "2",
                slug: "why-rss-never-really-died",
                title: "Why RSS never really died",
                description:
                    "Long declared obsolete, the humble feed quietly powers podcasts, newsletters, and a growing indie web revival.",
                feedTitle: "The Verge",
                readingTimeMinutes: 8,
                savedAt: "43m ago",
                isRead: true,
                isStarred: false,
            },
            {
                id: "3",
                slug: "postgres-full-text-search-basics",
                title: "Getting started with Postgres full-text search",
                description:
                    "tsvector, tsquery, and ranking explained with real queries you can drop into an existing schema today.",
                feedTitle: "Crunchy Data",
                readingTimeMinutes: 10,
                savedAt: "2h ago",
                isRead: false,
                isStarred: true,
            },
        ],
    },
    {
        id: "1",
        title: "Last 30 days",
        list: [
            {
                id: "0",
                slug: "the-case-for-single-binary-deployments",
                title: "The case for single-binary deployments",
                description:
                    "Shipping one Go binary with an embedded frontend removes a whole category of ops problems. Here's the trade-off.",
                feedTitle: "Dave Cheney",
                readingTimeMinutes: 7,
                savedAt: "5 days ago",
                isRead: true,
                isStarred: false,
            },
            {
                id: "1",
                slug: "designing-for-read-it-later",
                title: "Designing for read-it-later, not read-it-now",
                description:
                    "What changes about typography, layout, and progress tracking when you assume the reader saved this for a calmer moment.",
                feedTitle: "Smashing Magazine",
                readingTimeMinutes: 9,
                savedAt: "3 weeks ago",
                isRead: false,
                isStarred: false,
            },
        ],
    },
];
