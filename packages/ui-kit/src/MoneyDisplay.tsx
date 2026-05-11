// MoneyDisplay — quota → ¥ 格式化（PRD §20）.
import * as React from 'react';

export interface MoneyDisplayProps {
  /** quota 单位（与 partner_db.partner_wallet.balance 一致；BIGINT，单位 = "Fy-api 内部 quota point"） */
  quota: number;
  /** 转换成人民币的兑换比例；W1e 从 biz_setting 读 */
  ratio?: number;
  /** locale */
  locale?: 'zh-CN' | 'en-US';
}

export const MoneyDisplay: React.FC<MoneyDisplayProps> = ({ quota, ratio = 500_000, locale = 'zh-CN' }) => {
  const cny = quota / ratio;
  return (
    <span data-testid="money-display">
      {locale === 'zh-CN'
        ? `¥${cny.toFixed(2)}`
        : new Intl.NumberFormat('en-US', { style: 'currency', currency: 'CNY' }).format(cny)}
    </span>
  );
};
