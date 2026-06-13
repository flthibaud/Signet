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
