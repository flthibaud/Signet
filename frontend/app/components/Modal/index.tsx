import { Fragment } from "react";
import {
  Dialog,
  DialogPanel,
  Transition,
  TransitionChild,
} from "@headlessui/react";
import { twMerge } from "tailwind-merge";
import { X } from "lucide-react";

type ModalProps = {
  className?: string;
  classWrap?: string;
  classOverlay?: string;
  classButtonClose?: string;
  visible: boolean;
  onClose: () => void;
  initialFocus?: any;
  children: React.ReactNode;
  video?: boolean;
};

const Modal = ({
  className,
  classWrap,
  classOverlay,
  classButtonClose,
  visible,
  onClose,
  initialFocus,
  children,
  video,
}: ModalProps) => {
  return (
    <Transition show={visible} as={Fragment}>
      <Dialog
        initialFocus={initialFocus}
        className={`fixed inset-0 z-50 flex p-6 overflow-auto scroll-smooth max-md:px-4 ${className}`}
        onClose={onClose}
      >
        <TransitionChild
          as={Fragment}
          enter="ease-out duration-300"
          enterFrom="opacity-0"
          enterTo="opacity-100"
          leave="ease-in duration-200"
          leaveFrom="opacity-100"
          leaveTo="opacity-0"
        >
          <div
            className={`fixed inset-0 ${
              video ? "bg-n-7/95" : "bg-n-7/75 dark:bg-n-6/90"
            } ${classOverlay}`}
            aria-hidden="true"
          />
        </TransitionChild>
        <TransitionChild
          as={Fragment}
          enter="ease-out duration-300"
          enterFrom={`opacity-0 ${!video && "scale-95"}`}
          enterTo={`opacity-100 ${!video && "scale-100"}`}
          leave="ease-in duration-200"
          leaveFrom={`opacity-100 ${!video && "scale-100"}`}
          leaveTo={`opacity-0 ${!video && "scale-95"}`}
        >
          <DialogPanel
            className={twMerge(
              `relative z-10 max-w-150 w-full m-auto bg-n-1 rounded-3xl dark:bg-n-7 ${
                video &&
                "static max-w-5xl aspect-video rounded-[1.25rem] bg-n-7 overflow-hidden shadow-[0_2.5rem_8rem_rgba(0,0,0,0.5)]"
              } ${classWrap}`,
            )}
          >
            {children}
            <button
              className={twMerge(
                `text-n-7 transition-colors hover:text-primary-1 dark:text-n-1 ${
                  video &&
                  "absolute top-6 right-6 w-10 h-10 bg-n-1 rounded-full text-n-7!"
                } ${classButtonClose}`,
              )}
              onClick={onClose}
            >
              <X />
            </button>
          </DialogPanel>
        </TransitionChild>
      </Dialog>
    </Transition>
  );
};

export default Modal;
