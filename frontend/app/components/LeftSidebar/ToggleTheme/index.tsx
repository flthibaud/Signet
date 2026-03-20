import { useState, useEffect } from "react";
import { twMerge } from "tailwind-merge";
import { Sun, Moon } from "lucide-react";

type ToggleThemeProps = {
  visible?: boolean;
};

const ToggleTheme = ({ visible }: ToggleThemeProps) => {
  const [colorMode, setColorMode] = useState<"light" | "dark">("light");

  useEffect(() => {
    const current = document.documentElement.getAttribute("data-theme");
    if (current === "dark" || current === "light") {
      setColorMode(current);
    }
  }, []);

  const handleSetColorMode = (mode: "light" | "dark") => {
    setColorMode(mode);
    document.documentElement.setAttribute("data-theme", mode);
  };

  const items = [
    {
      title: "Light",
      icon: Sun,
      active: colorMode === "light",
      onClick: () => handleSetColorMode("light"),
    },
    {
      title: "Dark",
      icon: Moon,
      active: colorMode === "dark",
      onClick: () => handleSetColorMode("dark"),
    },
  ];

  return (
    <div
      className={`${
        visible &&
        `relative flex w-full p-1 bg-[#232627] rounded-xl before:absolute before:left-1 before:top-1 before:bottom-1 before:w-[calc(50%-0.25rem)] before:bg-[#141718] before:rounded-[0.625rem] before:transition-all ${
          colorMode === "dark" && "before:translate-x-full"
        }`
      }`}
    >
      {items.map((item, index) => (
        <button
          className={twMerge(
            `relative z-1 group flex justify-center items-center ${
              !visible
                ? `flex md:h-16 rounded-xl bg-[#232627] w-8 h-8 md:w-16 md:mx-auto ${
                    item.active && "hidden"
                  }`
                : `h-10 basis-1/2 base2 font-semibold text-[#6C7275] transition-colors hover:text-[#FEFEFE] ${
                    item.active && "text-[#FEFEFE]"
                  }`
            }`,
          )}
          key={index}
          onClick={item.onClick}
        >
          <item.icon
            className={`stroke-[#6C7275] transition-colors group-hover:stroke-[#FEFEFE] ${
              visible && "mr-3"
            } ${item.active && visible && "stroke-[#FEFEFE]"}`}
          />
          {visible && item.title}
        </button>
      ))}
    </div>
  );
};

export default ToggleTheme;
