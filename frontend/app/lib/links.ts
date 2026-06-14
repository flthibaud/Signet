import { useMutation, useQueryClient } from "@tanstack/react-query";

import { apiFetch } from "./api";

type UpdateLinkInput = {
  slug: string;
  isRead: boolean;
};

/**
 * Updates a link's read state. On success it refreshes the article river and
 * the subscription unread counts, and patches the cached article so the open
 * page reflects the new state without a refetch.
 */
export function useUpdateLink() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ slug, isRead }: UpdateLinkInput) =>
      apiFetch(`/v1/links/${slug}`, {
        method: "PATCH",
        body: JSON.stringify({ is_read: isRead }),
      }),
    onSuccess: (_data, { slug, isRead }) => {
      queryClient.invalidateQueries({ queryKey: ["subscriptions"] });
      queryClient.invalidateQueries({ queryKey: ["links"] });
      queryClient.setQueryData(
        ["article", slug],
        (old: { link?: { is_read: boolean } } | undefined) =>
          old?.link ? { ...old, link: { ...old.link, is_read: isRead } } : old,
      );
    },
  });
}
