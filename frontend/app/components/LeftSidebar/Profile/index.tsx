import { useEffect, useState } from "react";
import type { User } from "~/types/user";

type ProfileProps = {
  visible?: boolean;
};

const Profile = ({ visible }: ProfileProps) => {
  const [user, setUser] = useState<User | null>(null);

  useEffect(() => {
    fetch("/v1/users/me")
      .then((res) => res.json())
      .then((data) => setUser(data.user));
  }, []);

  if (!user) return null;

  return (
    <div
      className={`${
        !visible ? "mb-6" : "mb-3 shadow-[0_1.25rem_1.5rem_0_rgba(0,0,0,0.5)]"
      }`}
    >
      <div className={`${visible && "p-2.5 bg-[#232627] rounded-xl"}`}>
        <div
          className={`flex items-center ${
            !visible ? "justify-center" : "px-2.5 py-2.5 pb-4.5"
          }`}
        >
          <div className="relative w-10 h-10">
            <img
              className="rounded-full object-cover w-10 h-10"
              src="/images/avatar.jpg"
              alt="Avatar"
            />
            <div className="absolute -right-0.75 -bottom-0.75 w-4.5 h-4.5 bg-[#3FDD78] rounded-full border-4 border-[#232627]"></div>
          </div>
          {visible && (
            <>
              <div className="ml-4">
                <div className="base2 font-semibold text-[#FEFEFE]">
                  {user?.username}
                </div>
                <div className="caption1 text-sm font-semibold text-[#E8ECEF]/50">
                  {user?.email}
                </div>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
};

export default Profile;
