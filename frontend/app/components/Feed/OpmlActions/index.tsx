import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import {
  AlertCircle,
  ChevronDown,
  Download,
  Loader2,
  Upload,
} from "lucide-react";

import { ApiError } from "~/lib/api";
import { exportOpml, isImportRunning, useImportOpml, useLatestOpmlImport } from "~/lib/opml";
import type { OpmlImport } from "~/types/opml";

const OpmlActions = () => {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const queryClient = useQueryClient();

  const importOpml = useImportOpml();
  const { data } = useLatestOpmlImport();
  const [error, setError] = useState<string | null>(null);
  const [isExporting, setIsExporting] = useState(false);
  const [showFailures, setShowFailures] = useState(false);

  const job = data?.import;
  const running = isImportRunning(job);

  // Subscriptions appear as the import goes, so refresh the list under it —
  // once per step, not on every render.
  useEffect(() => {
    if (!job) return;
    queryClient.invalidateQueries({ queryKey: ["subscriptions"] });
  }, [job?.processed, job?.status, queryClient]);

  const onFileChosen = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    // Reset straight away so choosing the same file twice fires again.
    event.target.value = "";
    if (!file) return;

    setError(null);
    setShowFailures(false);

    try {
      await importOpml.mutateAsync(file);
    } catch (err) {
      setError(
        err instanceof ApiError
          ? Object.values(err.fieldErrors ?? {})[0] ?? err.message
          : "The file could not be imported.",
      );
    }
  };

  const onExport = async () => {
    setError(null);
    setIsExporting(true);
    try {
      await exportOpml();
    } catch {
      setError("The export could not be generated.");
    } finally {
      setIsExporting(false);
    }
  };

  return (
    <div className="mx-auto mt-6 max-w-126">
      <div className="flex items-center justify-center gap-2 text-sm">
        <input
          ref={fileInputRef}
          type="file"
          accept=".opml,.xml,text/xml,application/xml"
          className="hidden"
          onChange={onFileChosen}
        />
        <button
          type="button"
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg font-medium text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 disabled:opacity-50 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-white"
          onClick={() => fileInputRef.current?.click()}
          disabled={importOpml.isPending || running}
        >
          {importOpml.isPending ? (
            <Loader2 size={16} className="animate-spin" />
          ) : (
            <Upload size={16} />
          )}
          Import OPML
        </button>

        <span className="text-gray-300 dark:text-gray-600">·</span>

        <button
          type="button"
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg font-medium text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 disabled:opacity-50 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-white"
          onClick={onExport}
          disabled={isExporting}
        >
          {isExporting ? (
            <Loader2 size={16} className="animate-spin" />
          ) : (
            <Download size={16} />
          )}
          Export OPML
        </button>
      </div>

      {error && (
        <div className="mt-3 p-3 rounded-xl bg-red-500/10 text-red-500 text-sm font-semibold">
          {error}
        </div>
      )}

      {job && (
        <ImportReport
          job={job}
          running={running}
          showFailures={showFailures}
          onToggleFailures={() => setShowFailures((shown) => !shown)}
        />
      )}
    </div>
  );
};

type ImportReportProps = {
  job: OpmlImport;
  running: boolean;
  showFailures: boolean;
  onToggleFailures: () => void;
};

const ImportReport = ({
  job,
  running,
  showFailures,
  onToggleFailures,
}: ImportReportProps) => {
  const failures = job.results.filter((r) => r.status === "failed");
  const progress = job.total > 0 ? (job.processed / job.total) * 100 : 0;

  return (
    <div className="mt-4 p-4 rounded-xl bg-gray-50 dark:bg-gray-800/50">
      {running ? (
        <>
          <div className="flex items-center justify-between text-sm text-gray-600 dark:text-gray-300">
            <span className="flex items-center gap-2">
              <Loader2 size={14} className="animate-spin" />
              Importing subscriptions…
            </span>
            <span className="tabular-nums">
              {job.processed} / {job.total}
            </span>
          </div>
          <div className="mt-2 h-1.5 w-full rounded-full bg-gray-200 dark:bg-gray-700 overflow-hidden">
            <div
              className="h-full rounded-full bg-primary-1 transition-[width] duration-300"
              style={{ width: `${progress}%` }}
            />
          </div>
        </>
      ) : (
        <div className="flex items-center justify-between gap-3 text-sm">
          <span className="text-gray-600 dark:text-gray-300">
            <strong className="text-gray-900 dark:text-white">
              {job.imported}
            </strong>{" "}
            imported
            {job.skipped > 0 && <> · {job.skipped} already subscribed</>}
            {job.failed > 0 && <> · {job.failed} failed</>}
          </span>

          {job.status === "interrupted" && (
            <span className="flex items-center gap-1.5 text-xs text-amber-600 dark:text-amber-400">
              <AlertCircle size={14} />
              Stopped early
            </span>
          )}
        </div>
      )}

      {failures.length > 0 && !running && (
        <>
          <button
            type="button"
            onClick={onToggleFailures}
            className="mt-2 flex items-center gap-1 text-xs font-medium text-gray-400 transition-colors hover:text-gray-600 dark:hover:text-gray-200"
          >
            <ChevronDown
              size={14}
              className={`transition-transform ${showFailures ? "rotate-180" : ""}`}
            />
            {showFailures ? "Hide" : "Show"} what failed
          </button>

          {showFailures && (
            <ul className="mt-2 flex flex-col gap-1 text-xs">
              {failures.map((failure) => (
                <li
                  key={failure.url}
                  className="flex items-baseline justify-between gap-3 text-gray-500 dark:text-gray-400"
                >
                  <span className="truncate">{failure.title || failure.url}</span>
                  <span className="shrink-0 text-gray-400 dark:text-gray-500">
                    {failure.reason}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </>
      )}
    </div>
  );
};

export default OpmlActions;
