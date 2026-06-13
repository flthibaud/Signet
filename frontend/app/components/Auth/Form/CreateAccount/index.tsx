import { Mail, Lock, User } from "lucide-react";
import { useForm, type SubmitHandler } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";

import Field from "~/components/Field";
import { applyApiError } from "~/lib/api";
import { useSignUp } from "~/lib/auth";
import { signUpSchema, type SignUpInputs } from "~/lib/validations/auth";

type CreateAccountProps = {
  onSuccess: () => void;
};

const CreateAccount = ({ onSuccess }: CreateAccountProps) => {
  const signUp = useSignUp();
  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<SignUpInputs>({
    resolver: zodResolver(signUpSchema),
  });

  const onSubmit: SubmitHandler<SignUpInputs> = async (formData) => {
    try {
      await signUp.mutateAsync(formData);
      onSuccess();
    } catch (error) {
      applyApiError(error, setError);
    }
  };

  return (
    <form onSubmit={handleSubmit(onSubmit)} noValidate>
      {errors.root && (
        <div className="mb-4 p-3 rounded-xl bg-red-500/10 text-red-500 text-sm font-semibold">
          {errors.root.message}
        </div>
      )}
      <Field
        className="mb-4"
        classInput="dark:bg-[#141718] dark:border-[#141718] dark:focus:bg-transparent"
        placeholder="Username"
        icon={User}
        type="text"
        autoComplete="username"
        error={errors.username?.message}
        {...register("username")}
      />
      <Field
        className="mb-4"
        classInput="dark:bg-[#141718] dark:border-[#141718] dark:focus:bg-transparent"
        placeholder="Email"
        icon={Mail}
        type="email"
        autoComplete="email"
        error={errors.email?.message}
        {...register("email")}
      />
      <Field
        className="mb-6"
        classInput="dark:bg-[#141718] dark:border-[#141718] dark:focus:bg-transparent"
        placeholder="Password"
        icon={Lock}
        type="password"
        autoComplete="new-password"
        error={errors.password?.message}
        {...register("password")}
      />
      <Field
        className="mb-6"
        classInput="dark:bg-[#141718] dark:border-[#141718] dark:focus:bg-transparent"
        placeholder="Confirm Password"
        icon={Lock}
        type="password"
        autoComplete="new-password"
        error={errors.confirmPassword?.message}
        {...register("confirmPassword")}
      />
      <button
        className="btn btn-blue btn-large w-full mb-6"
        type="submit"
        disabled={isSubmitting}
      >
        {isSubmitting ? "Submitting..." : "Create account"}
      </button>
      <div className="text-center caption1 text-[#6C7275]">
        By creating an account, you agree to our{" "}
        <a
          className="text-[#343839] transition-colors hover:text-[#141718] dark:text-[#E8ECEF] dark:hover:text-[#FEFEFE]"
          href="/"
        >
          Terms of Service
        </a>{" "}
        and{" "}
        <a
          className="text-[#343839] transition-colors hover:text-[#141718] dark:text-[#E8ECEF] dark:hover:text-[#FEFEFE]"
          href="/"
        >
          Privacy & Cookie Statement
        </a>
        .
      </div>
    </form>
  );
};

export default CreateAccount;
