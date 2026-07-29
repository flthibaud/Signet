export type OpmlImportStatus = "running" | "completed" | "interrupted";

export type OpmlEntryStatus = "imported" | "skipped" | "failed";

export interface OpmlImportResult {
  url: string;
  title?: string;
  status: OpmlEntryStatus;
  reason?: string;
}

export interface OpmlImport {
  id: string;
  status: OpmlImportStatus;
  total: number;
  processed: number;
  imported: number;
  skipped: number;
  failed: number;
  results: OpmlImportResult[];
  created_at: string;
  updated_at: string;
  finished_at: string | null;
}

export interface OpmlImportResponse {
  import: OpmlImport;
}
