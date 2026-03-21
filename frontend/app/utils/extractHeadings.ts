import type { Heading } from "~/types/link";
import { slugify } from "~/utils/slugify";

export const extractHeadings = (markdown: string): Heading[] => {
  const headings: Heading[] = [];
  const lines = markdown.split("\n");
  for (const line of lines) {
    const match = line.match(/^(#{1,3})\s+(.+)/);
    if (match) {
      const raw = match[2].trim();
      // Strip markdown links: [text](url) -> text
      const text = raw.replace(/\[([^\]]+)\]\([^)]+\)/g, "$1");
      headings.push({ id: slugify(text), text, level: match[1].length });
    }
  }
  return headings;
}