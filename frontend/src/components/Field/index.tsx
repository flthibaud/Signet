import { twMerge } from "tailwind-merge";
import type { LucideIcon } from "lucide-react";

type FieldProps = {
  className?: string;
  classInput?: string;
  label?: string;
  textarea?: boolean;
  note?: string;
  type?: string;
  value: string;
  onChange: (event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => void;
  placeholder?: string;
  required?: boolean;
  icon?: LucideIcon;
};

const Field = ({
  className,
  classInput,
  label,
  textarea,
  note,
  type,
  value,
  onChange,
  placeholder,
  required,
  icon: Icon,
}: FieldProps) => {
  const handleKeyDown = (event: any) => {
    const remainingChars = 880 - value.length;
    if (remainingChars <= 0 && event.key !== "Backspace") {
      event.preventDefault();
    }
  };

  const remainingChars = 880 - value.length;

  return (
    <div className={`${className}`}>
      <div className="">
        {label && (
          <div className="flex mb-2 base2 font-semibold">
            {label}
            {textarea && (
              <span className="ml-auto pl-4 text-[#6C7275]/50">{remainingChars}</span>
            )}
          </div>
        )}
        <div className="relative">
          {textarea ? (
            <textarea
              className={`w-full h-24 px-3.5 py-3 bg-[#F3F5F7] border-2 border-[#F3F5F7] rounded-xl base2 text-[#141718] outline-none transition-colors placeholder:text-[#6C7275]/50 focus:bg-transparent resize-none dark:bg-[#232627] dark:border-[#232627] dark:text-[#E8ECEF] dark:focus:bg-transparent ${
                Icon && "pl-12.5"
              } ${value !== "" && "bg-transparent border-[#E8ECEF]/50"}`}
              value={value}
              onChange={onChange}
              onKeyDown={handleKeyDown}
              placeholder={placeholder}
              required={required}
            />
          ) : (
            <input
              className={twMerge(
                `w-full h-13 px-3.5 bg-[#F3F5F7] border-2 border-[#F3F5F7] rounded-xl base2 text-[#141718] outline-none transition-colors placeholder:text-[#6C7275]/50 focus:bg-transparent dark:bg-[#232627] dark:border-[#232627] dark:text-n-3 dark:focus:bg-transparent ${
                  Icon && "pl-12.5"
                } ${
                  value !== "" && "bg-transparent border-[#E8ECEF]/50"
                } ${classInput}`,
              )}
              type={type || "text"}
              value={value}
              onChange={onChange}
              placeholder={placeholder}
              required={required}
            />
          )}

          {Icon && (
            <Icon
              className={`absolute top-3.5 left-4 pointer-events-none transition-colors w-6 h-6 ${
                value !== "" ? "text-[#6C7275]" : "text-[#6C7275]/50"
              }`}
            />
          )}
        </div>
        {note && <div className="mt-2 base2 text-[#6C7275]/50">{note}</div>}
      </div>
    </div>
  );
};

export default Field;
