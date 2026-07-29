import { Loader2, Rss, Trash2 } from "lucide-react";
import { Link } from "react-router";

import { useSubscriptions, useUnsubscribe } from "~/lib/feeds";
import type { Subscription } from "~/types/subscription";

const UNFILED = "Uncategorized";

type Group = {
  folderId: number | null;
  name: string;
  subscriptions: Subscription[];
};

/**
 * Groups subscriptions by folder, alphabetically, with the unfiled ones last.
 */
function groupByFolder(subscriptions: Subscription[]): Group[] {
  const groups = new Map<string, Group>();

  for (const sub of subscriptions) {
    const name = sub.folder?.name ?? UNFILED;

    let group = groups.get(name);
    if (!group) {
      group = { folderId: sub.folder?.id ?? null, name, subscriptions: [] };
      groups.set(name, group);
    }
    group.subscriptions.push(sub);
  }

  return [...groups.values()].sort((a, b) => {
    if (a.folderId === null) return 1;
    if (b.folderId === null) return -1;
    return a.name.localeCompare(b.name);
  });
}

const SubscriptionList = () => {
  const { data, isPending, isError } = useSubscriptions();
  const unsubscribe = useUnsubscribe();

  if (isPending) {
    return (
      <div className="mt-10 flex justify-center">
        <Loader2 size={20} className="animate-spin text-gray-400" />
      </div>
    );
  }

  if (isError) {
    return (
      <div className="mt-10 text-center text-sm text-red-500">
        Could not load your subscriptions.
      </div>
    );
  }

  const subscriptions = data.subscriptions ?? [];

  if (subscriptions.length === 0) {
    return (
      <div className="mt-10 text-center text-sm text-n-4">
        You have no subscriptions yet.
      </div>
    );
  }

  const groups = groupByFolder(subscriptions);
  const showHeaders = groups.length > 1 || groups[0].folderId !== null;

  return (
    <div className="mt-10 mx-auto max-w-126 flex flex-col gap-6">
      {groups.map((group) => (
        <section key={group.name}>
          {showHeaders && (
            <h2 className="mb-1 text-xs font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500">
              {group.folderId !== null ? (
                <Link
                  to={`/app?folder_id=${group.folderId}`}
                  className="transition-colors hover:text-[#0084FF]"
                >
                  {group.name}
                </Link>
              ) : (
                group.name
              )}
            </h2>
          )}

          <ul className="flex flex-col divide-y divide-gray-200 dark:divide-gray-700">
            {group.subscriptions.map((sub) => (
              <SubscriptionRow
                key={sub.id}
                subscription={sub}
                onUnsubscribe={() => unsubscribe.mutate(sub.id)}
                isRemoving={
                  unsubscribe.isPending && unsubscribe.variables === sub.id
                }
              />
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
};

type SubscriptionRowProps = {
  subscription: Subscription;
  onUnsubscribe: () => void;
  isRemoving: boolean;
};

const SubscriptionRow = ({
  subscription,
  onUnsubscribe,
  isRemoving,
}: SubscriptionRowProps) => {
  const { feed, custom_title, unread_count } = subscription;
  const title = custom_title || feed.title || feed.url;

  return (
    <li className="flex items-center gap-4 py-4">
      {feed.image_url ? (
        <img
          src={feed.image_url}
          alt=""
          className="w-10 h-10 rounded-lg object-cover shrink-0 bg-gray-100 dark:bg-gray-700"
          loading="lazy"
          onError={(e) => {
            (e.target as HTMLImageElement).style.display = "none";
          }}
        />
      ) : (
        <div className="w-10 h-10 rounded-lg shrink-0 flex items-center justify-center bg-gray-100 dark:bg-gray-700">
          <Rss size={18} className="text-gray-400" />
        </div>
      )}

      <div className="flex-1 min-w-0">
        <Link
          to={`/app?feed_id=${feed.id}`}
          className="block font-semibold text-gray-900 dark:text-white truncate transition-colors hover:text-[#0084FF]"
        >
          {title}
        </Link>
        {feed.site_url && (
          <a
            href={feed.site_url}
            target="_blank"
            rel="noreferrer"
            className="text-xs text-gray-400 dark:text-gray-500 hover:underline truncate block"
          >
            {feed.site_url}
          </a>
        )}
      </div>

      {unread_count > 0 && (
        <span className="shrink-0 text-xs font-semibold px-2 py-0.5 rounded-full bg-[#0084FF]/10 text-[#0084FF]">
          {unread_count}
        </span>
      )}

      <button
        type="button"
        onClick={onUnsubscribe}
        disabled={isRemoving}
        aria-label={`Unsubscribe from ${title}`}
        className="shrink-0 p-2 rounded-lg text-gray-400 transition-colors hover:text-red-500 hover:bg-red-500/10 disabled:opacity-50"
      >
        {isRemoving ? (
          <Loader2 size={18} className="animate-spin" />
        ) : (
          <Trash2 size={18} />
        )}
      </button>
    </li>
  );
};

export default SubscriptionList;
