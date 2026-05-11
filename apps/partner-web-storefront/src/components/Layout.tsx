// Layout —— Nav + Outlet + ComplianceFooter
import { Outlet } from "react-router-dom";
import { NavBar } from "./NavBar";
import { ComplianceFooter } from "./ComplianceFooter";

export function Layout(): JSX.Element {
  return (
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        flexDirection: "column",
        background: "#0a0c10",
        color: "#e5e7eb",
      }}
    >
      <NavBar />
      <main id="main" style={{ flex: 1, maxWidth: 1200, margin: "0 auto", padding: 24, width: "100%" }}>
        <Outlet />
      </main>
      <ComplianceFooter />
    </div>
  );
}
