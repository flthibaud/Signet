import { useState, useEffect, useCallback } from "react";
import type { Heading } from "~/types/link";

export default function TableOfContents({
  headings,
  scrollContainer,
}: {
  headings: Heading[];
  scrollContainer: React.RefObject<HTMLDivElement | null>;
}) {
  const [activeId, setActiveId] = useState<string>("");

  const handleClick = useCallback(
    (id: string) => {
      const el = scrollContainer.current?.querySelector(`#${CSS.escape(id)}`);
      if (el) {
        el.scrollIntoView({ behavior: "smooth", block: "start" });
      }
    },
    [scrollContainer],
  );

  useEffect(() => {
    const container = scrollContainer.current;
    if (!container || headings.length === 0) return;

    const handleScroll = () => {
      const headingElements = headings
        .map((h) => container.querySelector(`#${CSS.escape(h.id)}`))
        .filter(Boolean) as HTMLElement[];

      let current = headings[0]?.id ?? "";
      for (const el of headingElements) {
        const rect = el.getBoundingClientRect();
        const containerRect = container.getBoundingClientRect();
        if (rect.top - containerRect.top <= 80) {
          current = el.id;
        }
      }
      setActiveId(current);
    };

    container.addEventListener("scroll", handleScroll, { passive: true });
    handleScroll();
    return () => container.removeEventListener("scroll", handleScroll);
  }, [headings, scrollContainer]);

  if (headings.length === 0) return null;

  return (
    <nav className="hidden xl:block absolute left-0 top-0 bottom-0 w-62 py-12 px-6 overflow-y-auto z-10">
      <div className="sticky top-0">
        <p className="text-xs font-semibold uppercase tracking-wider text-neutral-400 mb-3">
          Sommaire
        </p>
        <ul className="space-y-1.5">
          {headings.map((h) => (
            <li key={h.id}>
              <button
                onClick={() => handleClick(h.id)}
                className={`text-left text-sm leading-snug transition-colors w-full truncate ${
                  h.level === 2 ? "pl-3" : h.level === 3 ? "pl-6" : ""
                } ${
                  activeId === h.id
                    ? "text-[#0084FF] font-medium"
                    : "text-neutral-500 hover:text-neutral-300"
                }`}
              >
                {h.text}
              </button>
            </li>
          ))}
        </ul>
      </div>
    </nav>
  );
}