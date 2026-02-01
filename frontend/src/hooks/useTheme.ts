import { useEffect, useState } from "react";

export const useTheme = () => {
  const [theme, setTheme] = useState<"light" | "dark">("light");

  useEffect(() => {
    const getTheme = () => {
      const currentTheme = document.documentElement.getAttribute("data-theme");
      return currentTheme === "dark" ? "dark" : "light";
    };

    setTheme(getTheme());

    const observer = new MutationObserver((mutations) => {
      mutations.forEach((mutation) => {
        if (
          mutation.type === "attributes" &&
          mutation.attributeName === "data-theme"
        ) {
          setTheme(getTheme());
        }
      });
    });

    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["data-theme"],
    });

    return () => observer.disconnect();
  }, []);

  return theme;
};