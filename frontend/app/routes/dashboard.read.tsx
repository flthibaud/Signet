import { isValidElement, useEffect, useMemo, useRef, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams } from "react-router";
import Markdown from "react-markdown";
import { extractHeadings } from "~/utils/extractHeadings";
import { slugify } from "~/utils/slugify";
import { useUpdateLink } from "~/lib/links";

import TableOfContents from "~/components/Links/TableOfContents";

function getTextContent(node: ReactNode): string {
  if (typeof node === "string") return node;
  if (typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(getTextContent).join("");
  if (isValidElement(node)) {
    return getTextContent(node.props.children as ReactNode);
  }
  return "";
}

export default function ArticlePage() {
  const { slug } = useParams();
  const scrollRef = useRef<HTMLDivElement>(null);
  const { data: article, isLoading, error } = useQuery({
    queryKey: ["article", slug],
    queryFn: async () => {
      const response = await fetch(`/v1/links/${slug}`);
      if (!response.ok) {
        throw new Error("Failed to fetch article");
      }
      return response.json();
    },
  });

  const link = article?.link;
  const headings = useMemo(
    () => (link?.text_content ? extractHeadings(link.text_content) : []),
    [link?.text_content],
  );

  // Mark the article as read once it has been opened, which decrements the
  // feed's unread badge. The cache patch in useUpdateLink flips is_read, so
  // this fires at most once per article.
  const updateLink = useUpdateLink();
  useEffect(() => {
    if (link && !link.is_read) {
      updateLink.mutate({ slug: link.slug, isRead: true });
    }
  }, [link?.slug, link?.is_read]);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center grow">
        <p className="text-n-4 base1">Loading...</p>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex items-center justify-center grow">
        <p className="text-red-400 base1">Error: {error.message}</p>
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