import { useMutation } from "@tanstack/react-query";

import { apiFetch } from "./api";
import type { SignInInputs, SignUpInputs } from "./validations/auth";

/**
 * Authenticates the user. On success the backend sets an HttpOnly `auth_token`
 * cookie, so the returned payload is not needed by the caller.
 */
export function useSignIn() {
  return useMutation({
    mutationFn: (input: SignInInputs) =>
      apiFetch("/v1/tokens/authentication", {
        method: "POST",
        body: JSON.stringify(input),
      }),
  });
}

/**
 * Logs the user out of this device: the backend invalidates the token this
 * browser authenticated with — the user's other sessions stay signed in — and
 * expires the cookie. A full page navigation then clears all client-side state
 * and lets the server redirect to the guest pages.
 */
export function useSignOut() {
  return useMutation({
    mutationFn: () =>
      apiFetch("/v1/tokens/authentication", { method: "DELETE" }),
    onSuccess: () => {
      window.location.assign("/auth");
    },
  });
}

/**
 * Registers a new user. The backend does not return a session here, so the
 * caller is expected to send the user to the sign-in form afterwards.
 */
export function useSignUp() {
  return useMutation({
    mutationFn: ({ username, email, password }: SignUpInputs) =>
      apiFetch("/v1/users", {
        method: "POST",
        body: JSON.stringify({ username, email, password }),
      }),
  });
}
