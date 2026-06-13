import { forwardRef } from "react";
import { twMerge } from "tailwind-merge";
import type { LucideIcon } from "lucide-react";

type FieldProps = React.InputHTMLAttributes<HTMLInputElement> & {
  className?: string;
  classInput?: string;
  label?: string;
  textarea?: boolean;
  note?: string;
  error?: string;
  icon?: LucideIcon;
};

const Field = forwardRef<HTMLInputElement | HTMLTextAreaElement, FieldProps>(
  (
    {
      className,
      classInput,
      label,
      textarea,
      note,
      error,
      type,
      placeholder,
      required,
      icon: Icon,
      ...rest
    },
    ref,
  ) => {
    const errorClass = error
      ? "border-red-500/60 dark:border-red-500/60 focus:border-red-500/60"
      : "";

    return (
      <div className={`${className}`}>
        <div className="">
          {label && (
            <div className="flex mb-2 base2 font-semibold">{label}</div>
          )}
          <div className="relative">
            {textarea ? (
              <textarea
                className={`w-full h-24 px-3.5 py-3 bg-[#F3F5F7] border-2 border-[#F3F5F7] rounded-xl base2 text-[#141718] outline-none transition-colors placeholder:text-[#6C7275]/50 focus:bg-transparent resize-none dark:bg-[#232627] dark:border-[#232627] dark:text-[#E8ECEF] dark:focus:bg-transparent ${
                  Icon && "pl-12.5"
                }`}
                ref={ref as React.Ref<HTMLTextAreaElement>}
                placeholder={placeholder}
                required={required}
                {...(rest as React.TextareaHTMLAttributes<HTMLTextAreaElement>)}
              />
            ) : (
              <input
                className={twMerge(
                  `w-full h-13 px-3.5 bg-[#F3F5F7] border-2 border-[#F3F5F7] rounded-xl base2 text-[#141718] outline-none transition-colors placeholder:text-[#6C7275]/50 focus:bg-transparent dark:bg-[#232627] dark:border-[#232627] dark:text-[#E8ECEF] dark:focus:bg-transparent ${
                    Icon && "pl-12.5"
                  } ${classInput} ${errorClass}`,
                )}
                aria-invalid={error ? true : undefined}
                type={type || "text"}
                ref={ref as React.Ref<HTMLInputElement>}
                placeholder={placeholder}
                required={required}
                {...rest}
              />
            )}

            {Icon && (
              <Icon className="absolute top-3.5 left-4 pointer-events-none transition-colors w-6 h-6 text-[#6C7275]/50" />
            )}
          </div>
          {error ? (
            <div className="mt-2 base2 text-red-500">{error}</div>
          ) : (
            note && <div className="mt-2 base2 text-[#6C7275]/50">{note}</div>
          )}
        </div>
      </div>
    );
  },
);

Field.displayName = "Field";

export default Field;
