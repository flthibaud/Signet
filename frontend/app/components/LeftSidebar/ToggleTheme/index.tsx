import { twMerge } from "tailwind-merge";
import { Sun, Moon } from "lucide-react";
import useTheme from "~/hooks/useTheme";

type ToggleThemeProps = {
  visible?: boolean;
};

const ToggleTheme = ({ visible }: ToggleThemeProps) => {
  const [colorTheme, setTheme] = useTheme();

  const handleSetColorMode = (mode: "light" | "dark") => {
    setTheme(mode);
  };

  const items = [
    {
      title: "Light",
      icon: Sun,
      active: colorTheme === "light",
      onClick: () => handleSetColorMode("light"),
    },
    {
      title: "Dark",
      icon: Moon,
      active: colorTheme === "dark",
      onClick: () => handleSetColorMode("dark"),
    },
  ];

  return (
    <div
      className={`${
        visible &&
        `relative flex w-full p-1 bg-[#232627] rounded-xl before:absolute before:left-1 before:top-1 before:bottom-1 before:w-[calc(50%-0.25rem)] before:bg-[#141718] before:rounded-[0.625rem] before:transition-all ${
          colorTheme === "dark" && "before:translate-x-full"
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
