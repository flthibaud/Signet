import { useEffect, useState } from "react";

/**
 * Returns `value` delayed by `delay` ms, resetting the timer on every change.
 * Used to keep search-as-you-type from firing a request per keystroke.
 */
export default function useDebouncedValue<T>(value: T, delay = 250): T {
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);

  return debounced;
}
