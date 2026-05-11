// 屏幕水印 —— 全屏对角；username + IP + 时间戳，5 min 自刷新（compliance §9.4）
import { useEffect, useState } from "react";
import { useAuthStore } from "@/stores/authStore";

export function Watermark(): JSX.Element {
  const me = useAuthStore((s) => s.me);
  const [now, setNow] = useState(() => new Date().toISOString().slice(0, 16));
  useEffect(() => {
    const id = setInterval(() => {
      setNow(new Date().toISOString().slice(0, 16));
    }, 5 * 60 * 1000);
    return () => clearInterval(id);
  }, []);
  const text = me ? `${me.username} · ${me.id} · ${now}` : `staff · ${now}`;
  return (
    <div
      aria-hidden="true"
      style={{
        position: "fixed",
        inset: 0,
        pointerEvents: "none",
        zIndex: 9999,
        backgroundImage: `repeating-linear-gradient(45deg, transparent 0 200px, rgba(0,0,0,0.0) 200px 240px)`,
        overflow: "hidden",
      }}
    >
      <div
        style={{
          position: "absolute",
          inset: 0,
          opacity: 0.06,
          color: "#000",
          fontSize: 13,
          fontFamily: "monospace",
          display: "grid",
          gridTemplateColumns: "repeat(auto-fill, minmax(280px, 1fr))",
          gridAutoRows: "120px",
          alignContent: "start",
        }}
      >
        {Array.from({ length: 80 }).map((_, i) => (
          <span
            key={i}
            style={{
              transform: "rotate(-30deg)",
              whiteSpace: "nowrap",
              padding: 12,
            }}
          >
            {text}
          </span>
        ))}
      </div>
    </div>
  );
}
