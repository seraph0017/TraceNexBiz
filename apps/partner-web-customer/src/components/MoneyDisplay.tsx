// MoneyDisplay —— 分 → 元，带千分位 + 货币符号
import { fenToYuan } from "@/lib/pii";

interface Props {
  fen: number;
  currency?: string;
  className?: string;
}

export function MoneyDisplay({ fen, currency = "¥", className }: Props): JSX.Element {
  return (
    <span className={className} style={{ fontVariantNumeric: "tabular-nums" }}>
      {currency} {fenToYuan(fen)}
    </span>
  );
}
