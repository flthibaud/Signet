import { useEffect, useState } from "react";

/**
 * Tracks a CSS media query from JS. Reach for it only when a breakpoint has to
 * pick a *different component* rather than different classes — rendering both
 * and hiding one with `max-md:` would mount two copies of the same state.
 */
export default function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState(() => window.matchMedia(query).matches);

  useEffect(() => {
    const list = window.matchMedia(query);
    const handleChange = () => setMatches(list.matches);

    // The viewport may have moved between the initial render and this effect.
    handleChange();
    list.addEventListener("change", handleChange);
    return () => list.removeEventListener("change", handleChange);
  }, [query]);

  return matches;
}
