import Field from "../../../Field";
import { Mail, Lock } from 'lucide-react';
import { useForm, type SubmitHandler } from "react-hook-form"

type SignInProps = {
  onClick: () => void;
};

type SignInInputs = {
  email: string
  password: string
}

const SignIn = ({ onClick }: SignInProps) => {
  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<SignInInputs>()

  const onSubmit: SubmitHandler<SignInInputs> = async (formData) => {
    const res = await fetch("/v1/tokens/authentication", { 
      method: "POST", 
      body: JSON.stringify(formData) 
    });
    
    if (res.ok) window.location.href = "/app";
  };

  return (
    <form action="" onSubmit={handleSubmit(onSubmit)}>
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
        {...register("email", {required: true})}
        required
        autoComplete="email"
      />
      <Field
        className="mb-2"
        classInput="dark:bg-[#141718] dark:border-[#141718] dark:focus:bg-transparent"
        placeholder="Password"
        icon={Lock}
        type="password"
        {...register("password", {required: true})}
        required
        autoComplete="current-password"
      />
      <button
        className="mb-6 base2 text-[#0084FF] transition-colors hover:text-[#0084FF]/90"
        type="button"
        onClick={onClick}
      >
        Forgot password?
      </button>
      <button className="btn btn-blue btn-large w-full" type="submit" disabled={isSubmitting}>
        {isSubmitting ? "Signing in..." : "Sign in"}
      </button>
    </form>
  );
};

export default SignIn;
