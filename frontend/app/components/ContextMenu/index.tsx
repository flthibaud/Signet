import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { LucideIcon } from "lucide-react";

export type ContextMenuItem =
  | { type: "separator" }
  | { type: "label"; label: string }
  | {
      type?: "item";
      label: string;
      icon?: LucideIcon;
      danger?: boolean;
      selected?: boolean;
      onSelect: () => void;
    };

export type ContextMenuPosition = { x: number; y: number };

type ContextMenuProps = {
  position: ContextMenuPosition | null;
  items: ContextMenuItem[];
  onClose: () => void;
};

const MARGIN = 8;

/**
 * Menu opened by a right click, positioned at the pointer. Rendered fixed
 * rather than inside the row it belongs to: the sidebar's folder list clips its
 * overflow, and a menu near the bottom would be cut off.
 */
const ContextMenu = ({ position, items, onClose }: ContextMenuProps) => {
  const ref = useRef<HTMLDivElement>(null);
  const [offset, setOffset] = useState<ContextMenuPosition | null>(null);

  // Measure once mounted, then flip the menu back inside the viewport.
  useLayoutEffect(() => {
    if (!position || !ref.current) {
      setOffset(null);
      return;
    }

    const { width, height } = ref.current.getBoundingClientRect();
    setOffset({
      x: Math.min(position.x, window.innerWidth - width - MARGIN),
      y: Math.min(position.y, window.innerHeight - height - MARGIN),
    });
  }, [position, items.length]);

  useEffect(() => {
    if (!position) return;

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    const onPointerDown = (event: MouseEvent) => {
      if (!ref.current?.contains(event.target as Node)) onClose();
    };

    window.addEventListener("keydown", onKeyDown);
    window.addEventListener("mousedown", onPointerDown);
    window.addEventListener("resize", onClose);
    // Capture phase: the list scrolls in its own container, which does not
    // bubble a scroll event to the window.
    window.addEventListener("scroll", onClose, true);

    return () => {
      window.removeEventListener("keydown", onKeyDown);
      window.removeEventListener("mousedown", onPointerDown);
      window.removeEventListener("resize", onClose);
      window.removeEventListener("scroll", onClose, true);
    };
  }, [position, onClose]);

  if (!position) return null;

  return (
    <div
      ref={ref}
      role="menu"
      className="fixed z-40 min-w-52 max-h-80 overflow-y-auto scrollbar-none p-1.5 rounded-xl bg-n-6 shadow-[0_0_1rem_0.25rem_rgba(0,0,0,0.12),0_2rem_2rem_-1.5rem_rgba(0,0,0,0.2),inset_0_0_0_0.0625rem_#343839]"
      style={{
        left: offset?.x ?? position.x,
        top: offset?.y ?? position.y,
        // Hidden for the frame it takes to measure, so it never flashes at the
        // unclamped position.
        visibility: offset ? "visible" : "hidden",
      }}
    >
      {items.map((item, index) => {
        if ("type" in item && item.type === "separator") {
          return <div key={index} className="my-1.5 -mx-1.5 h-px bg-n-5" />;
        }

        if ("type" in item && item.type === "label") {
          return (
            <div key={index} className="px-3 py-1 caption2 text-n-4">
              {item.label}
            </div>
          );
        }

        const entry = item as Extract<ContextMenuItem, { onSelect: () => void }>;
        const Icon = entry.icon;

        return (
          <button
            key={index}
            type="button"
            role="menuitem"
            className={`flex items-center w-full px-3 py-2 rounded-lg base2 text-left transition-colors hover:cursor-pointer ${
              entry.danger
                ? "text-n-3/75 hover:bg-red-500/10 hover:text-red-500"
                : "text-n-3/75 hover:bg-n-5 hover:text-n-1"
            } ${entry.selected && "text-n-1"}`}
            onClick={() => {
              entry.onSelect();
              onClose();
            }}
          >
            {Icon && <Icon size={16} className="shrink-0 mr-3 text-n-4" />}
            <span className="truncate">{entry.label}</span>
            {entry.selected && (
              <span className="ml-auto pl-3 shrink-0 text-primary-1">•</span>
            )}
          </button>
        );
      })}
    </div>
  );
};

export default ContextMenu;
