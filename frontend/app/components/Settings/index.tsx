import { useState } from "react";
import type { LucideIcon } from "lucide-react";
import useMediaQuery from "~/hooks/useMediaQuery";
import Select from "~/components/Select";
import Menu from "./Menu";
import EditProfile from "./EditProfile";
import Password from "./Password";
import Sessions from "./Sessions";
import Appearance from "./Appearance";
import DeleteAccount from "./DeleteAccount";

export type SettingsItem = {
    id: string;
    title: string;
    icon: LucideIcon;
};

type SettingsProps = {
    items: SettingsItem[];
    activeItem?: number;
};

const Settings = ({ items, activeItem }: SettingsProps) => {
    const [active, setActive] = useState<SettingsItem>(
        items[activeItem || 0]
    );

    const isMobile = useMediaQuery("(max-width: 767px)");

    return (
        <div className="p-12 max-lg:px-8 max-md:pt-16 max-md:px-5 max-md:pb-8">
            <div className="flex max-md:block">
                {isMobile ? (
                    <Select
                        className="mb-6"
                        classButton="dark:bg-transparent"
                        classArrow="dark:text-n-4"
                        items={items}
                        value={active}
                        onChange={setActive}
                    />
                ) : (
                    <div className="shrink-0 w-[13.25rem]">
                        <Menu
                            value={active}
                            setValue={setActive}
                            buttons={items}
                        />
                    </div>
                )}
                <div className="grow pl-12 max-md:pl-0">
                    {active.id === "edit-profile" && <EditProfile />}
                    {active.id === "password" && <Password />}
                    {active.id === "sessions" && <Sessions />}
                    {active.id === "appearance" && <Appearance />}
                    {active.id === "delete-account" && <DeleteAccount />}
                </div>
            </div>
        </div>
    );
};

export default Settings;
