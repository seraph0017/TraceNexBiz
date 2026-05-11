// Layout —— Nav + Banner + Outlet + ComplianceFooter
import { Outlet } from "react-router-dom";
import { NavBar } from "./NavBar";
import { OrphanBanner } from "./OrphanBanner";
import { ComplianceFooter } from "./ComplianceFooter";

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
      <OrphanBanner />
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
      <ComplianceFooter />
    </div>
  );
}
