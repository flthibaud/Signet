import { useState } from "react";
import { Mail, Lock } from "lucide-react";
import Field from "../../../Field";

type CreateAccountProps = {};

const CreateAccount = ({}: CreateAccountProps) => {
  const [email, setEmail] = useState<string>("");
  const [password, setPassword] = useState<string>("");

  return (
    <form action="" onSubmit={() => console.log("Submit")}>
      <Field
        className="mb-4"
        classInput="dark:bg-[#141718] dark:border-[#141718] dark:focus:bg-transparent"
        placeholder="Email"
        icon={Mail}
        type="email"
        value={email}
        onChange={(e: any) => setEmail(e.target.value)}
        required
      />
      <Field
        className="mb-6"
        classInput="dark:bg-[#141718] dark:border-[#141718] dark:focus:bg-transparent"
        placeholder="Password"
        icon={Lock}
        type="password"
        value={password}
        onChange={(e: any) => setPassword(e.target.value)}
        required
      />
      <button className="btn btn-blue btn-large w-full mb-6" type="submit">
        Create Account
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
