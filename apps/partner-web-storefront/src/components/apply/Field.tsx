// 通用受控字段 —— 与 react-hook-form 通过 register 接入
import * as React from "react";
import type { FieldError, UseFormRegisterReturn } from "react-hook-form";
import { useTranslation } from "react-i18next";

export interface FieldProps {
  id: string;
  label: string;
  required?: boolean;
  type?: string;
  placeholder?: string;
  hint?: string;
  error?: FieldError;
  registration: UseFormRegisterReturn;
  multiline?: boolean;
  autoComplete?: string;
  inputMode?: React.HTMLAttributes<HTMLInputElement>["inputMode"];
}

export function Field(props: FieldProps): JSX.Element {
  const { t } = useTranslation();
  const errorText = props.error?.message ? translateError(props.error.message, t) : undefined;
  const errorId = `${props.id}-err`;
  const hintId = `${props.id}-hint`;
  const describedBy = [props.hint ? hintId : null, errorText ? errorId : null]
    .filter(Boolean)
    .join(" ") || undefined;
  const baseStyle: React.CSSProperties = {
    width: "100%",
    boxSizing: "border-box",
    padding: "8px 10px",
    background: "#0f1722",
    border: `1px solid ${errorText ? "#f87171" : "#1f2937"}`,
    borderRadius: 4,
    color: "#e5e7eb",
    fontSize: 14,
  };
  return (
    <div style={{ marginBottom: 12 }}>
      <label htmlFor={props.id} style={{ display: "block", color: "#cbd5e1", marginBottom: 4 }}>
        {props.label} {props.required ? <span style={{ color: "#f87171" }}>*</span> : null}
      </label>
      {props.multiline ? (
        <textarea
          id={props.id}
          rows={4}
          placeholder={props.placeholder}
          style={baseStyle}
          aria-invalid={Boolean(errorText)}
          aria-describedby={describedBy}
          {...props.registration}
        />
      ) : (
        <input
          id={props.id}
          type={props.type ?? "text"}
          placeholder={props.placeholder}
          autoComplete={props.autoComplete}
          inputMode={props.inputMode}
          style={baseStyle}
          aria-invalid={Boolean(errorText)}
          aria-describedby={describedBy}
          {...props.registration}
        />
      )}
      {props.hint ? (
        <div id={hintId} style={{ fontSize: 12, color: "#9ca3af", marginTop: 4 }}>
          {props.hint}
        </div>
      ) : null}
      {errorText ? (
        <div id={errorId} role="alert" style={{ fontSize: 12, color: "#f87171", marginTop: 4 }}>
          {errorText}
        </div>
      ) : null}
    </div>
  );
}

function translateError(message: string, t: (k: string, opts?: Record<string, unknown>) => string): string {
  // 错误信息可能是 i18n key（来自 zod refine）或原始字符串
  if (message.startsWith("validation.") || message.startsWith("apply.")) {
    return t(message);
  }
  return message;
}
