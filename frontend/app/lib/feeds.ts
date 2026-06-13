import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { apiFetch } from "./api";
import type { SubscriptionsResponse } from "~/types/subscription";
import type { SubscribeInputs } from "./validations/feed";

const subscriptionsKey = ["subscriptions"] as const;

/** Fetches the current user's feed subscriptions. */
export function useSubscriptions() {
  return useQuery({
    queryKey: subscriptionsKey,
    queryFn: () => apiFetch<SubscriptionsResponse>("/v1/subscriptions"),
  });
}

/**
 * Subscribes the current user to an RSS feed. The backend creates the feed if
 * needed and imports its articles in the background.
 */
export function useSubscribe() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (input: SubscribeInputs) =>
      apiFetch("/v1/subscriptions", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: subscriptionsKey });
    },
  });
}

/** Unsubscribes the current user from a feed. */
export function useUnsubscribe() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: number) =>
      apiFetch(`/v1/subscriptions/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: subscriptionsKey });
    },
  });
}
