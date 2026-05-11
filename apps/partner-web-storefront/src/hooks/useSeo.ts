// SEO meta —— SPA 模式下用 document.title / meta tag 注入
// 不依赖 react-helmet（节省体积），自管 effect 保证组件卸载时回滚
import { useEffect } from "react";

export interface SeoMeta {
  title: string;
  description?: string;
  /** og:image 完整 URL */
  ogImage?: string;
  /** canonical 完整 URL */
  canonical?: string;
  /** 是否允许搜索引擎索引（招商落地页默认 true） */
  robots?: "index,follow" | "noindex,nofollow";
}

function setMeta(name: string, content: string, attr: "name" | "property" = "name"): () => void {
  if (typeof document === "undefined") return () => undefined;
  let el = document.querySelector<HTMLMetaElement>(`meta[${attr}="${name}"]`);
  const created = !el;
  if (!el) {
    el = document.createElement("meta");
    el.setAttribute(attr, name);
    document.head.appendChild(el);
  }
  const prev = el.getAttribute("content");
  el.setAttribute("content", content);
  return () => {
    if (created) {
      el?.parentElement?.removeChild(el);
    } else if (prev !== null) {
      el?.setAttribute("content", prev);
    }
  };
}

function setLink(rel: string, href: string): () => void {
  if (typeof document === "undefined") return () => undefined;
  let el = document.querySelector<HTMLLinkElement>(`link[rel="${rel}"]`);
  const created = !el;
  if (!el) {
    el = document.createElement("link");
    el.setAttribute("rel", rel);
    document.head.appendChild(el);
  }
  const prev = el.getAttribute("href");
  el.setAttribute("href", href);
  return () => {
    if (created) el?.parentElement?.removeChild(el);
    else if (prev) el?.setAttribute("href", prev);
  };
}

export function useSeo(meta: SeoMeta): void {
  useEffect(() => {
    const prevTitle = document.title;
    document.title = meta.title;
    const cleanups: Array<() => void> = [];
    if (meta.description) {
      cleanups.push(setMeta("description", meta.description));
      cleanups.push(setMeta("og:description", meta.description, "property"));
    }
    cleanups.push(setMeta("og:title", meta.title, "property"));
    if (meta.ogImage) cleanups.push(setMeta("og:image", meta.ogImage, "property"));
    if (meta.canonical) cleanups.push(setLink("canonical", meta.canonical));
    if (meta.robots) cleanups.push(setMeta("robots", meta.robots));
    return () => {
      document.title = prevTitle;
      cleanups.forEach((fn) => fn());
    };
  }, [meta.title, meta.description, meta.ogImage, meta.canonical, meta.robots]);
}
