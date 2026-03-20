import { Links } from "~/components/Links";

export default function LinksPage() {
  return (
    <div className="flex flex-col h-full">
      <div className="flex justify-between items-center px-10 py-6 border-gray-700 border-b 2xl:py-4 md:px-4 shrink-0">
        <div>
          <input type="checkbox" name="rss-toggle" id="rss-toggle" />
        </div>
      </div>

      <div className="grow px-6 py-6 overflow-y-auto scroll-smooth md:px-4 md:pb-6">
        <Links />
      </div>
    </div>
  );
}
