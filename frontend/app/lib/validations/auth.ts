import { z } from "zod";

/**
 * Client-side validation schemas mirroring the backend rules
 * (see internal/data/users.go: ValidateUser / ValidatePasswordPlaintext).
 */

export const signInSchema = z.object({
  email: z.email("Please enter a valid email address."),
  password: z.string().min(1, "Password is required."),
});

export const signUpSchema = z
  .object({
    username: z
      .string()
      .min(1, "Username is required.")
      .max(500, "Username must be at most 500 characters."),
    email: z.email("Please enter a valid email address."),
    password: z
      .string()
      .min(8, "Password must be at least 8 characters.")
      .max(72, "Password must be at most 72 characters."),
    confirmPassword: z.string().min(1, "Please confirm your password."),
  })
  .refine((data) => data.password === data.confirmPassword, {
    message: "Passwords do not match.",
    path: ["confirmPassword"],
  });

export type SignInInputs = z.infer<typeof signInSchema>;
export type SignUpInputs = z.infer<typeof signUpSchema>;
