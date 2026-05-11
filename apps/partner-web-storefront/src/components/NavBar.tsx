// 顶部导航 —— 包含跳过链接（无障碍 WCAG 2.4.1）+ 语言切换
import * as React from "react";
import { Link, NavLink } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { setLocale, type Locale } from "@/i18n";

const NAV_LINKS: ReadonlyArray<{ to: string; key: string }> = [
  { to: "/", key: "nav.home" },
  { to: "/models", key: "nav.models" },
  { to: "/pricing", key: "nav.pricing" },
  { to: "/apply-partner", key: "nav.apply" },
  { to: "/legal/privacy", key: "nav.legal" },
];

export function NavBar(): JSX.Element {
  const { t, i18n } = useTranslation();
  const current = (i18n.resolvedLanguage ?? "zh-CN") as Locale;
  const onSwitch = React.useCallback(() => {
    setLocale(current === "zh-CN" ? "en-US" : "zh-CN");
  }, [current]);

  return (
    <header
      style={{
        position: "sticky",
        top: 0,
        zIndex: 10,
        background: "#0E1116",
        borderBottom: "1px solid #2a2f36",
      }}
    >
      <a
        href="#main"
        style={{
          position: "absolute",
          left: -9999,
          top: 0,
          color: "#fff",
          background: "#1d4ed8",
          padding: "4px 8px",
        }}
        onFocus={(e) => {
          e.currentTarget.style.left = "8px";
        }}
        onBlur={(e) => {
          e.currentTarget.style.left = "-9999px";
        }}
      >
        {t("skip_to_main")}
      </a>
      <nav
        aria-label="primary"
        style={{
          maxWidth: 1200,
          margin: "0 auto",
          padding: "12px 16px",
          display: "flex",
          alignItems: "center",
          gap: 16,
          color: "#cbd5e1",
        }}
      >
        <Link
          to="/"
          style={{ color: "#fff", fontWeight: 600, textDecoration: "none", marginRight: 24 }}
        >
          TraceNex Partner
        </Link>
        <div style={{ display: "flex", flex: 1, gap: 16 }}>
          {NAV_LINKS.map((link) => (
            <NavLink
              key={link.to}
              to={link.to}
              style={({ isActive }) => ({
                color: isActive ? "#fff" : "#cbd5e1",
                textDecoration: "none",
                padding: "4px 0",
                borderBottom: isActive ? "2px solid #3b82f6" : "2px solid transparent",
              })}
            >
              {t(link.key)}
            </NavLink>
          ))}
        </div>
        <button
          type="button"
          onClick={onSwitch}
          aria-label={t("nav.lang")}
          style={{
            background: "transparent",
            color: "#cbd5e1",
            border: "1px solid #2a2f36",
            borderRadius: 4,
            padding: "4px 10px",
            cursor: "pointer",
          }}
        >
          {current === "zh-CN" ? "EN" : "中文"}
        </button>
      </nav>
    </header>
  );
}
