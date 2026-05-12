// 共享的步骤导航按钮（next / prev / submit）
import { useTranslation } from "react-i18next";

export function FormButtons(props: {
  next?: boolean;
  prev?: boolean;
  submit?: boolean;
  submitLabel?: string;
  disabled?: boolean;
  onPrev?: () => void;
  onNext?: () => void;
  onSubmit?: () => void | Promise<void>;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <div style={{ display: "flex", gap: 8, marginTop: 16 }}>
      {props.prev ? (
        <button
          type="button"
          onClick={props.onPrev}
          style={{
            padding: "8px 16px",
            background: "transparent",
            color: "#cbd5e1",
            border: "1px solid #2a2f36",
            borderRadius: 4,
            cursor: "pointer",
          }}
        >
          {t("apply.prev")}
        </button>
      ) : null}
      {props.next ? (
        <button
          type={props.onNext ? "button" : "submit"}
          onClick={props.onNext}
          disabled={props.disabled}
          style={{
            padding: "8px 16px",
            background: "#2563eb",
            color: "#fff",
            border: 0,
            borderRadius: 4,
            cursor: props.disabled ? "not-allowed" : "pointer",
            opacity: props.disabled ? 0.5 : 1,
          }}
        >
          {t("apply.next")}
        </button>
      ) : null}
      {props.submit ? (
        <button
          type="button"
          onClick={() => props.onSubmit?.()}
          disabled={props.disabled}
          style={{
            padding: "8px 16px",
            background: "#16a34a",
            color: "#fff",
            border: 0,
            borderRadius: 4,
            cursor: props.disabled ? "not-allowed" : "pointer",
            opacity: props.disabled ? 0.5 : 1,
          }}
        >
          {props.submitLabel ?? t("apply.submit")}
        </button>
      ) : null}
    </div>
  );
}
