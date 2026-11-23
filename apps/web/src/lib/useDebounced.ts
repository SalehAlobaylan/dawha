import { useEffect, useState } from "react";

export function useDebouncedKeyed<T>(value: T, delay: number, key: string): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(timer);
    // The key, not the value, is the dependency: content-addressed, so a
    // stable object identity cannot reschedule and a changed one must.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, delay]);
  return debounced;
}

/**
 * The scalar twin: the value is its own key, so identity-by-value is the
 * debounce contract and no key argument is needed.
 */
export function useDebouncedScalar<T extends string | number | null>(value: T, delay: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(timer);
  }, [value, delay]);
  return debounced;
}

