import {
    Listbox,
    ListboxButton,
    Transition,
    ListboxOptions,
    ListboxOption,
} from "@headlessui/react";
import { twMerge } from "tailwind-merge";
import { Check, ChevronDown, type LucideIcon } from "lucide-react";

type SelectProps = {
    label?: string;
    title?: string;
    icon?: LucideIcon;
    className?: string;
    classButton?: string;
    classArrow?: string;
    classOptions?: string;
    classOption?: string;
    classIcon?: string;
    placeholder?: string;
    items: any;
    value: any;
    onChange: any;
    small?: boolean;
    up?: boolean;
};

const Select = ({
    label,
    title,
    icon: Icon,
    className,
    classButton,
    classArrow,
    classOptions,
    classOption,
    classIcon,
    placeholder,
    items,
    value,
    onChange,
    small,
    up,
}: SelectProps) => {
    const ValueIcon: LucideIcon | undefined = value?.icon;

    return (
        <div className={`relative ${className}`}>
            {label && (
                <div className="flex mb-2 base2 font-semibold">{label}</div>
            )}
            <Listbox value={value} onChange={onChange}>
                {({ open }) => (
                    <>
                        <ListboxButton
                            className={twMerge(
                                `flex items-center w-full h-13 px-4 rounded-xl bg-[#FEFEFE] base2 outline-none tap-highlight-color ${
                                    small
                                        ? `h-9 pr-3 rounded-md shadow-[0_0.125rem_0.25rem_rgba(0,0,0,0.15)] dark:shadow-[0_0.125rem_0.25rem_rgba(0,0,0,0.15),inset_0_0_0_0.0625rem_rgba(254,254,254,.1)] dark:bg-[#232627] ${
                                              open &&
                                              "shadow-[0_0.125rem_0.25rem_rgba(0,0,0,0.15)]"
                                          }`
                                        : `shadow-[inset_0_0_0_0.0625rem_#E8ECEF] dark:shadow-[inset_0_0_0_0.0625rem_#343839] dark:bg-transparent ${
                                              open &&
                                              "!shadow-[inset_0_0_0_0.125rem_#0084FF]"
                                          }`
                                } ${classButton}`
                            )}
                        >
                            {title && (
                                <div className="shrink-0 mr-2 pr-2 border-r border-n-3 text-n-4 dark:border-[#6C7275]/50">
                                    {title}
                                </div>
                            )}
                            {Icon && (
                                <Icon
                                    className={`shrink-0 mr-2 dark:text-[#6C7275] ${
                                        small && "w-5 h-5 mr-1.5"
                                    } ${classIcon}`}
                                />
                            )}
                            {value?.color && (
                                <div
                                    className="shrink-0 w-3.5 h-3.5 ml-1 mr-4 rounded"
                                    style={{ backgroundColor: value.color }}
                                ></div>
                            )}
                            {ValueIcon && (
                                <ValueIcon className="w-5 h-5 mr-3 dark:text-[#FEFEFE]" />
                            )}
                            <span
                                className={`mr-auto truncate ${
                                    small && "font-semibold"
                                }`}
                            >
                                {value ? (
                                    value.title
                                ) : (
                                    <span className="text-[#6C7275]">
                                        {placeholder}
                                    </span>
                                )}
                            </span>
                            <ChevronDown
                                className={`shrink-0 ml-2 transition-transform dark:text-[#FEFEFE] ${
                                    open && "rotate-180"
                                } ${small && "ml-1"} ${classArrow}`}
                            />
                        </ListboxButton>
                        <Transition
                            leave="transition ease-in duration-100"
                            leaveFrom="opacity-100"
                            leaveTo="opacity-0"
                        >
                            <ListboxOptions
                                className={twMerge(
                                    `absolute left-0 right-0 w-full mt-2 p-2 bg-[#FEFEFE] rounded-lg shadow-[0_0_1rem_0.25rem_rgba(0,0,0,0.04),0_2rem_2rem_-1.5rem_rgba(0,0,0,0.1),inset_0_0_0_0.0625rem_#E8ECEF] outline-none dark:shadow-[0_0_1rem_0.25rem_rgba(0,0,0,0.04),0_2rem_2rem_-1.5rem_rgba(0,0,0,0.1),inset_0_0_0_0.0625rem_#343839] dark:bg-[#232627] ${
                                        small && "right-auto mt-1 shadow-md"
                                    } ${
                                        up &&
                                        `top-auto bottom-full mt-0 ${
                                            small ? "mb-1" : "mb-2"
                                        }`
                                    } ${open && "z-10"} ${classOptions}`
                                )}
                            >
                                {items.map((item: any) => {
                                    const ItemIcon: LucideIcon | undefined =
                                        item.icon;

                                    return (
                                        <ListboxOption
                                            className={`flex items-start p-2 rounded-lg base2 text-[#6C7275] transition-colors cursor-pointer hover:text-[#141718] ui-selected:!bg-[#E8ECEF]/50 ui-selected:!text-[#141718] tap-highlight-color dark:hover:text-[#E8ECEF] dark:ui-selected:!bg-[#141718] dark:ui-selected:!text-[#FEFEFE] ${
                                                small && "py-1 font-semibold"
                                            } ${classOption}`}
                                            key={item.id}
                                            value={item}
                                        >
                                            {item.color && (
                                                <div
                                                    className="shrink-0 w-3.5 h-3.5 mt-[0.3125rem] ml-1 mr-4 rounded"
                                                    style={{
                                                        backgroundColor:
                                                            item.color,
                                                    }}
                                                ></div>
                                            )}
                                            {ItemIcon && (
                                                <ItemIcon className="w-5 h-5 mt-0.5 mr-3 dark:text-[#FEFEFE]" />
                                            )}
                                            <div className="mr-auto">
                                                {item.title}
                                            </div>
                                            {!small && (
                                                <Check className="hidden w-5 h-5 ml-2 mt-0.5 text-[#141718] ui-selected:inline-block dark:text-[#FEFEFE]" />
                                            )}
                                        </ListboxOption>
                                    );
                                })}
                            </ListboxOptions>
                        </Transition>
                    </>
                )}
            </Listbox>
        </div>
    );
};

export default Select;
