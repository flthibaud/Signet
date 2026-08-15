import OpmlActions from "~/components/Feed/OpmlActions";
import SubscribeForm from "~/components/Feed/SubscribeForm";
import SubscriptionList from "~/components/Feed/SubscriptionList";

export default function FeedPage() {
  return (
    <div className="grow px-10 py-20 overflow-y-auto scroll-smooth max-[1420px]:py-12 max-md:px-4 max-md:pt-0 max-md:pb-6">
      <div className="mb-10 text-center">
        <div className="text-black body1 max-[1420px]:body1S dark:text-white">
          Paste URL of an RSS feed to subscribe and see the latest articles.
        </div>
      </div>
      <SubscribeForm />
      <OpmlActions />
      <SubscriptionList />
    </div>
  );
}
