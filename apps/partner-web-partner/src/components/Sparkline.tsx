// 简易 SVG sparkline —— Dashboard 30 天趋势；不引入 ECharts/Recharts 体积
interface Props {
  data: { date: string; net: number }[];
  width?: number;
  height?: number;
}

export function Sparkline({ data, width = 600, height = 160 }: Props): JSX.Element {
  if (data.length === 0) {
    return (
      <div style={{ height, display: "flex", alignItems: "center", justifyContent: "center", color: "#9ca3af" }}>
        无数据
      </div>
    );
  }
  const padding = 24;
  const maxV = Math.max(...data.map((d) => d.net), 1);
  const minV = Math.min(...data.map((d) => d.net), 0);
  const span = maxV - minV || 1;
  const stepX = (width - padding * 2) / Math.max(data.length - 1, 1);
  const points = data.map((d, i) => {
    const x = padding + i * stepX;
    const y = height - padding - ((d.net - minV) / span) * (height - padding * 2);
    return [x, y] as const;
  });
  const path = points.map(([x, y], i) => `${i === 0 ? "M" : "L"}${x.toFixed(1)},${y.toFixed(1)}`).join(" ");
  const area = `${path} L${points[points.length - 1]?.[0]},${height - padding} L${points[0]?.[0]},${height - padding} Z`;
  return (
    <svg
      width={width}
      height={height}
      role="img"
      aria-label="30 天净收益走势"
      style={{ display: "block", maxWidth: "100%" }}
    >
      <path d={area} fill="rgba(29, 78, 216, 0.12)" />
      <path d={path} fill="none" stroke="#1d4ed8" strokeWidth={2} />
      {points.map(([x, y], i) => (
        <circle key={i} cx={x} cy={y} r={2} fill="#1d4ed8" />
      ))}
    </svg>
  );
}
