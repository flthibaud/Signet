import SubscribeForm from "~/components/Feed/SubscribeForm";
import SubscriptionList from "~/components/Feed/SubscriptionList";

export default function FeedPage() {
  return (
    <div className="grow px-10 py-20 overflow-y-auto scroll-smooth 2xl:py-12 md:px-4 md:pt-0 md:pb-6">
      <div className="mb-10 text-center">
        <div className="text-black body1 text-n-4 2xl:body1S dark:text-white">
          Paste URL of an RSS feed to subscribe and see the latest articles.
        </div>
      </div>
      <SubscribeForm />
      <SubscriptionList />
    </div>
  );
}
