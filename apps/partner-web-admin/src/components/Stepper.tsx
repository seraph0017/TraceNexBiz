// 多步骤进度指示器 —— for saga 三阶段 UI
interface Step {
  label: string;
  status: "pending" | "active" | "done" | "failed";
}

export function Stepper({ steps }: { steps: Step[] }): JSX.Element {
  return (
    <ol
      style={{
        display: "flex",
        flexDirection: "column",
        gap: 12,
        listStyle: "none",
        padding: 0,
        margin: 0,
      }}
      aria-label="saga progress"
    >
      {steps.map((s, i) => (
        <li
          key={i}
          aria-current={s.status === "active" ? "step" : undefined}
          style={{ display: "flex", alignItems: "center", gap: 12, color: colorOf(s.status) }}
        >
          <span style={badgeStyle(s.status)}>{iconOf(s.status, i + 1)}</span>
          <span>{s.label}</span>
        </li>
      ))}
    </ol>
  );
}

function colorOf(s: Step["status"]): string {
  switch (s) {
    case "done":
      return "#16a34a";
    case "active":
      return "#1d4ed8";
    case "failed":
      return "#dc2626";
    default:
      return "#9ca3af";
  }
}

function badgeStyle(s: Step["status"]): React.CSSProperties {
  return {
    display: "inline-flex",
    width: 24,
    height: 24,
    borderRadius: "50%",
    alignItems: "center",
    justifyContent: "center",
    fontSize: 12,
    background:
      s === "done"
        ? "#dcfce7"
        : s === "active"
          ? "#dbeafe"
          : s === "failed"
            ? "#fee2e2"
            : "#f3f4f6",
    color: colorOf(s),
    border: `1px solid ${colorOf(s)}`,
  };
}

function iconOf(s: Step["status"], n: number): string {
  if (s === "done") return "✓";
  if (s === "failed") return "!";
  return String(n);
}
