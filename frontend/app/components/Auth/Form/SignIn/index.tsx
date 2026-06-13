import Field from "~/components/Field";
import { Mail, Lock } from "lucide-react";
import { useForm, type SubmitHandler } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useNavigate } from "react-router";

import { applyApiError } from "~/lib/api";
import { useSignIn } from "~/lib/auth";
import { signInSchema, type SignInInputs } from "~/lib/validations/auth";

type SignInProps = {
  onForgotPassword: () => void;
  notice?: string | null;
};

const SignIn = ({ onForgotPassword, notice }: SignInProps) => {
  const navigate = useNavigate();
  const signIn = useSignIn();
  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<SignInInputs>({
    resolver: zodResolver(signInSchema),
  });

  const onSubmit: SubmitHandler<SignInInputs> = async (formData) => {
    try {
      await signIn.mutateAsync(formData);
      navigate("/app");
    } catch (error) {
      applyApiError(error, setError);
    }
  };

  return (
    <form onSubmit={handleSubmit(onSubmit)} noValidate>
      {notice && (
        <div className="mb-4 p-3 rounded-xl bg-green-500/10 text-green-600 text-sm font-semibold dark:text-green-400">
          {notice}
        </div>
      )}
      {errors.root && (
        <div className="mb-4 p-3 rounded-xl bg-red-500/10 text-red-500 text-sm font-semibold">
          {errors.root.message}
        </div>
      )}
      <Field
        className="mb-4"
        classInput="dark:bg-[#141718] dark:border-[#141718] dark:focus:bg-transparent"
        placeholder="Your email address"
        icon={Mail}
        type="email"
        autoComplete="email"
        error={errors.email?.message}
        {...register("email")}
      />
      <Field
        className="mb-2"
        classInput="dark:bg-[#141718] dark:border-[#141718] dark:focus:bg-transparent"
        placeholder="Password"
        icon={Lock}
        type="password"
        autoComplete="current-password"
        error={errors.password?.message}
        {...register("password")}
      />
      <button
        className="mb-6 base2 text-[#0084FF] transition-colors hover:text-[#0084FF]/90"
        type="button"
        onClick={onForgotPassword}
      >
        Forgot password?
      </button>
      <button
        className="btn btn-blue btn-large w-full"
        type="submit"
        disabled={isSubmitting}
      >
        {isSubmitting ? "Signing in..." : "Sign in"}
      </button>
    </form>
  );
};

export default SignIn;
