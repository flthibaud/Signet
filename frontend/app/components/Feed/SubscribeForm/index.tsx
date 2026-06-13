import { Rss } from "lucide-react";
import { useForm, type SubmitHandler } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";

import Field from "~/components/Field";
import { applyApiError } from "~/lib/api";
import { useSubscribe } from "~/lib/feeds";
import { subscribeSchema, type SubscribeInputs } from "~/lib/validations/feed";

const SubscribeForm = () => {
  const subscribe = useSubscribe();
  const {
    register,
    handleSubmit,
    reset,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<SubscribeInputs>({
    resolver: zodResolver(subscribeSchema),
  });

  const onSubmit: SubmitHandler<SubscribeInputs> = async (formData) => {
    try {
      await subscribe.mutateAsync(formData);
      reset();
    } catch (error) {
      applyApiError(error, setError);
    }
  };

  return (
    <form onSubmit={handleSubmit(onSubmit)} noValidate className="mx-auto max-w-126">
      {errors.root && (
        <div className="mb-4 p-3 rounded-xl bg-red-500/10 text-red-500 text-sm font-semibold">
          {errors.root.message}
        </div>
      )}
      {subscribe.isSuccess && (
        <div className="mb-4 p-3 rounded-xl bg-green-500/10 text-green-600 text-sm font-semibold dark:text-green-400">
          Subscribed! Articles are being imported in the background.
        </div>
      )}
      <Field
        className="mb-4"
        classInput="dark:bg-[#141718] dark:border-[#141718] dark:focus:bg-transparent"
        placeholder="https://example.com/feed.xml"
        icon={Rss}
        type="url"
        autoComplete="url"
        error={errors.url?.message}
        {...register("url")}
      />
      <button
        className="btn btn-blue btn-large w-full"
        type="submit"
        disabled={isSubmitting}
      >
        {isSubmitting ? "Subscribing..." : "Subscribe"}
      </button>
    </form>
  );
};

export default SubscribeForm;
