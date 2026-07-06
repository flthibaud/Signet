import { isValidElement, useEffect, useMemo, useRef, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams } from "react-router";
import Markdown from "react-markdown";
import { extractHeadings } from "~/utils/extractHeadings";
import { slugify } from "~/utils/slugify";
import { apiFetch } from "~/lib/api";
import { useUpdateLink } from "~/lib/links";
import type { LinkDetail } from "~/types/link";

import TableOfContents from "~/components/Links/TableOfContents";

function getTextContent(node: ReactNode): string {
  if (typeof node === "string") return node;
  if (typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(getTextContent).join("");
  if (isValidElement<{ children?: ReactNode }>(node)) {
    return getTextContent(node.props.children);
  }
  return "";
}

export default function ArticlePage() {
  const { slug } = useParams();
  const scrollRef = useRef<HTMLDivElement>(null);
  const { data: article, isLoading, error } = useQuery({
    queryKey: ["article", slug],
    queryFn: () => apiFetch<{ link: LinkDetail }>(`/v1/links/${slug}`),
  });

  const link = article?.link;
  const headings = useMemo(
    () => (link?.text_content ? extractHeadings(link.text_content) : []),
    [link?.text_content],
  );

  // Reading progress tracking. The scroll position maps to a 0..1 progress
  // value; we keep the maximum reached in this session (scrolling back up
  // doesn't regress it), persist it periodically and when leaving the page,
  // and flip is_read once the reader gets close enough to the end.
  const updateLink = useUpdateLink();
  const progressRef = useRef(0); // max progress reached
  const lastSavedRef = useRef(0); // last value sent to the API
  const markedReadRef = useRef(false);
  const restoredRef = useRef(false);

  // Restore the saved position once the article content is rendered.
  useEffect(() => {
    const el = scrollRef.current;
    if (!link || !el || restoredRef.current) return;
    restoredRef.current = true;

    progressRef.current = link.reading_progress ?? 0;
    lastSavedRef.current = progressRef.current;
    markedReadRef.current = link.is_read;

    const maxScroll = el.scrollHeight - el.clientHeight;
    if (maxScroll > 0 && progressRef.current > 0 && progressRef.current < 1) {
      el.scrollTop = progressRef.current * maxScroll;
    }
    if (maxScroll <= 0 && !markedReadRef.current) {
      // The whole article fits on screen: it's read as soon as it's open.
      markedReadRef.current = true;
      progressRef.current = 1;
      lastSavedRef.current = 1;
      updateLink.mutate({ slug: link.slug, isRead: true, readingProgress: 1 });
    }
  }, [link]);

  useEffect(() => {
    const el = scrollRef.current;
    const currentSlug = link?.slug;
    if (!el || !currentSlug) return;

    const save = () => {
      const value = progressRef.current;
      if (Math.abs(value - lastSavedRef.current) < 0.01) return;
      lastSavedRef.current = value;
      updateLink.mutate({ slug: currentSlug, readingProgress: value });
    };

    const onScroll = () => {
      const maxScroll = el.scrollHeight - el.clientHeight;
      const progress =
        maxScroll <= 0 ? 1 : Math.min(1, el.scrollTop / maxScroll);
      if (progress > progressRef.current) progressRef.current = progress;

      if (!markedReadRef.current && progressRef.current >= 0.98) {
        markedReadRef.current = true;
        progressRef.current = 1;
        lastSavedRef.current = 1;
        updateLink.mutate({
          slug: currentSlug,
          isRead: true,
          readingProgress: 1,
        });
      }
    };

    const interval = setInterval(save, 3000);
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => {
      clearInterval(interval);
      el.removeEventListener("scroll", onScroll);
      save(); // persist the position when leaving the article
    };
  }, [link?.slug]);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center grow">
        <p className="text-n-4 base1">Loading...</p>
      </div>
    );
  }

  if (error || !link) {
    return (
      <div className="flex items-center justify-center grow">
        <p className="text-red-400 base1">
          Error: {error?.message ?? "Article not found"}
        </p>
      </div>
    );
  }

  return (
    <div className="relative grow overflow-hidden">
      <TableOfContents headings={headings} scrollContainer={scrollRef} />
      <div ref={scrollRef} className="h-full overflow-y-auto scroll-smooth">
        <article className="mx-auto max-w-2xl px-6 py-12 md:px-4">
          <header className="mb-8">
            <h1 className="font-inter text-2xl font-bold tracking-tight text-[#232627] dark:text-white mb-3">
              {link.title}
            </h1>
            <div className="flex items-center gap-3 text-sm text-neutral-500">
              {link.author && <span>{link.author}</span>}
              {link.author && link.feed_title && <span>&middot;</span>}
              {link.feed_title && <span>{link.feed_title}</span>}
              {link.published_at && (
                <>
                  <span>&middot;</span>
                  <time>
                    {new Date(link.published_at).toLocaleDateString("fr-FR", {
                      day: "numeric",
                      month: "long",
                      year: "numeric",
                    })}
                  </time>
                </>
              )}
            </div>
          </header>

          <div className="prose prose-neutral dark:prose-invert max-w-none text-[#232627] dark:text-neutral-200 leading-relaxed">
            <Markdown
              components={{
                h1: ({ children, node, ...props }) => (
                  <h1 id={slugify(getTextContent(children))} {...props}>{children}</h1>
                ),
                h2: ({ children, node, ...props }) => (
                  <h2 id={slugify(getTextContent(children))} {...props}>{children}</h2>
                ),
                h3: ({ children, node, ...props }) => (
                  <h3 id={slugify(getTextContent(children))} {...props}>{children}</h3>
                ),
              }}
            >
              {link.text_content}
            </Markdown>
          </div>
        </article>
      </div>
    </div>
  );
}