// NavBar —— 顶层导航 + 当前用户 + 语言切换
import { NavLink } from "react-router-dom";
import { Avatar, Button, Dropdown } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { useAuth } from "@/hooks/useAuth";
import { setLocale, type Locale } from "@/i18n";

const LINKS: { to: string; key: string }[] = [
  { to: "/dashboard", key: "nav.dashboard" },
  { to: "/customers", key: "nav.customers" },
  { to: "/allocate", key: "nav.allocate" },
  { to: "/invitations", key: "nav.invitations" },
  { to: "/pricing", key: "nav.pricing" },
  { to: "/wallet", key: "nav.wallet" },
  { to: "/statements", key: "nav.statements" },
  { to: "/disputes", key: "nav.disputes" },
  { to: "/tickets", key: "nav.tickets" },
  { to: "/kyc", key: "nav.kyc" },
  { to: "/settings", key: "nav.settings" },
];

export function NavBar(): JSX.Element {
  const { t, i18n } = useTranslation();
  const { me, logout } = useAuth();

  const switchLang = (l: Locale): void => {
    setLocale(l);
  };

  return (
    <header
      style={{
        display: "flex",
        alignItems: "center",
        gap: 12,
        padding: "12px 24px",
        borderBottom: "1px solid #e5e7eb",
        background: "#fff",
        position: "sticky",
        top: 0,
        zIndex: 50,
      }}
    >
      <strong style={{ fontSize: 16 }}>TraceNex Partner</strong>
      <nav style={{ display: "flex", gap: 4, flex: 1, flexWrap: "wrap" }} aria-label="primary">
        {LINKS.map((l) => (
          <NavLink
            key={l.to}
            to={l.to}
            style={({ isActive }) => ({
              padding: "6px 10px",
              borderRadius: 6,
              color: isActive ? "#1d4ed8" : "#374151",
              background: isActive ? "#eff6ff" : "transparent",
              textDecoration: "none",
              fontSize: 14,
            })}
          >
            {t(l.key)}
          </NavLink>
        ))}
      </nav>
      <Dropdown
        trigger="click"
        position="bottomRight"
        render={
          <Dropdown.Menu>
            <Dropdown.Item onClick={() => switchLang("zh-CN")}>简体中文</Dropdown.Item>
            <Dropdown.Item onClick={() => switchLang("en-US")}>English</Dropdown.Item>
          </Dropdown.Menu>
        }
      >
        <Button theme="borderless" size="small">
          {i18n.language === "en-US" ? "EN" : "中"}
        </Button>
      </Dropdown>
      <Dropdown
        trigger="click"
        position="bottomRight"
        render={
          <Dropdown.Menu>
            <Dropdown.Item onClick={() => logout("current")}>{t("nav.logout")}</Dropdown.Item>
            <Dropdown.Item type="danger" onClick={() => logout("all")}>
              {t("auth.logout_all")}
            </Dropdown.Item>
          </Dropdown.Menu>
        }
      >
        <span style={{ display: "flex", alignItems: "center", gap: 8, cursor: "pointer" }}>
          <Avatar size="small" alt="me">
            {me?.contact_name?.slice(0, 1) ?? "P"}
          </Avatar>
          <span style={{ fontSize: 13, color: "#374151" }}>
            {me?.contact_name ?? "—"}
          </span>
        </span>
      </Dropdown>
    </header>
  );
}
