type ProfileProps = {
  visible?: boolean;
};

const Profile = ({ visible }: ProfileProps) => (
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
            className="rounded-full object-cover"
            src="/images/avatar.jpg"
            alt="Avatar"
          />
          <div className="absolute -right-0.75 -bottom-0.75 w-4.5 h-4.5 bg-[#3FDD78] rounded-full border-4 border-[#232627]"></div>
        </div>
        {visible && (
          <>
            <div className="ml-4 mr-4">
              <div className="base2 font-semibold text-[#FEFEFE]">
                Tran Mau Tri Tam
              </div>
              <div className="caption1 font-semibold text-[#E8ECEF]/50">
                tam@ui8.net
              </div>
            </div>
            <div className="shrnik-0 ml-auto self-start px-3 bg-[#3FDD78] rounded-lg caption1 font-bold text-[#141718]">
              Free
            </div>
          </>
        )}
      </div>
      {visible && (
        <a className="btn border-[#343839] text-[#FEFEFE] hover:bg-[#343839] w-full mt-2" href="/pricing">
          Upgraded to Pro
        </a>
      )}
    </div>
  </div>
);

export default Profile;
