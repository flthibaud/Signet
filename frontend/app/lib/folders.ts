import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { apiFetch } from "./api";
import type { Folder, FoldersResponse } from "~/types/subscription";

const foldersKey = ["folders"] as const;
const subscriptionsKey = ["subscriptions"] as const;

/**
 * Fetches the current user's folders. Folders are the source of truth for the
 * sidebar's list: an empty one still has to show up, and subscriptions alone
 * would not reveal it.
 */
export function useFolders() {
  return useQuery({
    queryKey: foldersKey,
    queryFn: () => apiFetch<FoldersResponse>("/v1/folders"),
  });
}

/**
 * Invalidates both lists. Filing a feed changes the folders shown in the
 * sidebar and the subscriptions shown on the feed page.
 */
function useFolderMutation<TVariables>(
  mutationFn: (variables: TVariables) => Promise<unknown>,
) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: foldersKey });
      queryClient.invalidateQueries({ queryKey: subscriptionsKey });
    },
  });
}

export function useCreateFolder() {
  return useFolderMutation((name: string) =>
    apiFetch<{ folder: Folder }>("/v1/folders", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
  );
}

export function useRenameFolder() {
  return useFolderMutation(({ id, name }: { id: number; name: string }) =>
    apiFetch<{ folder: Folder }>(`/v1/folders/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ name }),
    }),
  );
}

/** Deleting a folder unfiles its feeds; it never unsubscribes. */
export function useDeleteFolder() {
  return useFolderMutation((id: number) =>
    apiFetch(`/v1/folders/${id}`, { method: "DELETE" }),
  );
}

/** Files a subscription into a folder, or unfiles it when folderId is null. */
export function useSetSubscriptionFolder() {
  return useFolderMutation(
    ({ id, folderId }: { id: number; folderId: number | null }) =>
      apiFetch(`/v1/subscriptions/${id}`, {
        method: "PATCH",
        body: JSON.stringify({ folder_id: folderId }),
      }),
  );
}
