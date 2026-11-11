import { useEffect, useSyncExternalStore } from "react";

type Theme = "light" | "dark";

const STORAGE_KEY = "theme";

// The theme is a module-level store rather than one useState per caller: the
// sidebar toggle and the Appearance panel of the settings modal are mounted at
// the same time, and with a copy of the state each, switching the theme in one
// left the other highlighting the old value until it remounted.
let current: Theme | null = null;
const listeners = new Set<() => void>();

function read(): Theme {
  const stored = localStorage.getItem(STORAGE_KEY);
  if (stored === "light" || stored === "dark") return stored;
  return window.matchMedia("(prefers-color-scheme: dark)").matches
    ? "dark"
    : "light";
}

// Resolved on first read, not at import time: SPA mode prerenders the shell at
// build time, where localStorage and matchMedia do not exist.
function getSnapshot(): Theme {
  if (current === null) current = read();
  return current;
}

function getServerSnapshot(): Theme {
  return "light";
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function setTheme(theme: Theme) {
  if (theme === current) return;
  current = theme;
  localStorage.setItem(STORAGE_KEY, theme);
  listeners.forEach((listener) => listener());
}

export default function useTheme() {
  const theme = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
  }, [theme]);

  return [theme, setTheme] as const;
}
