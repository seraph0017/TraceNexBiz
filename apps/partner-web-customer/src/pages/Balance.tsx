// Balance —— 当前余额展示 + 充值入口
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { Button, Card, Descriptions, Spin } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";
import { MoneyDisplay } from "@/components/MoneyDisplay";
import { getBalance } from "@/api/customer";

export function Balance(): JSX.Element {
  const { t } = useTranslation();
  const { data, isLoading } = useQuery({
    queryKey: ["customer", "balance"],
    queryFn: getBalance,
  });

  return (
    <Page
      title={t("balance.title")}
      actions={
        <Link to="/topup">
          <Button theme="solid" type="primary">
            {t("balance.topup_cta")}
          </Button>
        </Link>
      }
    >
      {isLoading ? (
        <Spin />
      ) : (
        <Card>
          <Descriptions
            data={[
              {
                key: t("balance.current"),
                value: data ? <MoneyDisplay fen={data.balance} /> : "—",
              },
              { key: t("balance.currency"), value: data?.currency ?? "CNY" },
              { key: t("balance.updated_at"), value: data?.updated_at ?? "—" },
            ]}
          />
        </Card>
      )}
    </Page>
  );
}
