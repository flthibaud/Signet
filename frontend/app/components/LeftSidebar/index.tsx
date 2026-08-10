import { useState, useEffect } from "react";
import Modal from "~/components/Modal";
import Navigation from "./Navigation";
import Profile from "./Profile";
import ToggleTheme from "./ToggleTheme";
import Logo from "~/components/Logo";
import Search from "~/components/Search";
import FolderList from "./FolderList";

import {
  PanelLeftClose,
  PanelRightClose,
  House,
  Search as SearchIcon,
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
      color: "white",
      url: "/app",
    },
    {
      title: "Search",
      icon: SearchIcon,
      color: "white",
      onClick: () => setVisibleSearch(true),
    },
    {
      title: "Library",
      icon: Library,
      color: "white",
      url: "/app/library",
    },
    {
      title: "Feed",
      icon: Rss,
      color: "white",
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
          `fixed z-20 top-0 left-0 bottom-0 flex flex-col pt-30 px-0 bg-n-7`,
          isCollapsed ? "md:w-24 md:pb-38 w-16 pb-30 md:px-4" : "w-80 pb-58 px-4",
        )}
      >
        <div
          className={`absolute top-0 right-0 left-0 flex items-center h-30 pl-7 pr-6 ${
            isCollapsed ? "justify-center max-md:px-4" : "justify-between"
          }`}
        >
          {!isCollapsed && <Logo />}
          <button className="group tap-highlight-color" onClick={handleClick}>
            {isCollapsed ? (
              <PanelRightClose className="text-n-4 transition-colors group-hover:text-n-3" />
            ) : (
              <PanelLeftClose className="text-n-4 transition-colors group-hover:text-n-3" />
            )}
          </button>
        </div>
        {/* Navigation and the "New folder" button stay put; the folder tree is
            the only thing that scrolls, however long it gets. */}
        <div className="flex flex-col grow min-h-0">
          <Navigation visible={!isCollapsed} items={navigation} />
          <div
            className={`shrink-0 my-4 h-px bg-n-6 ${
              isCollapsed ? "-mx-4 max-md:mx-0" : "-mx-2 max-md:mx-0"
            }`}
          ></div>
          <FolderList visible={!isCollapsed} />
        </div>
        <div className="absolute left-0 bottom-0 right-0 pb-6 px-4 bg-n-7 before:absolute before:left-0 before:right-0 before:bottom-full before:h-10 before:bg-linear-to-t before:from-[#131617] before:to-[rgba(19,22,23,0)] before:pointer-events-none max-md:px-3">
          <Profile visible={!isCollapsed} />
          <ToggleTheme visible={!isCollapsed} />
        </div>
      </div>
      <Modal
        className="max-md:p-0!"
        classWrap="flex flex-col overflow-hidden h-[80dvh] rounded-3xl max-md:min-h-screen-ios max-md:rounded-none dark:shadow-[inset_0_0_0_0.0625rem_#232627,0_2rem_4rem_-1rem_rgba(0,0,0,0.33)] dark:max-md:shadow-none"
        classButtonClose="hidden max-md:flex max-md:absolute max-md:top-6 max-md:left-6 dark:text-n-1"
        classOverlay="max-md:bg-n-1"
        visible={visibleSearch}
        onClose={() => setVisibleSearch(false)}
      >
        <Search />
      </Modal>
    </>
  );
};

export default LeftSidebar;
