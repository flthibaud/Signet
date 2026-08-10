import { useState } from "react";
import { Disclosure, DisclosureButton, DisclosurePanel } from "@headlessui/react";
import { Link } from "react-router";
import {
  ChevronDown,
  ChevronRight,
  Folder as FolderIcon,
  FolderPlus,
  Inbox,
  Loader2,
  Pencil,
  Plus,
  Rss,
  Trash2,
} from "lucide-react";

import Modal from "~/components/Modal";
import AddFolderList from "~/components/AddFolderList";
import ContextMenu, {
  type ContextMenuItem,
  type ContextMenuPosition,
} from "~/components/ContextMenu";
import {
  useDeleteFolder,
  useFolders,
  useSetSubscriptionFolder,
} from "~/lib/folders";
import { useSubscriptions, useUnsubscribe } from "~/lib/feeds";
import type { Folder, Subscription } from "~/types/subscription";

type MenuTarget =
  | { kind: "folder"; folder: Folder | null }
  | { kind: "feed"; subscription: Subscription };

type MenuState = { position: ContextMenuPosition; target: MenuTarget };

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
  const [menu, setMenu] = useState<MenuState | null>(null);

  const foldersQuery = useFolders();
  const subscriptionsQuery = useSubscriptions();
  const deleteFolder = useDeleteFolder();
  const setFolder = useSetSubscriptionFolder();
  const unsubscribe = useUnsubscribe();

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

  const onUnsubscribe = (subscription: Subscription) => {
    const confirmed = window.confirm(
      `Unsubscribe from "${subscriptionTitle(subscription)}"? The articles you already saved stay in your library.`,
    );
    if (confirmed) {
      unsubscribe.mutate(subscription.id);
    }
  };

  const openFolderMenu = (event: React.MouseEvent, folder: Folder | null) => {
    event.preventDefault();
    setMenu({
      position: { x: event.clientX, y: event.clientY },
      target: { kind: "folder", folder },
    });
  };

  const openFeedMenu = (event: React.MouseEvent, subscription: Subscription) => {
    event.preventDefault();
    setMenu({
      position: { x: event.clientX, y: event.clientY },
      target: { kind: "feed", subscription },
    });
  };

  const folders = foldersQuery.data?.folders ?? [];

  const menuItems: ContextMenuItem[] = (() => {
    if (!menu) return [];

    if (menu.target.kind === "folder") {
      const { folder } = menu.target;

      // The unfiled group is not a row in the table: it can only offer the
      // action that does not need an id.
      if (!folder) {
        return [
          { label: "New folder", icon: FolderPlus, onSelect: openCreate },
        ];
      }

      return [
        { label: "Rename", icon: Pencil, onSelect: () => openRename(folder) },
        {
          label: "Delete",
          icon: Trash2,
          danger: true,
          onSelect: () => onDelete(folder),
        },
        { type: "separator" },
        { label: "New folder", icon: FolderPlus, onSelect: openCreate },
      ];
    }

    const { subscription } = menu.target;
    const currentFolderID = subscription.folder?.id ?? null;

    return [
      { type: "label", label: "Move to" },
      ...folders.map((folder) => ({
        label: folder.name,
        icon: FolderIcon,
        selected: folder.id === currentFolderID,
        onSelect: () =>
          setFolder.mutate({ id: subscription.id, folderId: folder.id }),
      })),
      {
        label: "No folder",
        icon: Inbox,
        selected: currentFolderID === null,
        onSelect: () => setFolder.mutate({ id: subscription.id, folderId: null }),
      },
      { type: "separator" },
      {
        label: "Unsubscribe",
        icon: Trash2,
        danger: true,
        onSelect: () => onUnsubscribe(subscription),
      },
    ];
  })();

  return (
    <>
      <div className="flex flex-col min-h-0 grow pb-6">
        {/* Above the list, not under it: pinned there it keeps its place
            however many folders are expanded. */}
        <button
          type="button"
          aria-label="New folder"
          onClick={openCreate}
          className={`group flex shrink-0 items-center w-full h-10 mb-2 rounded-lg caption1 font-semibold text-n-4 transition-colors hover:cursor-pointer hover:bg-n-6 hover:text-n-1 ${
            visible ? "px-3" : "justify-center px-2"
          }`}
        >
          <span className="flex justify-center items-center w-6 h-6 shrink-0 rounded-md bg-n-6 transition-colors group-hover:bg-n-5">
            <Plus size={14} />
          </span>
          {visible && <span className="ml-3">New folder</span>}
        </button>

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
              onFolderMenu={openFolderMenu}
              onFeedMenu={openFeedMenu}
            />
          ))}
        </div>
      </div>

      <ContextMenu
        position={menu?.position ?? null}
        items={menuItems}
        onClose={() => setMenu(null)}
      />
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
  onFolderMenu: (event: React.MouseEvent, folder: Folder | null) => void;
  onFeedMenu: (event: React.MouseEvent, subscription: Subscription) => void;
};

const FolderRow = ({
  group,
  visible,
  onFolderMenu,
  onFeedMenu,
}: FolderRowProps) => {
  const Icon = group.folder ? FolderIcon : Inbox;

  // Collapsed, there is no room for a tree: a folder becomes a filter link.
  // The unfiled group has no folder id to filter on, so it is left out.
  if (!visible) {
    if (!group.folder) return null;

    return (
      <Link
        to={`/app?folder_id=${group.folder.id}`}
        title={group.name}
        onContextMenu={(event) => onFolderMenu(event, group.folder)}
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
          <div
            onContextMenu={(event) => onFolderMenu(event, group.folder)}
            className="flex items-center w-full h-12 pl-3 pr-2 rounded-lg text-n-3/75 base2 font-semibold transition-colors hover:text-n-1"
          >
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

            {group.unread > 0 && (
              <div className="ml-auto px-2 bg-n-6 rounded-lg base2 font-semibold text-n-4">
                {group.unread}
              </div>
            )}
          </div>

          <DisclosurePanel>
            {group.subscriptions.length === 0 ? (
              <div className="pl-14 py-2 caption1 text-n-4/75">No feeds yet.</div>
            ) : (
              group.subscriptions.map((sub) => (
                <FeedRow key={sub.id} subscription={sub} onMenu={onFeedMenu} />
              ))
            )}
          </DisclosurePanel>
        </>
      )}
    </Disclosure>
  );
};

type FeedRowProps = {
  subscription: Subscription;
  onMenu: (event: React.MouseEvent, subscription: Subscription) => void;
};

const FeedRow = ({ subscription, onMenu }: FeedRowProps) => {
  const { feed, unread_count } = subscription;
  const title = subscriptionTitle(subscription);

  return (
    <Link
      to={`/app?feed_id=${feed.id}`}
      onContextMenu={(event) => onMenu(event, subscription)}
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
