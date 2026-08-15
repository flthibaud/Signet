import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { ApiError, apiFetch } from "./api";
import type { OpmlImport, OpmlImportResponse } from "~/types/opml";

export const latestImportKey = ["opml", "import", "latest"] as const;

export function isImportRunning(imp: OpmlImport | undefined): boolean {
  return imp?.status === "running";
}

export function useImportOpml() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (file: File) =>
      apiFetch<OpmlImportResponse>("/v1/opml/import", {
        method: "POST",
        body: file,
        headers: { "Content-Type": "text/xml" },
      }),
    onSuccess: (data) => {
      queryClient.setQueryData(latestImportKey, data);
    },
  });
}

export function useLatestOpmlImport() {
  return useQuery({
    queryKey: latestImportKey,
    queryFn: async () => {
      try {
        return await apiFetch<OpmlImportResponse>("/v1/opml/imports/latest");
      } catch (error) {
        if (error instanceof ApiError && error.status === 404) return null;
        throw error;
      }
    },
    refetchInterval: (query) =>
      isImportRunning(query.state.data?.import) ? 2000 : false,
  });
}

/**
 * Downloads the user's subscriptions as an OPML file.
 */
export async function exportOpml(): Promise<void> {
  const res = await fetch("/v1/opml/export", { credentials: "include" });

  if (!res.ok) {
    throw new ApiError(res.status, "The export could not be generated.");
  }

  const blob = await res.blob();
  const url = URL.createObjectURL(blob);

  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filenameFrom(res) ?? "signet-subscriptions.opml";
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();

  URL.revokeObjectURL(url);
}

/** Reads the filename the server chose, so the date in it is the server's. */
function filenameFrom(res: Response): string | null {
  const disposition = res.headers.get("Content-Disposition");
  if (!disposition) return null;

  const match = disposition.match(/filename="?([^"]+)"?/);
  return match ? match[1] : null;
}
