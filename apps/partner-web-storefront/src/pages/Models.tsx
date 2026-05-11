import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { fetchPublicModels, type PublicModel, type PublicModelsResponse } from "@/api";
import { useSeo } from "@/hooks/useSeo";

function PriceCell({ value }: { value: string }): JSX.Element {
  // 已经是 string；直接展示，避免 IEEE 754 误差
  return <span>¥ {value}</span>;
}

function ModelRow({ m, t }: { m: PublicModel; t: (k: string) => string }): JSX.Element {
  return (
    <tr>
      <td style={cell}>
        <div style={{ fontWeight: 600, color: "#fff" }}>{m.display_name}</div>
        {m.description ? (
          <div style={{ color: "#9ca3af", fontSize: 12, marginTop: 4 }}>{m.description}</div>
        ) : null}
      </td>
      <td style={cell}>{m.vendor}</td>
      <td style={cell}>{m.context_window.toLocaleString()}</td>
      <td style={cell}>
        <PriceCell value={m.price_input_per_1k} />
      </td>
      <td style={cell}>
        <PriceCell value={m.price_output_per_1k} />
      </td>
      <td style={cell} aria-label={t("models.col.filing")}>
        {m.algorithm_filing_no ?? "—"}
      </td>
    </tr>
  );
}

const cell: React.CSSProperties = {
  padding: "12px 16px",
  borderBottom: "1px solid #1f2937",
  fontSize: 14,
  color: "#e5e7eb",
  textAlign: "left",
};

export function Models(): JSX.Element {
  const { t } = useTranslation();
  useSeo({
    title: `${t("models.title")} | ${t("app.title")}`,
    description: t("models.subtitle"),
    canonical: "https://partner.tracenex.cn/models",
    robots: "index,follow",
  });

  const { data, isLoading, isError, refetch } = useQuery<PublicModelsResponse>({
    queryKey: ["public", "models"],
    queryFn: fetchPublicModels,
    staleTime: 60_000,
  });

  return (
    <section>
      <h1 style={{ color: "#fff", margin: 0 }}>{t("models.title")}</h1>
      <p style={{ color: "#9ca3af" }}>{t("models.subtitle")}</p>
      {data && !data.icp_license_active ? (
        <div
          role="note"
          style={{
            padding: 12,
            background: "#1e293b",
            border: "1px solid #334155",
            borderRadius: 6,
            marginBottom: 16,
            color: "#fde68a",
          }}
        >
          {t("models.beta_banner")}
        </div>
      ) : null}

      {isLoading ? (
        <p>{t("common.loading")}</p>
      ) : isError ? (
        <div role="alert">
          <p>{t("errors.network")}</p>
          <button type="button" onClick={() => refetch()}>
            {t("common.retry")}
          </button>
        </div>
      ) : data && data.models.length > 0 ? (
        <div style={{ overflowX: "auto" }}>
          <table
            style={{ width: "100%", borderCollapse: "collapse", marginTop: 12 }}
            aria-label={t("models.title")}
          >
            <thead>
              <tr style={{ background: "#0f1722" }}>
                <th style={{ ...cell, fontWeight: 600 }}>{t("models.col.name")}</th>
                <th style={{ ...cell, fontWeight: 600 }}>{t("models.col.vendor")}</th>
                <th style={{ ...cell, fontWeight: 600 }}>{t("models.col.context")}</th>
                <th style={{ ...cell, fontWeight: 600 }}>{t("models.col.in_price")}</th>
                <th style={{ ...cell, fontWeight: 600 }}>{t("models.col.out_price")}</th>
                <th style={{ ...cell, fontWeight: 600 }}>{t("models.col.filing")}</th>
              </tr>
            </thead>
            <tbody>
              {data.models
                .filter((m) => m.enabled)
                .map((m) => (
                  <ModelRow key={m.id} m={m} t={t} />
                ))}
            </tbody>
          </table>
        </div>
      ) : (
        <p style={{ color: "#9ca3af" }}>{t("models.empty")}</p>
      )}
    </section>
  );
}
