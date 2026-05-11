// 步骤指示器 —— 视觉化当前进度

export interface StepInfo {
  key: string;
  label: string;
}

interface StepperProps {
  steps: ReadonlyArray<StepInfo>;
  current: string;
}

export function Stepper({ steps, current }: StepperProps): JSX.Element {
  const idx = steps.findIndex((s) => s.key === current);
  return (
    <ol
      aria-label="申请进度"
      style={{ display: "flex", gap: 12, padding: 0, listStyle: "none", marginBottom: 24 }}
    >
      {steps.map((s, i) => {
        const done = i < idx;
        const active = i === idx;
        return (
          <li
            key={s.key}
            aria-current={active ? "step" : undefined}
            style={{
              display: "flex",
              alignItems: "center",
              gap: 6,
              color: active ? "#fff" : done ? "#22c55e" : "#6b7280",
            }}
          >
            <span
              style={{
                width: 24,
                height: 24,
                borderRadius: 999,
                background: active ? "#2563eb" : done ? "#16a34a" : "#1f2937",
                color: "#fff",
                display: "inline-flex",
                alignItems: "center",
                justifyContent: "center",
                fontSize: 12,
              }}
            >
              {done ? "✓" : i + 1}
            </span>
            <span>{s.label}</span>
          </li>
        );
      })}
    </ol>
  );
}
