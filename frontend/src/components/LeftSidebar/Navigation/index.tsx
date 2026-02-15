import { twMerge } from "tailwind-merge";
import type { LucideIcon } from "lucide-react";

type NavigationType = {
  title: string;
  icon: LucideIcon;
  color: string;
  url?: string;
  onClick?: () => void;
};

type NavigationProps = {
  visible?: boolean;
  items: NavigationType[];
};

const Navigation = ({ visible, items }: NavigationProps) => {
  // const pathname = usePathname();
  const pathname = "/app"; // Placeholder for pathname

  return (
    <div className={`${!visible && "px-2"}`}>
      {items.map((item, index) =>
        item.url ? (
          <a
            className={twMerge(
              `flex items-center h-12 base2 font-semibold text-[#E8ECEF]/75 rounded-lg transition-colors hover:text-[#FEFEFE] ${
                pathname === item.url &&
                "text-[#FEFEFE] bg-gradient-to-l from-[#323337] to-[rgba(70,79,111,0.3)] shadow-[inset_0px_0.0625rem_0_rgba(255,255,255,0.05),0_0.25rem_0.5rem_0_rgba(0,0,0,0.1)]"
              } ${visible ? "px-5" : "px-3"}`,
            )}
            href={item.url}
            key={index}
          >
            <item.icon className={item.color} />
            {visible && <div className="ml-5">{item.title}</div>}
          </a>
        ) : (
          <button
            className={`flex items-center w-full h-12 base2 font-semibold text-[#E8ECEF]/75 rounded-lg transition-colors hover:text-[#FEFEFE] ${
              visible ? "px-5" : "px-3"
            }`}
            key={index}
            onClick={item.onClick}
          >
            <item.icon className={item.color} />
            {visible && <div className="ml-5">{item.title}</div>}
            {item.title === "Search" && visible && (
              <div className="ml-auto px-2 rounded-md bg-[#6C7275]/50 caption1 font-semibold text-[#E8ECEF]">
                ⌘ F
              </div>
            )}
          </button>
        ),
      )}
    </div>
  );
};

export default Navigation;
