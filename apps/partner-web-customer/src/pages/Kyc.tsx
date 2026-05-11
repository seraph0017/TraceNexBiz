// KYC —— 客户实名认证；身份证 / 真实姓名 / 手机
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Banner, Button, Card, Input, Spin, Tag } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import { getKycStatus, submitKyc } from "@/api/customer";
import { useApiToast } from "@/hooks/useApiToast";

export function Kyc(): JSX.Element {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const [realName, setRealName] = useState("");
  const [idCard, setIdCard] = useState("");
  const [phone, setPhone] = useState("");

  const { data, isLoading } = useQuery({
    queryKey: ["customer", "kyc"],
    queryFn: getKycStatus,
  });

  const mut = useMutation({
    mutationFn: submitKyc,
    onSuccess: () => {
      showSuccess(t("app.submit"));
      void qc.invalidateQueries({ queryKey: ["customer", "kyc"] });
    },
    onError: showError,
  });

  if (isLoading) return <Spin />;
  const status = data?.status ?? "none";
  const canEdit = status === "none" || status === "rejected";

  return (
    <Page title={t("kyc.title")}>
      <Card>
        <div style={{ marginBottom: 12 }}>
          {t("kyc.title")}:{" "}
          <Tag color={status === "approved" ? "green" : status === "rejected" ? "red" : "amber"}>
            {t(`kyc.status_${status}`)}
          </Tag>
        </div>
        {status === "rejected" && data?.reject_reason && (
          <Banner
            type="danger"
            description={t("kyc.reject_reason", { reason: data.reject_reason })}
            closeIcon={null}
          />
        )}
        {canEdit && (
          <div style={{ marginTop: 12, maxWidth: 480 }}>
            <Field label={t("kyc.real_name")}>
              <Input value={realName} onChange={setRealName} aria-label="real-name" />
            </Field>
            <Field label={t("kyc.id_card")}>
              <Input value={idCard} onChange={setIdCard} aria-label="id-card" />
            </Field>
            <Field label={t("kyc.phone")}>
              <Input value={phone} onChange={setPhone} aria-label="phone" />
            </Field>
            <Button
              theme="solid"
              type="primary"
              loading={mut.isPending}
              onClick={() => mut.mutate({ real_name: realName, id_card: idCard, phone })}
            >
              {t("kyc.submit")}
            </Button>
          </div>
        )}
      </Card>
    </Page>
  );
}
