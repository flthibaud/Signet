import { useState } from "react";
import { Disclosure, DisclosureButton, DisclosurePanel } from "@headlessui/react";
import { Link } from "react-router";
import {
  ChevronDown,
  ChevronRight,
  CirclePlus,
  Folder as FolderIcon,
  Inbox,
  Loader2,
  Pencil,
  Rss,
  Trash2,
} from "lucide-react";

import Modal from "~/components/Modal";
import AddFolderList from "~/components/AddFolderList";
import { useDeleteFolder, useFolders } from "~/lib/folders";
import { useSubscriptions } from "~/lib/feeds";
import type { Folder, Subscription } from "~/types/subscription";

const UNFILED_ID = "unfiled";

type Group = {
  key: string;
  name: string;
  folder: Folder | null;
  subscriptions: Subscription[];
  unread: number;
};

/**
 * Folders come from their own endpoint so that an empty one still shows up;
 * the subscriptions fill them in. Unfiled feeds land in a trailing pseudo-group
 * that has no row in the folders table.
 */
function buildGroups(folders: Folder[], subscriptions: Subscription[]): Group[] {
  const groups = new Map<string, Group>();

  for (const folder of folders) {
    groups.set(String(folder.id), {
      key: String(folder.id),
      name: folder.name,
      folder,
      subscriptions: [],
      unread: 0,
    });
  }

  const unfiled: Group = {
    key: UNFILED_ID,
    name: "Uncategorized",
    folder: null,
    subscriptions: [],
    unread: 0,
  };

  for (const sub of subscriptions) {
    // A folder the list does not know about is treated as unfiled rather than
    // dropped, so a feed can never disappear from the sidebar.
    const group = sub.folder ? groups.get(String(sub.folder.id)) ?? unfiled : unfiled;
    group.subscriptions.push(sub);
    group.unread += sub.unread_count;
  }

  const ordered = [...groups.values()];
  if (unfiled.subscriptions.length > 0) {
    ordered.push(unfiled);
  }

  return ordered;
}

function subscriptionTitle(sub: Subscription): string {
  return sub.custom_title || sub.feed.title || sub.feed.url;
}

type FolderListProps = {
  visible?: boolean;
};

const FolderList = ({ visible }: FolderListProps) => {
  const [visibleModal, setVisibleModal] = useState<boolean>(false);
  const [editing, setEditing] = useState<Folder | null>(null);

  const foldersQuery = useFolders();
  const subscriptionsQuery = useSubscriptions();
  const deleteFolder = useDeleteFolder();

  const isPending = foldersQuery.isPending || subscriptionsQuery.isPending;
  const isError = foldersQuery.isError || subscriptionsQuery.isError;

  const groups = buildGroups(
    foldersQuery.data?.folders ?? [],
    subscriptionsQuery.data?.subscriptions ?? [],
  );

  const openCreate = () => {
    setEditing(null);
    setVisibleModal(true);
  };

  const openRename = (folder: Folder) => {
    setEditing(folder);
    setVisibleModal(true);
  };

  const onDelete = (folder: Folder) => {
    const confirmed = window.confirm(
      `Delete the folder "${folder.name}"? Its feeds stay subscribed and move back to Uncategorized.`,
    );
    if (confirmed) {
      deleteFolder.mutate(folder.id);
    }
  };

  return (
    <>
      <div className="flex flex-col min-h-0 grow pb-6">
        <div
          className={`min-h-0 grow overflow-y-auto scroll-smooth scrollbar-none ${
            !visible && "px-2"
          }`}
        >
          {isPending && (
            <div className="flex items-center justify-center h-12">
              <Loader2 size={16} className="animate-spin text-n-4" />
            </div>
          )}

          {isError && visible && (
            <div className="px-5 py-3 caption1 text-n-4">
              Your folders could not be loaded.
            </div>
          )}

          {!isPending && !isError && groups.length === 0 && visible && (
            <div className="px-5 py-3 caption1 text-n-4">No folders yet.</div>
          )}

          {groups.map((group) => (
            <FolderRow
              key={group.key}
              group={group}
              visible={visible}
              onRename={openRename}
              onDelete={onDelete}
            />
          ))}
        </div>
        <button
          className={`group flex shrink-0 items-center w-full h-12 text-left base2 text-n-3/75 transition-colors hover:cursor-pointer hover:text-n-3 ${
            visible ? "px-5" : "justify-center px-3"
          }`}
          onClick={openCreate}
        >
          <CirclePlus className="text-n-4 transition-colors group-hover:text-n-3" />
          {visible && <div className="ml-5">New folder</div>}
        </button>
      </div>
      <Modal
        className="max-md:p-0!"
        classWrap="max-w-160 max-md:min-h-dvh max-md:rounded-none max-md:pb-8"
        classButtonClose="absolute top-6 right-6 w-10 h-10 rounded-full bg-[#F3F5F7] max-md:right-5 dark:bg-[#6C7275]/25 dark:text-[#6C7275] dark:hover:text-[#FEFEFE]"
        visible={visibleModal}
        onClose={() => setVisibleModal(false)}
      >
        <AddFolderList
          folder={editing}
          onCancel={() => setVisibleModal(false)}
          onDone={() => setVisibleModal(false)}
        />
      </Modal>
    </>
  );
};

type FolderRowProps = {
  group: Group;
  visible?: boolean;
  onRename: (folder: Folder) => void;
  onDelete: (folder: Folder) => void;
};

const FolderRow = ({ group, visible, onRename, onDelete }: FolderRowProps) => {
  const Icon = group.folder ? FolderIcon : Inbox;

  // Collapsed, there is no room for a tree: a folder becomes a filter link.
  // The unfiled group has no folder id to filter on, so it is left out.
  if (!visible) {
    if (!group.folder) return null;

    return (
      <Link
        to={`/app?folder_id=${group.folder.id}`}
        title={group.name}
        className="flex justify-center items-center w-full h-12 rounded-lg text-n-3/75 transition-colors hover:text-n-1"
      >
        <Icon size={20} />
      </Link>
    );
  }

  return (
    <Disclosure>
      {({ open }) => (
        <>
          {/* The folder name links to its filtered article list, so it sits
              beside the toggle rather than inside it: an anchor nested in a
              button is invalid HTML. */}
          <div className="group flex items-center w-full h-12 pl-3 pr-2 rounded-lg text-n-3/75 base2 font-semibold transition-colors hover:text-n-1">
            <DisclosureButton
              aria-label={open ? `Collapse ${group.name}` : `Expand ${group.name}`}
              className="flex justify-center items-center w-6 h-6 shrink-0 text-n-4 transition-colors hover:cursor-pointer hover:text-n-3"
            >
              {open ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
            </DisclosureButton>
            <Icon size={18} className="ml-2 shrink-0 text-n-4" />
            {group.folder ? (
              <Link to={`/app?folder_id=${group.folder.id}`} className="ml-3 truncate">
                {group.name}
              </Link>
            ) : (
              <span className="ml-3 truncate">{group.name}</span>
            )}

            {group.folder && (
              <div className="ml-auto flex items-center opacity-0 transition-opacity group-hover:opacity-100">
                <button
                  type="button"
                  aria-label={`Rename ${group.name}`}
                  onClick={() => onRename(group.folder!)}
                  className="p-1 rounded text-n-4 transition-colors hover:cursor-pointer hover:text-n-1"
                >
                  <Pencil size={14} />
                </button>
                <button
                  type="button"
                  aria-label={`Delete ${group.name}`}
                  onClick={() => onDelete(group.folder!)}
                  className="p-1 rounded text-n-4 transition-colors hover:cursor-pointer hover:text-red-500"
                >
                  <Trash2 size={14} />
                </button>
              </div>
            )}

            {group.unread > 0 && (
              <div
                className={`px-2 bg-n-6 rounded-lg base2 font-semibold text-n-4 ${
                  group.folder ? "ml-2 group-hover:hidden" : "ml-auto"
                }`}
              >
                {group.unread}
              </div>
            )}
          </div>

          <DisclosurePanel>
            {group.subscriptions.length === 0 ? (
              <div className="pl-14 py-2 caption1 text-n-4/75">No feeds yet.</div>
            ) : (
              group.subscriptions.map((sub) => (
                <FeedRow key={sub.id} subscription={sub} />
              ))
            )}
          </DisclosurePanel>
        </>
      )}
    </Disclosure>
  );
};

const FeedRow = ({ subscription }: { subscription: Subscription }) => {
  const { feed, unread_count } = subscription;
  const title = subscriptionTitle(subscription);

  return (
    <Link
      to={`/app?feed_id=${feed.id}`}
      className="flex items-center w-full h-10 pl-11 pr-2 rounded-lg text-n-3/60 caption1 transition-colors hover:text-n-1"
    >
      {feed.image_url ? (
        <img
          src={feed.image_url}
          alt=""
          className="w-4 h-4 shrink-0 rounded object-cover"
          loading="lazy"
          onError={(e) => {
            (e.target as HTMLImageElement).style.display = "none";
          }}
        />
      ) : (
        <Rss size={14} className="shrink-0 text-n-4" />
      )}
      <span className="ml-3 truncate">{title}</span>
      {unread_count > 0 && (
        <span className="ml-auto pl-2 shrink-0 text-n-4">{unread_count}</span>
      )}
    </Link>
  );
};

export default FolderList;
