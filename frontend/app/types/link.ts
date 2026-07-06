export interface Link {
  id: number;
  title: string;
  slug: string;
  description: string;
  image_url: string;
  saved_at: string;
  feed_title: string;
  reading_time_minutes: number;
  published_at: string;
  is_read: boolean;
  is_starred: boolean;
}

export interface LinkDetail {
  id: number;
  slug: string;
  title: string;
  author: string;
  image_url?: string;
  reading_time_minutes: number;
  text_content: string;
  feed_title?: string;
  published_at: string;
  saved_at: string;
  is_read: boolean;
  is_starred: boolean;
}

export interface LinksResponse {
  links: Link[];
  metadata: {
    current_page: number;
    page_size: number;
    total_records: number;
    total_pages: number;
  };
}

export interface Heading {
  id: string;
  text: string;
  level: number;
}
