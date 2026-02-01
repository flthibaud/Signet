import { useState } from "react";
import Field from "../../../Field";
import { ArrowLeft, Mail } from "lucide-react";

type ForgotPasswordProps = {
  onClick: () => void;
};

const ForgotPassword = ({ onClick }: ForgotPasswordProps) => {
  const [email, setEmail] = useState<string>("");

  return (
    <>
      <button
        className="group flex items-center mb-8 h5 text-black dark:text-[#FEFEFE]"
        onClick={onClick}
      >
        <ArrowLeft className="mr-4 text-black transition-transform group-hover:-translate-x-1 dark:text-[#FEFEFE]" />
        Reset your password
      </button>
      <form action="" onSubmit={() => console.log("Submit")}>
        <Field
          className="mb-6"
          classInput="dark:bg-[#141718] dark:border-[#141718] dark:focus:bg-transparent"
          placeholder="Email"
          icon={Mail}
          type="email"
          value={email}
          onChange={(e: any) => setEmail(e.target.value)}
          required
        />
        <button className="btn btn-blue btn-large w-full mb-6" type="submit">
          Reset password
        </button>
      </form>
    </>
  );
};

export default ForgotPassword;
