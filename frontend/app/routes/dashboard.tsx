import { useState } from "react";
import { Outlet } from "react-router";
import LeftSidebar from "~/components/LeftSidebar";
import Burger from "~/components/Burger";

export default function DashboardLayout() {
  const [isCollapsed, setIsCollapsed] = useState(false);

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
          <div className="relative flex flex-col grow max-w-full overflow-hidden">
            <Burger
              className={!isCollapsed ? "hidden" : ""}
              onClick={() => setIsCollapsed(!isCollapsed)}
            />
            <Outlet />
          </div>
        </div>
      </div>
    </div>
  );
}
