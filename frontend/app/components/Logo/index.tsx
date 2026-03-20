type LogoProps = {
  className?: string;
  dark?: boolean;
};

const Logo = ({ className, dark }: LogoProps) => (
  <a className={`flex w-[11.88rem] ${className}`} href="/">
    <img
      className="w-full h-auto"
      src={dark ? "/images/logo-dark.svg" : "/images/logo.svg"}
      width={190}
      height={40}
      alt="Brainwave"
    />
  </a>
);

export default Logo;
