// Layout —— Nav + Outlet；带 KYC 驳回 banner
import { Outlet } from "react-router-dom";
import { NavBar } from "./NavBar";
import { KycBanner } from "./KycBanner";

export function Layout(): JSX.Element {
  return (
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        flexDirection: "column",
        background: "#f9fafb",
      }}
    >
      <NavBar />
      <KycBanner />
      <main
        id="main"
        style={{
          flex: 1,
          maxWidth: 1280,
          margin: "0 auto",
          padding: 24,
          width: "100%",
        }}
      >
        <Outlet />
      </main>
    </div>
  );
}
