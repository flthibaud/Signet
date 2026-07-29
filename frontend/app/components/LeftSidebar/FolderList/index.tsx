import { useState } from "react";
import { Disclosure, DisclosureButton, DisclosurePanel } from "@headlessui/react";
import { twMerge } from "tailwind-merge";
import { ChevronDown, CirclePlus } from "lucide-react";

import Modal from "~/components/Modal";
import AddFolderList from "~/components/AddFolderList";

type FolderListType = {
  id: string;
  title: string;
  counter: number;
  color: string;
};

type FolderListProps = {
  visible?: boolean;
  items: FolderListType[];
};

const FolderList = ({ visible, items }: FolderListProps) => {
  const [visibleModal, setVisibleModal] = useState<boolean>(false);

  return (
    <>
      <div className="mb-auto pb-6">
        <Disclosure defaultOpen={true}>
          <DisclosureButton
            className={`flex items-center w-full h-12 text-left base2 text-n-4/75 transition-colors hover:cursor-pointer hover:text-n-3 ${
              visible ? "px-5" : "justify-center px-3"
            }`}
          >
            <ChevronDown className="text-n-4 transition-transform data-open:rotate-180" />
            {visible && <div className="ml-5">Folder list</div>}
          </DisclosureButton>
          <DisclosurePanel
            transition
            className={`origin-top transition duration-100 ease-out data-closed:scale-95 data-closed:opacity-0 ${
              !visible && "px-2"
            }`}
          >
            {items.map((item) => (
              <div
                className={twMerge(
                  `flex items-center w-full h-12 rounded-lg text-n-3/75 base2 font-semibold transition-colors hover:text-n-1 ${
                    visible ? "px-5" : "px-3"
                  }`,
                )}
                key={item.id}
              >
                <div className="flex justify-center items-center w-6 h-6">
                  <div
                    className="w-3.5 h-3.5 rounded"
                    style={{
                      backgroundColor: item.color,
                    }}
                  ></div>
                </div>
                {visible && (
                  <>
                    <div className="ml-5">{item.title}</div>
                    <div className="ml-auto px-2 bg-n-6 rounded-lg base2 font-semibold text-n-4">
                      {item.counter}
                    </div>
                  </>
                )}
              </div>
            ))}
          </DisclosurePanel>
        </Disclosure>
        <button
          className={`group flex items-center w-full h-12 text-left base2 text-n-3/75 transition-colors hover:cursor-pointer hover:text-n-3 ${
            visible ? "px-5" : "justify-center px-3"
          }`}
          onClick={() => setVisibleModal(true)}
        >
          <CirclePlus className="text-n-4 transition-colors group-hover:text-n-3" />
          {visible && <div className="ml-5">New folder</div>}
        </button>
      </div>
      <Modal
        className="max-md:p-0!"
        classWrap="max-w-160 max-md:min-h-dvh max-md:rounded-none max-md:pb-8"
        classButtonClose="absolute top-6 right-6 w-10 h-10 rounded-full bg-[#F3F5F7] max-md:right-5 dark:bg-[#6C7275]/25 dark:text-[#6C7275] dark:hover:text-[#FEFEFE]"
        visible={visibleModal}
        onClose={() => setVisibleModal(false)}
      >
        <AddFolderList onCancel={() => setVisibleModal(false)} />
      </Modal>
    </>
  );
};

export default FolderList;
