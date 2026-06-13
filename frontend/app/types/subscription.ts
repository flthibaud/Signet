export interface Feed {
  id: number;
  url: string;
  title: string;
  site_url: string;
  image_url?: string;
  is_active: boolean;
  last_fetched_at: string;
  created_at: string;
}

export interface Subscription {
  id: number;
  custom_title: string | null;
  custom_icon: string | null;
  created_at: string;
  feed: Feed;
  unread_count: number;
}

export interface SubscriptionsResponse {
  subscriptions: Subscription[];
}
