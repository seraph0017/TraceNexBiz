// Legal/:doc —— 动态加载法律文档
// 安全：markdown 渲染不直接 dangerouslySetInnerHTML；用最小子集（标题 / 段落 / 列表 / 链接）的安全解析
//      避免引入 marked / remark 增大 bundle，且杜绝 XSS（PRD §17.6）
import * as React from "react";
import { useParams, Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { fetchLegalDoc, type LegalDoc } from "@/api";
import { useSeo } from "@/hooks/useSeo";

const VALID_DOCS = new Set([
  "privacy",
  "terms",
  "partner",
  "algorithm_filing",
  "gen_ai_filing",
  "platform_qualification",
  "dpo",
  "complaint",
  "children",
]);

function titleKey(slug: string | undefined): string {
  if (!slug) return "legal.not_found";
  return `legal.title.${slug}`;
}

/** 安全 markdown：仅支持标题、段落、列表、超链接；其余统一转义 */
function renderMarkdown(text: string): React.ReactNode {
  const lines = text.split(/\r?\n/);
  const blocks: React.ReactNode[] = [];
  let buffer: string[] = [];
  let listBuffer: string[] | null = null;

  const flushParagraph = (): void => {
    if (buffer.length > 0) {
      blocks.push(<p key={blocks.length}>{renderInline(buffer.join(" "))}</p>);
      buffer = [];
    }
  };
  const flushList = (): void => {
    if (listBuffer) {
      const items = listBuffer;
      blocks.push(
        <ul key={blocks.length} style={{ paddingLeft: 20 }}>
          {items.map((it, i) => (
            <li key={i}>{renderInline(it)}</li>
          ))}
        </ul>,
      );
      listBuffer = null;
    }
  };

  for (const raw of lines) {
    const line = raw.trimEnd();
    if (line.startsWith("## ")) {
      flushParagraph();
      flushList();
      blocks.push(
        <h2 key={blocks.length} style={{ color: "#fff", marginTop: 24 }}>
          {line.slice(3)}
        </h2>,
      );
      continue;
    }
    if (line.startsWith("# ")) {
      flushParagraph();
      flushList();
      blocks.push(
        <h1 key={blocks.length} style={{ color: "#fff" }}>
          {line.slice(2)}
        </h1>,
      );
      continue;
    }
    if (line.startsWith("- ") || line.startsWith("* ")) {
      flushParagraph();
      if (!listBuffer) listBuffer = [];
      listBuffer.push(line.slice(2));
      continue;
    }
    if (line.length === 0) {
      flushParagraph();
      flushList();
      continue;
    }
    flushList();
    buffer.push(line);
  }
  flushParagraph();
  flushList();
  return blocks;
}

function renderInline(text: string): React.ReactNode {
  // 只匹配 markdown link [label](url)，url 必须以 http(s):/ 或 mailto: 开头
  const re = /\[([^\]]+)\]\((https?:\/\/[^)]+|mailto:[^)]+)\)/g;
  const out: React.ReactNode[] = [];
  let lastIndex = 0;
  let m: RegExpExecArray | null;
  let key = 0;
  while ((m = re.exec(text)) !== null) {
    if (m.index > lastIndex) out.push(text.slice(lastIndex, m.index));
    out.push(
      <a
        key={key++}
        href={m[2]}
        target="_blank"
        rel="noreferrer noopener"
        style={{ color: "#60a5fa" }}
      >
        {m[1]}
      </a>,
    );
    lastIndex = m.index + m[0].length;
  }
  if (lastIndex < text.length) out.push(text.slice(lastIndex));
  return out;
}

export function Legal(): JSX.Element {
  const { doc } = useParams<{ doc: string }>();
  const { t } = useTranslation();
  const valid = doc ? VALID_DOCS.has(doc) : false;

  const query = useQuery<LegalDoc>({
    queryKey: ["public", "legal", doc],
    queryFn: () => fetchLegalDoc(doc as string),
    enabled: valid,
    staleTime: 5 * 60_000,
  });

  const titleText = t(titleKey(doc));
  useSeo({
    title: `${titleText} | ${t("app.title")}`,
    canonical: `https://partner.tracenex.cn/legal/${doc ?? ""}`,
    robots: "index,follow",
  });

  if (!valid) {
    return (
      <section>
        <h1>{t("legal.not_found")}</h1>
        <p>
          <Link to="/">{t("common.back_home")}</Link>
        </p>
      </section>
    );
  }

  return (
    <section>
      <h1 style={{ color: "#fff" }}>{titleText}</h1>
      {query.data ? (
        <>
          <p style={{ color: "#9ca3af", fontSize: 12 }}>
            {t("legal.updated_at", { date: query.data.updated_at })} ·{" "}
            {t("legal.version", { version: query.data.version })}
          </p>
          <article style={{ color: "#e5e7eb", lineHeight: 1.8 }}>
            {renderMarkdown(query.data.body_markdown)}
          </article>
        </>
      ) : query.isLoading ? (
        <p>{t("common.loading")}</p>
      ) : (
        <div role="alert">
          <p>{t("errors.network")}</p>
          <button type="button" onClick={() => query.refetch()}>
            {t("common.retry")}
          </button>
        </div>
      )}
    </section>
  );
}
