// admin Layout —— Watermark + 深色顶栏 + main
import { Outlet } from "react-router-dom";
import { NavBar } from "./NavBar";
import { Watermark } from "./Watermark";

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
      <Watermark />
      <NavBar />
      <main
        id="main"
        style={{
          flex: 1,
          maxWidth: 1440,
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
