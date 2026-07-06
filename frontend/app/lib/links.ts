import { useMutation, useQueryClient } from "@tanstack/react-query";

import { apiFetch } from "./api";

type UpdateLinkInput = {
  slug: string;
  isRead?: boolean;
  isStarred?: boolean;
  archived?: boolean;
};

/**
 * Updates a link's per-user state (read / starred / archived). Only the fields
 * provided are sent, matching the backend's partial PATCH semantics. On success
 * it refreshes the article river and the subscription unread counts, and
 * patches the cached article so the open page reflects the new state without a
 * refetch.
 */
export function useUpdateLink() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ slug, isRead, isStarred, archived }: UpdateLinkInput) =>
      apiFetch(`/v1/links/${slug}`, {
        method: "PATCH",
        body: JSON.stringify({
          ...(isRead !== undefined && { is_read: isRead }),
          ...(isStarred !== undefined && { is_starred: isStarred }),
          ...(archived !== undefined && { archived }),
        }),
      }),
    onSuccess: (_data, { slug, isRead, isStarred }) => {
      queryClient.invalidateQueries({ queryKey: ["subscriptions"] });
      queryClient.invalidateQueries({ queryKey: ["links"] });
      queryClient.setQueryData(
        ["article", slug],
        (
          old:
            | { link?: { is_read: boolean; is_starred: boolean } }
            | undefined,
        ) =>
          old?.link
            ? {
                ...old,
                link: {
                  ...old.link,
                  ...(isRead !== undefined && { is_read: isRead }),
                  ...(isStarred !== undefined && { is_starred: isStarred }),
                },
              }
            : old,
      );
    },
  });
}
