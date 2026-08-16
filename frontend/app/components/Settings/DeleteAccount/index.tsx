import { useState } from "react";
import Field from "~/components/Field";
import { Lock } from "lucide-react";

type DeleteAccountProps = {};

const DeleteAccount = ({}: DeleteAccountProps) => {
    const [password, setPassword] = useState<string>("");

    return (
        <form className="" action="" onSubmit={() => console.log("Submit")}>
            <div className="mb-8 h4">We’re sorry to see you go</div>
            <div className="mb-6 caption1 text-n-4">
                Warning: Deleting your account will permanently remove all of
                your data and cannot be undone. This includes your profile,
                feeds, and any other information associated with your
                account. Are you sure you want to proceed with deleting your
                account?
            </div>
            <Field
                className="mb-6"
                label="Your password"
                placeholder="Password"
                type="password"
                icon={Lock}
                value={password}
                onChange={(e: any) => setPassword(e.target.value)}
                required
            />
            <button className="btn-red w-full" disabled>
                Delete account
            </button>
        </form>
    );
};

export default DeleteAccount;
