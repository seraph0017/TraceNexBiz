// 通用 page 容器 —— 标题 + 操作区 + content
import * as React from "react";

interface PageProps {
  title: React.ReactNode;
  actions?: React.ReactNode;
  description?: React.ReactNode;
  children: React.ReactNode;
}

export function Page({ title, actions, description, children }: PageProps): JSX.Element {
  return (
    <div>
      <header
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          marginBottom: 16,
        }}
      >
        <div>
          <h1 style={{ margin: 0, fontSize: 22 }}>{title}</h1>
          {description && (
            <p style={{ margin: "4px 0 0", color: "#6b7280", fontSize: 13 }}>{description}</p>
          )}
        </div>
        {actions && <div style={{ display: "flex", gap: 8 }}>{actions}</div>}
      </header>
      <section>{children}</section>
    </div>
  );
}
