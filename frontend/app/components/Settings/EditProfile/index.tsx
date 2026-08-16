import { useEffect, useRef, useState } from "react";
import Field from "~/components/Field";
import { User } from "lucide-react";

type EditProfileProps = {};

const EditProfile = ({}: EditProfileProps) => {
    const [objectURL, setObjectURL] = useState<string | null>(
        "/images/avatar.jpg"
    );
    const [name, setName] = useState<string>("");

    // Every URL.createObjectURL pins its blob in memory until it is revoked,
    // and picking a new file replaces the state without freeing the old one —
    // so the previous URL is kept here and released on the next pick and on
    // unmount. The initial value is a plain path, hence the null start.
    const previousBlobURL = useRef<string | null>(null);

    useEffect(
        () => () => {
            if (previousBlobURL.current) {
                URL.revokeObjectURL(previousBlobURL.current);
            }
        },
        []
    );

    const handleUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        if (!file) return;

        if (previousBlobURL.current) {
            URL.revokeObjectURL(previousBlobURL.current);
        }
        previousBlobURL.current = URL.createObjectURL(file);
        setObjectURL(previousBlobURL.current);
    };

    return (
        <form className="" action="" onSubmit={() => console.log("Submit")}>
            <div className="mb-8 h4 max-md:mb-6">Edit profile</div>
            <div className="mb-3 base2 font-semibold text-n-6 dark:text-n-1">
                Avatar
            </div>
            <div className="flex items-center mb-6">
                <div className="relative flex justify-center items-center shrink-0 w-28 h-28 mr-4 rounded-full overflow-hidden bg-n-2 dark:bg-n-6">
                    {objectURL !== null ? (
                        <img
                            className="w-full h-full object-cover rounded-full"
                            src={objectURL}
                            alt="Avatar"
                        />
                    ) : (
                        <User className="w-8 h-8 text-n-4 dark:text-n-1" />
                    )}
                </div>
                <div className="grow">
                    <div className="relative inline-flex mb-4">
                        <input
                            className="peer absolute inset-0 opacity-0 cursor-pointer"
                            type="file"
                            onChange={handleUpload}
                        />
                        <button className="btn-stroke-light peer-hover:bg-n-3 dark:peer-hover:bg-n-5">
                            Upload new image
                        </button>
                    </div>
                    <div className="caption1 text-n-4">
                        <p>At least 800x800 px recommended.</p>
                        <p>JPG or PNG and GIF is allowed</p>
                    </div>
                </div>
            </div>
            <Field
                className="mb-6"
                label="Name"
                placeholder="Username"
                icon={User}
                value={name}
                onChange={(e: any) => setName(e.target.value)}
                required
            />
            <button className="btn-blue w-full">Save changes</button>
        </form>
    );
};

export default EditProfile;
