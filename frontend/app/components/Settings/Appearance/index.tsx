import { useState } from "react";
import { twMerge } from "tailwind-merge";
import Select from "~/components/Select";
import useTheme from "~/hooks/useTheme";

const languages = [
    {
        id: "0",
        title: "English (United States)",
    },
    {
        id: "1",
        title: "French",
    },
    {
        id: "2",
        title: "Ukrainian",
    },
];

type AppearanceProps = {};

const Appearance = ({}: AppearanceProps) => {
    const [colorTheme, setTheme] = useTheme();
    const [language, setLanguage] = useState<any>(languages[0]);

    const handleSetColorMode = (mode: "light" | "dark") => {
        setTheme(mode);
    };

    const items = [
        {
            title: "Light mode",
            image: "/images/theme-light.svg",
            active: colorTheme === "light",
            onClick: () => handleSetColorMode("light"),
        },
        {
            title: "Dark mode",
            image: "/images/theme-dark.svg",
            active: colorTheme === "dark",
            onClick: () => handleSetColorMode("dark"),
        },
    ];

    return (
        <>
            <div className="mb-8 h4">Appearance</div>
            <div className="mb-5 base1 font-semibold">Appearance</div>
            <div className="flex mb-8 pr-12 space-x-8 max-md:pr-0">
                {items.map((item, index) => (
                    <button
                        className={twMerge(
                            `basis-1/2 p-3 border-4 border-transparent bg-n-2 rounded-2xl text-left transition-colors dark:bg-n-6 dark:text-n-3/50 ${
                                item.active &&
                                "bg-transparent border-primary-1 text-n-6/50 dark:text-n-1 dark:bg-transparent"
                            }`
                        )}
                        key={index}
                        onClick={item.onClick}
                    >
                        <div className="mb-3">
                            <img
                                className="w-full rounded-xl"
                                src={item.image}
                                width={128}
                                height={80}
                                alt=""
                            />
                        </div>
                        {item.title}
                    </button>
                ))}
            </div>
            <div className="flex items-center max-md:block">
                <div className="mr-auto base1 font-semibold max-md:mb-4">
                    Primary language
                </div>
                <Select
                    className="min-w-52.5"
                    classButton="bg-n-3/75 dark:bg-n-6 dark:shadow-[inset_0_0_0_0.0625rem_#232627]"
                    items={languages}
                    value={language}
                    onChange={setLanguage}
                    up
                />
            </div>
        </>
    );
};

export default Appearance;
