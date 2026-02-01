import { useState } from "react";
import { Tab, TabGroup, TabList, TabPanel, TabPanels } from "@headlessui/react";
import Logo from "../../Logo";
import SignIn from "./SignIn";
import CreateAccount from "./CreateAccount";
import ForgotPassword from "./ForgotPassword";
import { useTheme } from "../../../hooks/useTheme";

const tabNav = ["Sign in", "Create account"];

type FormProps = {};

const Form = ({}: FormProps) => {
  const [forgot, setForgot] = useState<boolean>(false);

  const theme = useTheme();
  const isLightMode = theme === "light";

  return (
    <div className="w-full max-w-126 m-auto">
      {forgot ? (
        <ForgotPassword onClick={() => setForgot(false)} />
      ) : (
        <>
          <Logo className="max-w-47.5 mx-auto mb-8" dark={isLightMode} />
          <TabGroup defaultIndex={0}>
            <TabList className="flex mb-8 p-1 bg-[#F3F5F7] rounded-xl dark:bg-[#141718]">
              {tabNav.map((button, index) => (
                <Tab
                  className="basis-1/2 h-10 rounded-[0.625rem] base2 font-semibold text-[#6C7275] transition-colors outline-none hover:text-[#141718] data-selected:bg-[#FEFEFE] data-selected:text-[#141718] data-selected:shadow-[0_0.125rem_0.125rem_rgba(0,0,0,0.07),inset_0_0.25rem_0.125rem_#FFFFFF] tap-highlight-color dark:hover:text-[#FEFEFE] dark:data-selected:bg-[#232627] dark:data-selected:text-[#FEFEFE] dark:data-selected:shadow-[0_0.125rem_0.125rem_rgba(0,0,0,0.07),inset_0_0.0625rem_0.125rem_rgba(255,255,255,0.02)]"
                  key={index}
                >
                  {button}
                </Tab>
              ))}
            </TabList>
            {/* <button className="btn-stroke-light btn-large w-full mb-3">
              <img src="/images/google.svg" width={24} height={24} alt="" />
              <span className="ml-4">Continue with Google</span>
            </button>
            <button className="btn-stroke-light btn-large w-full">
              <img src="/images/apple.svg" width={24} height={24} alt="" />
              <span className="ml-4">Continue with Apple</span>
            </button>
            <div className="flex items-center my-8 md:my-4">
              <span className="grow h-0.25 bg-n-4/50"></span>
              <span className="shrink-0 mx-5 text-n-4/50">OR</span>
              <span className="grow h-0.25 bg-n-4/50"></span>
            </div> */}
            <TabPanels>
              <TabPanel>
                <SignIn onClick={() => setForgot(true)} />
              </TabPanel>
              <TabPanel>
                <CreateAccount />
              </TabPanel>
            </TabPanels>
          </TabGroup>
        </>
      )}
    </div>
  );
};

export default Form;
