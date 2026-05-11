// admin i18n init
import i18n, { type i18n as I18nType } from "i18next";
import { initReactI18next } from "react-i18next";
import zhCN from "./locales/zh-CN.json";
import enUS from "./locales/en-US.json";

const SUPPORTED = ["zh-CN", "en-US"] as const;
export type Locale = (typeof SUPPORTED)[number];

function detectLocale(): Locale {
  if (typeof window === "undefined") return "zh-CN";
  const stored = window.localStorage.getItem("tnbiz.locale");
  if (stored === "zh-CN" || stored === "en-US") return stored;
  return "zh-CN";
}

let initPromise: Promise<I18nType> | undefined;

export function initI18n(): Promise<I18nType> {
  if (initPromise) return initPromise;
  const lng = detectLocale();
  initPromise = i18n
    .use(initReactI18next)
    .init({
      lng,
      fallbackLng: "zh-CN",
      supportedLngs: SUPPORTED as unknown as string[],
      ns: ["admin"],
      defaultNS: "admin",
      resources: {
        "zh-CN": { admin: zhCN },
        "en-US": { admin: enUS },
      },
      interpolation: { escapeValue: false },
      returnNull: false,
    })
    .then(() => i18n);
  return initPromise;
}

export function setLocale(loc: Locale): void {
  if (typeof window !== "undefined") window.localStorage.setItem("tnbiz.locale", loc);
  void i18n.changeLanguage(loc);
}

export { i18n };
