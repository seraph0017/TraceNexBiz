// 简易 toast —— 不依赖 Semi UI 全量包；
// 后续 W1f/W1g 切到 Semi @douyinfe/semi-ui Toast 时同 API 兼容
import * as React from "react";

export interface ToastItem {
  id: number;
  text: string;
  severity: "error" | "warning" | "info" | "success";
  ttl?: number;
}

interface ToastContextValue {
  push(item: Omit<ToastItem, "id">): void;
  dismiss(id: number): void;
}

const ToastContext = React.createContext<ToastContextValue | null>(null);

export function ToastProvider({ children }: { children: React.ReactNode }): JSX.Element {
  const [items, setItems] = React.useState<ToastItem[]>([]);
  const idRef = React.useRef(0);

  const dismiss = React.useCallback((id: number) => {
    setItems((list) => list.filter((it) => it.id !== id));
  }, []);

  const push = React.useCallback(
    (item: Omit<ToastItem, "id">) => {
      idRef.current += 1;
      const id = idRef.current;
      const ttl = item.ttl ?? 4000;
      setItems((list) => [...list, { ...item, id }]);
      window.setTimeout(() => dismiss(id), ttl);
    },
    [dismiss],
  );

  const value = React.useMemo<ToastContextValue>(() => ({ push, dismiss }), [push, dismiss]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div
        aria-live="polite"
        aria-atomic="true"
        style={{
          position: "fixed",
          top: 16,
          right: 16,
          zIndex: 1000,
          display: "flex",
          flexDirection: "column",
          gap: 8,
        }}
      >
        {items.map((it) => (
          <div
            key={it.id}
            role={it.severity === "error" ? "alert" : "status"}
            data-testid="toast"
            data-severity={it.severity}
            style={{
              minWidth: 240,
              maxWidth: 360,
              background: severityColor(it.severity),
              color: "#fff",
              padding: "10px 14px",
              borderRadius: 6,
              fontSize: 14,
              boxShadow: "0 4px 12px rgba(0,0,0,0.3)",
            }}
          >
            {it.text}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastContextValue {
  const ctx = React.useContext(ToastContext);
  if (!ctx) throw new Error("useToast must be used within ToastProvider");
  return ctx;
}

function severityColor(s: ToastItem["severity"]): string {
  switch (s) {
    case "error":
      return "#dc2626";
    case "warning":
      return "#d97706";
    case "success":
      return "#16a34a";
    case "info":
    default:
      return "#2563eb";
  }
}
