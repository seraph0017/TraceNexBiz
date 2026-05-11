// PIPL 单独同意 —— 必须用户主动勾选；勾选时调用 /api/public/consent 落库
import * as React from "react";
import { useTranslation } from "react-i18next";
import { submitConsent } from "@/api";

export interface ConsentBoxProps {
  onAccepted: (consent: { consent_id: number; version: string }) => void;
  /** 已经接受过则隐藏 */
  accepted: boolean;
  /** 服务端期望的 consent template version */
  version: string;
}

export function ConsentBox(props: ConsentBoxProps): JSX.Element {
  const { t } = useTranslation();
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);

  const onChange = React.useCallback(
    async (e: React.ChangeEvent<HTMLInputElement>) => {
      if (!e.target.checked) return;
      setBusy(true);
      setError(null);
      try {
        const rec = await submitConsent({
          scope: "partner_apply.sensitive_pi",
          version: props.version,
          granted: true,
        });
        props.onAccepted({ consent_id: rec.consent_id, version: rec.version });
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : t("errors.network");
        setError(msg);
        e.target.checked = false;
      } finally {
        setBusy(false);
      }
    },
    [props, t],
  );

  return (
    <fieldset
      style={{
        border: "1px solid #1f2937",
        borderRadius: 6,
        padding: 16,
        background: "#11151c",
        marginBottom: 16,
      }}
    >
      <legend style={{ color: "#fde68a", padding: "0 8px" }}>{t("apply.consent.title")}</legend>
      <p style={{ color: "#9ca3af", fontSize: 13, lineHeight: 1.7 }}>{t("apply.consent.body")}</p>
      <label style={{ display: "flex", alignItems: "flex-start", gap: 8, color: "#e5e7eb" }}>
        <input
          type="checkbox"
          required
          disabled={busy || props.accepted}
          checked={props.accepted}
          onChange={onChange}
          aria-describedby="pipl-consent-desc"
        />
        <span id="pipl-consent-desc">{t("apply.consent.checkbox")}</span>
      </label>
      {error ? (
        <div role="alert" style={{ color: "#f87171", fontSize: 12, marginTop: 8 }}>
          {error}
        </div>
      ) : null}
    </fieldset>
  );
}
