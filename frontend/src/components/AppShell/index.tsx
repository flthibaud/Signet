import { useState, type ReactNode } from "react";
import { twMerge } from "tailwind-merge";
import LeftSidebar from "@/components/LeftSidebar";
import Burger from "@/components/Burger";

type AppShellProps = {
  children: ReactNode;
};

const AppShell = ({ children }: AppShellProps) => {
  const [isCollapsed, setIsCollapsed] = useState(true);

  return (
    <div
      className={`md:pr-6 pr-0 bg-[#141718] ${
        isCollapsed ? "md:pl-24 pl-0" : "md:pl-80 pl-24"
      }`}
    >
      <LeftSidebar
        isCollapsed={isCollapsed}
        setIsCollapsed={setIsCollapsed}
      />
      <div className="flex md:py-6 py-0 h-screen h-screen-ios">
        <div className="relative flex grow max-w-full bg-[#FEFEFE] md:rounded-[1.25rem] rounded-none dark:bg-[#232627]">
          <div className="relative flex flex-col grow max-w-full md:pt-18">
            <Burger
              className={!isCollapsed ? "hidden" : ""}
              onClick={() => setIsCollapsed(!isCollapsed)}
            />
            {children}
          </div>
        </div>
      </div>
    </div>
  );
};

export default AppShell;
