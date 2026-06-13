import { z } from "zod";

/**
 * Mirrors the backend rules in cmd/api/subscriptions.go:
 * url is required, must be a valid URL and at most 2048 characters.
 */
export const subscribeSchema = z.object({
  url: z
    .url("Please enter a valid URL.")
    .max(2048, "URL must be at most 2048 characters."),
});

export type SubscribeInputs = z.infer<typeof subscribeSchema>;
