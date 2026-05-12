// Step 5: 审阅 + consent_text_version 勾选 + 提交
import { useTranslation } from "react-i18next";
import { ConsentBox } from "@/components/apply/ConsentBox";
import type {
  ContactStep,
  CompanyStep,
  ScaleStep,
  KycStep,
  BankStep,
} from "@/schemas/applyPartner";
import { FormButtons } from "./FormButtons";

export function Step5Review(props: {
  draft: Partial<ContactStep & CompanyStep & ScaleStep & BankStep>;
  kyc: KycStep;
  consent: { consent_id: number; version: string } | null;
  onConsent: (c: { consent_id: number; version: string }) => void;
  consentVersion: string;
  onPrev: () => void;
  onSubmit: () => void | Promise<void>;
  submitting: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <div>
      <Summary draft={props.draft} kyc={props.kyc} />
      <ConsentBox
        version={props.consentVersion}
        accepted={Boolean(props.consent)}
        onAccepted={props.onConsent}
      />
      <FormButtons
        prev
        onPrev={props.onPrev}
        submit
        submitLabel={props.submitting ? t("apply.submitting") : t("apply.submit")}
        onSubmit={props.onSubmit}
        disabled={!props.consent || props.submitting}
      />
    </div>
  );
}

function Summary(props: {
  draft: Partial<ContactStep & CompanyStep & ScaleStep & BankStep>;
  kyc: KycStep;
}): JSX.Element {
  const { t } = useTranslation();
  // 只展示非敏感字段；身份证 / 上传 URL / 银行账号 用 "已填写" 表示
  const rows: Array<[string, string]> = [
    [t("apply.field.type"), props.draft.type ?? "—"],
    [t("apply.field.contact_name"), props.draft.contact_name ?? "—"],
    [t("apply.field.contact_email"), props.draft.contact_email ?? "—"],
    [t("apply.field.expected_calls"), String(props.draft.expected_monthly_calls ?? "—")],
  ];
  if (props.draft.type === "enterprise") {
    rows.push([t("apply.field.company_name"), props.draft.company_name ?? "—"]);
  }
  if (props.draft.tax_status) {
    rows.push(["税务身份", props.draft.tax_status]);
  }
  if (props.draft.settlement_bank_name) {
    rows.push(["开户银行", props.draft.settlement_bank_name]);
  }
  return (
    <dl style={{ background: "#11151c", border: "1px solid #1f2937", padding: 16, borderRadius: 6 }}>
      {rows.map(([k, v]) => (
        <div key={k} style={{ display: "flex", gap: 12, marginBottom: 6 }}>
          <dt style={{ color: "#9ca3af", width: 120 }}>{k}</dt>
          <dd style={{ color: "#e5e7eb", margin: 0 }}>{v}</dd>
        </div>
      ))}
      <div style={{ display: "flex", gap: 12, marginBottom: 6 }}>
        <dt style={{ color: "#9ca3af", width: 120 }}>KYC</dt>
        <dd style={{ color: "#22c55e", margin: 0 }}>
          {Object.values(props.kyc).filter(Boolean).length} 项已上传
        </dd>
      </div>
      {props.draft.settlement_bank_account ? (
        <div style={{ display: "flex", gap: 12 }}>
          <dt style={{ color: "#9ca3af", width: 120 }}>结算账号</dt>
          <dd style={{ color: "#22c55e", margin: 0 }}>已填写（提交后 HMAC blind_index 入库）</dd>
        </div>
      ) : null}
    </dl>
  );
}

export function SubmittedScreen({ id, onClear }: { id: number; onClear: () => void }): JSX.Element {
  const { t } = useTranslation();
  return (
    <section style={{ maxWidth: 600, margin: "64px auto", textAlign: "center" }}>
      <h1 style={{ color: "#22c55e" }}>{t("apply.success.title")}</h1>
      <p style={{ color: "#9ca3af" }}>{t("apply.success.body", { id })}</p>
      <button
        type="button"
        onClick={onClear}
        style={{
          padding: "8px 16px",
          background: "#2563eb",
          color: "#fff",
          border: 0,
          borderRadius: 4,
          cursor: "pointer",
        }}
      >
        {t("common.back_home")}
      </button>
    </section>
  );
}
