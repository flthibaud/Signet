import { useState, useEffect } from "react";
import Modal from "~/components/Modal";
import Navigation from "./Navigation";
import Profile from "./Profile";
import ToggleTheme from "./ToggleTheme";
import Logo from "~/components/Logo";

import {
  PanelLeftClose,
  PanelRightClose,
  House,
  Search,
  Library,
  Rss,
} from "lucide-react";
import { twMerge } from "tailwind-merge";

type LeftSidebarProps = {
  isCollapsed: boolean;
  setIsCollapsed?: React.Dispatch<React.SetStateAction<boolean>>;
};

const LeftSidebar = ({ isCollapsed, setIsCollapsed }: LeftSidebarProps) => {
  const [visibleSearch, setVisibleSearch] = useState<boolean>(false);

  useEffect(() => {
    const handleWindowKeyDown = (event: KeyboardEvent) => {
      if (event.metaKey && event.key === "f") {
        event.preventDefault();
        setVisibleSearch(true);
      }
    };

    window.addEventListener("keydown", handleWindowKeyDown);
    return () => {
      window.removeEventListener("keydown", handleWindowKeyDown);
    };
  }, []);

  const navigation = [
    {
      title: "Home",
      icon: House,
      color: "fill-accent-2",
      url: "/app",
    },
    {
      title: "Search",
      icon: Search,
      color: "fill-primary-2",
      onClick: () => setVisibleSearch(true),
    },
    {
      title: "Library",
      icon: Library,
      color: "fill-accent-4",
      url: "/app/library",
    },
    {
      title: "Feed",
      icon: Rss,
      color: "fill-accent-1",
      url: "/app/feed",
    },
  ];

  const handleClick = () => {
    if (setIsCollapsed) {
      setIsCollapsed(!isCollapsed);
    }
  };

  return (
    <>
      <div
        className={twMerge(
          `fixed z-20 top-0 left-0 bottom-0 flex flex-col pt-30 px-0 bg-[#141718]`,
          isCollapsed ? "md:w-24 md:pb-38 w-16 pb-30 md:px-4" : "w-80 pb-58 px-4",
        )}
      >
        <div
          className={`absolute top-0 right-0 left-0 flex items-center h-30 pl-7 pr-6 ${
            isCollapsed ? "justify-center md:px-4" : "justify-between"
          }`}
        >
          {!isCollapsed && <Logo />}
          <button className="group tap-highlight-color" onClick={handleClick}>
            {isCollapsed ? (
              <PanelRightClose className="fill-black transition-colors group-hover:fill-n-3" />
            ) : (
              <PanelLeftClose className="fill-black transition-colors group-hover:fill-n-3" />
            )}
          </button>
        </div>
        <div className="grow overflow-y-auto scroll-smooth scrollbar-none">
          <Navigation visible={!isCollapsed} items={navigation} />
          <div
            className={`my-4 h-px bg-n-6 ${
              isCollapsed ? "-mx-4 md:mx-0" : "-mx-2 md:mx-0"
            }`}
          ></div>
        </div>
        <div className="absolute left-0 bottom-0 right-0 pb-6 px-4 bg-n-7 before:absolute before:left-0 before:right-0 before:bottom-full before:h-10 before:bg-gradient-to-t before:from-[#131617] before:to-[rgba(19,22,23,0)] before:pointer-events-none md:px-3">
          <Profile visible={!isCollapsed} />
          <ToggleTheme visible={!isCollapsed} />
        </div>
      </div>
      <Modal
        className="md:p-0!"
        classWrap="md:min-h-screen-ios md:rounded-none dark:shadow-[inset_0_0_0_0.0625rem_#232627,0_2rem_4rem_-1rem_rgba(0,0,0,0.33)] dark:md:shadow-none"
        classButtonClose="hidden md:flex md:absolute md:top-6 md:left-6 dark:fill-n-1"
        classOverlay="md:bg-n-1"
        visible={visibleSearch}
        onClose={() => setVisibleSearch(false)}
      >
        <p>Search</p>
      </Modal>
    </>
  );
};

export default LeftSidebar;
