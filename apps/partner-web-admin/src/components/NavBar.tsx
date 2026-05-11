// admin NavBar —— 独立深色顶栏（ADR-F1）
import { NavLink } from "react-router-dom";
import { Avatar, Button, Dropdown } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { useAuth } from "@/hooks/useAuth";
import { setLocale, type Locale } from "@/i18n";

const LINKS: { to: string; key: string }[] = [
  { to: "/partners", key: "nav.partners" },
  { to: "/kyc", key: "nav.kyc" },
  { to: "/wallet", key: "nav.wallet" },
  { to: "/settlements", key: "nav.settlements" },
  { to: "/refunds", key: "nav.refunds" },
  { to: "/red-flush", key: "nav.red_flush" },
  { to: "/audit-log", key: "nav.audit_log" },
  { to: "/content-safety/reports", key: "nav.content_safety" },
  { to: "/pia", key: "nav.pia" },
  { to: "/pipl-complaints", key: "nav.pipl_complaints" },
  { to: "/system/security", key: "nav.system_security" },
  { to: "/system/biz-settings", key: "nav.system_biz" },
  { to: "/saga/force-resolve", key: "nav.saga_force_resolve" },
  { to: "/staff", key: "nav.staff" },
];

export function NavBar(): JSX.Element {
  const { t } = useTranslation();
  const { me, logout } = useAuth();
  const switchLang = (l: Locale): void => setLocale(l);

  return (
    <header
      style={{
        display: "flex",
        alignItems: "center",
        gap: 12,
        padding: "10px 16px",
        background: "#0f172a",
        color: "#e5e7eb",
        position: "sticky",
        top: 0,
        zIndex: 50,
        borderBottom: "1px solid #1e293b",
      }}
    >
      <a href="#main" className="visually-hidden" style={{ color: "#fff" }}>
        Skip to main content
      </a>
      <strong style={{ fontSize: 16, color: "#fbbf24" }}>TraceNex Admin</strong>
      <nav style={{ display: "flex", gap: 4, flex: 1, flexWrap: "wrap" }} aria-label="primary">
        {LINKS.map((l) => (
          <NavLink
            key={l.to}
            to={l.to}
            style={({ isActive }) => ({
              padding: "5px 8px",
              borderRadius: 4,
              color: isActive ? "#fbbf24" : "#cbd5e1",
              background: isActive ? "#1e293b" : "transparent",
              textDecoration: "none",
              fontSize: 13,
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
        <Button theme="borderless" size="small" style={{ color: "#cbd5e1" }}>
          中
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
        <span style={{ display: "flex", alignItems: "center", gap: 8, cursor: "pointer", color: "#fff" }}>
          <Avatar size="small" alt="me">
            {me?.username?.slice(0, 1).toUpperCase() ?? "S"}
          </Avatar>
          <span style={{ fontSize: 13 }}>
            {me?.username ?? "—"} · {me?.role ?? ""}
          </span>
        </span>
      </Dropdown>
    </header>
  );
}
