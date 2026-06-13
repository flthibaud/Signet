import type { FieldValues, Path, UseFormSetError } from "react-hook-form";

/**
 * Error thrown by `apiFetch` when the backend responds with a non-2xx status.
 *
 * The backend wraps every error in an `{ "error": ... }` envelope:
 *   - a plain string for generic errors (e.g. 401 "invalid authentication credentials")
 *   - a `{ field: message }` map for validation failures (422)
 *
 * `fieldErrors` is populated only in the validation case.
 */
export class ApiError extends Error {
  status: number;
  fieldErrors?: Record<string, string>;

  constructor(
    status: number,
    message: string,
    fieldErrors?: Record<string, string>,
  ) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.fieldErrors = fieldErrors;
  }
}

/**
 * Thin wrapper around `fetch` that sets JSON headers, forwards cookies and
 * normalizes the backend's error envelope into an `ApiError`.
 */
export async function apiFetch<T = unknown>(
  input: string,
  init?: RequestInit,
): Promise<T> {
  const res = await fetch(input, {
    credentials: "include",
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...init?.headers,
    },
  });

  const text = await res.text();
  const body = text ? safeJsonParse(text) : null;

  if (!res.ok) {
    const err = (body as { error?: unknown } | null)?.error;

    if (err && typeof err === "object") {
      throw new ApiError(
        res.status,
        "Please fix the errors below and try again.",
        err as Record<string, string>,
      );
    }

    throw new ApiError(
      res.status,
      typeof err === "string" ? capitalize(err) : "Something went wrong.",
    );
  }

  return body as T;
}

/**
 * Maps an `ApiError` onto a react-hook-form. Validation errors are attached to
 * their respective fields; anything else becomes a form-level `root` error.
 */
export function applyApiError<T extends FieldValues>(
  error: unknown,
  setError: UseFormSetError<T>,
) {
  if (error instanceof ApiError && error.fieldErrors) {
    for (const [field, message] of Object.entries(error.fieldErrors)) {
      setError(field as Path<T>, { message: capitalize(message) });
    }
    return;
  }

  setError("root", {
    message:
      error instanceof ApiError
        ? error.message
        : "Something went wrong. Please try again.",
  });
}

function safeJsonParse(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

function capitalize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}
