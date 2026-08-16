import { useQuery } from "@tanstack/react-query";

import { apiFetch } from "./api";
import type { AppConfigResponse } from "~/types/config";

const configKey = ["config"] as const;

/**
 * Reads the instance settings the guest pages need — currently only whether
 * this instance accepts new accounts. They come from the server's environment,
 * so they cannot change without a restart: never stale, never refetched.
 */
export function useAppConfig() {
  return useQuery({
    queryKey: configKey,
    queryFn: () => apiFetch<AppConfigResponse>("/v1/config"),
    staleTime: Infinity,
  });
}
