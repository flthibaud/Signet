import { useState } from "react";
import { Tab, TabGroup, TabList, TabPanel, TabPanels } from "@headlessui/react";
import SignIn from "./SignIn";
import CreateAccount from "./CreateAccount";
import ForgotPassword from "./ForgotPassword";

const tabNav = ["Sign in", "Create account"];

const SIGN_IN_TAB = 0;
const CREATE_ACCOUNT_TAB = 1;

const Form = () => {
  const [forgot, setForgot] = useState<boolean>(false);
  const [selectedIndex, setSelectedIndex] = useState<number>(SIGN_IN_TAB);
  const [notice, setNotice] = useState<string | null>(null);

  const handleTabChange = (index: number) => {
    setSelectedIndex(index);
    // Clear the post-sign-up notice when the user leaves the sign-in tab.
    if (index !== SIGN_IN_TAB) setNotice(null);
  };

  const handleSignUpSuccess = () => {
    setSelectedIndex(SIGN_IN_TAB);
    setNotice("Your account has been created. Please sign in.");
  };

  return (
    <div className="w-full max-w-126 m-auto">
      {forgot ? (
        <ForgotPassword onClick={() => setForgot(false)} />
      ) : (
        <TabGroup selectedIndex={selectedIndex} onChange={handleTabChange}>
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
          <TabPanels>
            <TabPanel>
              <SignIn onForgotPassword={() => setForgot(true)} notice={notice} />
            </TabPanel>
            <TabPanel>
              <CreateAccount onSuccess={handleSignUpSuccess} />
            </TabPanel>
          </TabPanels>
        </TabGroup>
      )}
    </div>
  );
};

export default Form;
