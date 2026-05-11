// 受控 form 字段封装 —— 由于 Semi UI Form.* 组件 value/onChange 由 form context 控制
// 本项目使用平面 controlled state（不进 Form context），直接 wrap label + 输入控件
import * as React from "react";

interface FieldProps {
  label: React.ReactNode;
  children: React.ReactNode;
  error?: string;
  hint?: string;
}

export function Field({ label, children, error, hint }: FieldProps): JSX.Element {
  return (
    <label style={{ display: "block", marginBottom: 12 }}>
      <div style={{ marginBottom: 4, fontSize: 13, color: "#374151" }}>{label}</div>
      {children}
      {hint && !error && (
        <div style={{ marginTop: 4, fontSize: 12, color: "#6b7280" }}>{hint}</div>
      )}
      {error && (
        <div role="alert" style={{ marginTop: 4, fontSize: 12, color: "#dc2626" }}>
          {error}
        </div>
      )}
    </label>
  );
}
