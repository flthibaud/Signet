import { Loader2, Rss, Trash2 } from "lucide-react";
import { Link } from "react-router";

import Select from "~/components/Select";
import { useSubscriptions, useUnsubscribe } from "~/lib/feeds";
import { useFolders, useSetSubscriptionFolder } from "~/lib/folders";
import type { Folder, Subscription } from "~/types/subscription";

const NO_FOLDER = { id: "none", title: "No folder", folderId: null } as const;

type FolderOption = { id: string; title: string; folderId: number | null };

function folderOptions(folders: Folder[]): FolderOption[] {
  return [
    NO_FOLDER,
    ...folders.map((folder) => ({
      id: String(folder.id),
      title: folder.name,
      folderId: folder.id,
    })),
  ];
}

const SubscriptionList = () => {
  const { data, isPending, isError } = useSubscriptions();
  const unsubscribe = useUnsubscribe();
  const { data: foldersData } = useFolders();
  const setFolder = useSetSubscriptionFolder();

  const options = folderOptions(foldersData?.folders ?? []);

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

  return (
    <div className="mt-10 mx-auto max-w-126">
      <ul className="flex flex-col divide-y divide-gray-200 dark:divide-gray-700">
        {subscriptions.map((sub) => (
          <SubscriptionRow
            key={sub.id}
            subscription={sub}
            options={options}
            onMove={(folderId) => setFolder.mutate({ id: sub.id, folderId })}
            onUnsubscribe={() => unsubscribe.mutate(sub.id)}
            isRemoving={
              unsubscribe.isPending && unsubscribe.variables === sub.id
            }
          />
        ))}
      </ul>
    </div>
  );
};

type SubscriptionRowProps = {
  subscription: Subscription;
  options: FolderOption[];
  onMove: (folderId: number | null) => void;
  onUnsubscribe: () => void;
  isRemoving: boolean;
};

const SubscriptionRow = ({
  subscription,
  options,
  onMove,
  onUnsubscribe,
  isRemoving,
}: SubscriptionRowProps) => {
  const { feed, custom_title, unread_count, folder } = subscription;
  const title = custom_title || feed.title || feed.url;
  const current =
    options.find((option) => option.folderId === (folder?.id ?? null)) ??
    options[0];

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

      <Select
        className="shrink-0 w-40 max-md:w-32"
        small
        items={options}
        value={current}
        onChange={(option: FolderOption) => onMove(option.folderId)}
      />

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
