// OrphanNotice 场景 I —— 30 天宽限 + adopt / direct / switch
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Banner, Button, Card, Radio, RadioGroup, Select, Spin } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { Page } from "@/components/Page";
import { Field } from "@/components/Field";
import { getOrphanState, chooseOrphanOption } from "@/api/customer";
import { useApiToast } from "@/hooks/useApiToast";

export function OrphanNotice(): JSX.Element {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { showError, showSuccess } = useApiToast();
  const [decision, setDecision] = useState<"adopt" | "direct" | "switch">("adopt");
  const [partner, setPartner] = useState<number | undefined>();

  const { data, isLoading } = useQuery({
    queryKey: ["customer", "orphan"],
    queryFn: getOrphanState,
  });

  const mut = useMutation({
    mutationFn: chooseOrphanOption,
    onSuccess: () => {
      showSuccess(t("orphan.submitted"));
      void qc.invalidateQueries({ queryKey: ["customer", "me"] });
    },
    onError: showError,
  });

  if (isLoading || !data) return <Spin />;

  return (
    <Page title={t("orphan.title")}>
      <Banner
        type="warning"
        description={t("orphan.intro", { at: data.orphaned_at ?? "—" })}
        closeIcon={null}
      />
      <Card style={{ marginTop: 12 }}>
        <p>{t("orphan.grace_until", { ts: data.grace_period_ends_at ?? "—" })}</p>
        <h3>{t("orphan.options_title")}</h3>
        <RadioGroup value={decision} onChange={(e) => setDecision(e.target.value as "adopt" | "direct" | "switch")}>
          {data.options.includes("adopt") && <Radio value="adopt">{t("orphan.opt_adopt")}</Radio>}
          {data.options.includes("direct") && <Radio value="direct">{t("orphan.opt_direct")}</Radio>}
          {data.options.includes("switch") && <Radio value="switch">{t("orphan.opt_switch")}</Radio>}
        </RadioGroup>
        {decision === "switch" && (
          <Field label={t("orphan.select_partner")}>
            <Select value={partner} onChange={(v) => setPartner(v as number)} style={{ minWidth: 240 }}>
              {data.candidate_partners.map((p) => (
                <Select.Option key={p.id} value={p.id}>
                  {p.name}
                </Select.Option>
              ))}
            </Select>
          </Field>
        )}
        <Button
          theme="solid"
          type="primary"
          style={{ marginTop: 12 }}
          loading={mut.isPending}
          disabled={decision === "switch" && !partner}
          onClick={() => mut.mutate({ decision, target_partner_id: decision === "switch" ? partner : undefined })}
        >
          {t("orphan.confirm")}
        </Button>
      </Card>
    </Page>
  );
}
