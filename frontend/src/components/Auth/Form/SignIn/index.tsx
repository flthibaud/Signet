import { useState } from "react";
import Field from "../../../Field";
import { Mail, Lock } from 'lucide-react';

type SignInProps = {
  onClick: () => void;
};

const SignIn = ({ onClick }: SignInProps) => {
  const [name, setName] = useState<string>("");
  const [password, setPassword] = useState<string>("");

  return (
    <form action="" onSubmit={() => console.log("Submit")}>
      <Field
        className="mb-4"
        classInput="dark:bg-[#141718] dark:border-[#141718] dark:focus:bg-transparent"
        placeholder="Username or email"
        icon={Mail}
        value={name}
        onChange={(e: any) => setName(e.target.value)}
        required
      />
      <Field
        className="mb-2"
        classInput="dark:bg-[#141718] dark:border-[#141718] dark:focus:bg-transparent"
        placeholder="Password"
        icon={Lock}
        type="password"
        value={password}
        onChange={(e: any) => setPassword(e.target.value)}
        required
      />
      <button
        className="mb-6 base2 text-[#0084FF] transition-colors hover:text-[#0084FF]/90"
        type="button"
        onClick={onClick}
      >
        Forgot password?
      </button>
      <button className="btn btn-blue btn-large w-full" type="submit">
        Sign in with Brainwave
      </button>
    </form>
  );
};

export default SignIn;
