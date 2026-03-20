import type { LucideIcon } from "lucide-react";

export type NavigationItem = {
  title: string;
  icon: LucideIcon;
  color: string;
  url?: string;
  onClick?: () => void;
};
