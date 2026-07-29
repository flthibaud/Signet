type BurgerProps = {
  className?: string;
  onClick: () => void;
};

const Burger = ({ className, onClick }: BurgerProps) => {
  return (
    <div>
      <button
        className={`relative z-25 shrink-0 flex flex-col items-center justify-center w-8 h-8 my-5 ml-auto mr-6 tap-highlight-color max-md:absolute max-md:top-5 max-md:right-4 max-md:m-0 ${className}`}
        onClick={onClick}
      >
        <span
          className={`w-5 h-0.5 my-0.5 bg-n-7 rounded-full transition-all dark:bg-n-4`}
        ></span>
        <span
          className={`w-5 h-0.5 my-0.5 bg-n-7 rounded-full transition-all dark:bg-n-4`}
        ></span>
      </button>
    </div>
  );
};

export default Burger;
