// PiiField — frontend §9 PII 默认脱敏展示组件 (e.g. 身份证 / 手机号 / 银行卡).
//
// W0 scaffold：默认渲染 mask（只显示首尾 2 位 + ***）；含 reveal 按钮 + audit_log 钩子（W1e 接 useElevatedAudit）.
import * as React from 'react';

export interface PiiFieldProps {
  value: string;
  /** 'phone' | 'idcard' | 'email' | 'bank' */
  kind?: 'phone' | 'idcard' | 'email' | 'bank';
  revealable?: boolean;
  onReveal?: () => void;
}

export const PiiField: React.FC<PiiFieldProps> = ({ value, kind = 'phone', revealable, onReveal }) => {
  const [revealed, setRevealed] = React.useState(false);
  const masked = mask(value, kind);
  const display = revealed ? value : masked;
  return (
    <span data-testid="pii-field" data-kind={kind}>
      {display}
      {revealable ? (
        <button
          type="button"
          onClick={() => {
            setRevealed((v) => !v);
            // TODO(W1e): elevated audit_log fire on reveal.
            onReveal?.();
          }}
        >
          {revealed ? '隐藏' : '显示'}
        </button>
      ) : null}
    </span>
  );
};

function mask(value: string, kind: PiiFieldProps['kind']): string {
  if (!value) return '';
  if (kind === 'email') {
    const [local, domain] = value.split('@');
    if (!domain || !local) return '***';
    return `${local.slice(0, 1)}***@${domain}`;
  }
  if (value.length <= 4) return '***';
  return `${value.slice(0, 2)}****${value.slice(-2)}`;
}
