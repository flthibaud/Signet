export interface Link {
  id: number;
  title: string;
  slug: string;
  /** The original article URL, for opening the source outside the reader. */
  article_url: string;
  description: string;
  image_url: string;
  saved_at: string;
  /** ISO timestamp, or null when the link is not archived. */
  archived_at: string | null;
  feed_title: string;
  reading_time_minutes: number;
  published_at: string;
  is_read: boolean;
  is_starred: boolean;
  reading_progress: number;
  reading_progress_anchor_index: number;
}

export interface LinkDetail {
  id: number;
  slug: string;
  title: string;
  article_url: string;
  author: string;
  image_url?: string;
  reading_time_minutes: number;
  text_content: string;
  feed_title?: string;
  published_at: string;
  saved_at: string;
  archived_at: string | null;
  is_read: boolean;
  is_starred: boolean;
  reading_progress: number;
  reading_progress_anchor_index: number;
}

export interface LinksResponse {
  links: Link[];
  metadata: {
    current_page: number;
    page_size: number;
    has_more: boolean;
  };
}

export interface Heading {
  id: string;
  text: string;
  level: number;
}
