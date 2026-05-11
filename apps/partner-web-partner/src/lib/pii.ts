// PII mask helpers —— 与 storefront schemas 中实现保持兼容
export function maskPhone(phone: string): string {
  if (!phone) return "";
  const trimmed = phone.replace(/\s+/g, "");
  if (trimmed.length <= 4) return "****";
  return trimmed.slice(0, 3) + "****" + trimmed.slice(-4);
}

export function maskEmail(email: string): string {
  const at = email.indexOf("@");
  if (at <= 0) return email;
  const head = email.slice(0, at);
  const tail = email.slice(at);
  if (head.length <= 2) return "*" + tail;
  return head[0] + "***" + head[head.length - 1] + tail;
}

export function maskIdCard(id: string): string {
  if (!id) return "";
  const t = id.replace(/\s+/g, "");
  if (t.length < 8) return "****";
  return t.slice(0, 4) + "**********" + t.slice(-4);
}

export function maskBankAccount(acct: string): string {
  if (!acct) return "";
  const t = acct.replace(/\s+/g, "");
  if (t.length <= 4) return "****";
  return "**** **** **** " + t.slice(-4);
}

/** 分 → 元 (zh-CN 千分位) */
export function fenToYuan(fen: number): string {
  const yuan = fen / 100;
  return yuan.toLocaleString("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}
