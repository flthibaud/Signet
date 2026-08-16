import type { SettingsItem } from "../index";

type MenuProps = {
    buttons: SettingsItem[];
    value: SettingsItem;
    setValue: (value: SettingsItem) => void;
};

const Menu = ({ buttons, value, setValue }: MenuProps) => {
    const handleClick = (value: SettingsItem) => {
        setValue(value);
    };

    return buttons.map((button, index) => {
        const Icon = button.icon;

        return (
            <div className="mb-1" key={index}>
                {button.id === "delete-account" && (
                    <div className="h-0.25 my-3 bg-n-3 dark:bg-n-6"></div>
                )}
                <button
                    className={`group flex items-center w-full px-3.5 py-1.5 rounded-full border-2 border-transparent base2 font-semibold transition-colors hover:bg-n-2 tap-highlight-color dark:hover:bg-n-6 dark:hover:text-n-1 ${
                        button.id === "delete-account"
                            ? `!text-accent-1 ${
                                  value === button &&
                                  "!border-accent-1 !bg-n-1 dark:!bg-transparent"
                              }`
                            : `text-n-4 ${
                                  value === button &&
                                  "!border-primary-1 text-n-7 !bg-n-1 dark:!bg-transparent dark:text-n-1"
                              }`
                    }`}
                    onClick={() => handleClick(button)}
                >
                    <Icon className="shrink-0 w-4 h-4 mr-3 transition-colors" />
                    {button.title}
                </button>
            </div>
        );
    });
};

export default Menu;
