// ApplyPartner —— 多步招商申请（PRD §4.1 场景 A/B happy path）
//
// 步骤：联系人 → 公司信息 → 业务规模 + 银行账户 → KYC 上传 → 审阅 + 单独同意 + 提交
//
// 状态：Zustand persist（localStorage）保存非敏感字段，KYC URL / 身份证 / 银行账号 / consent 仅在内存
// 表单：每步一个 react-hook-form scope，用 zod resolver；切步前手动 validate
// 提交：useThrottledSubmit 防止重复点击 + Idempotency-Key（client.ts 自动注入）
//
// 拆分后结构：
//   index.tsx              页面壳 + Stepper + 路由
//   Step1Basics.tsx        联系人
//   Step2KYC.tsx           公司信息 / 法人身份证
//   Step3Bank.tsx          业务规模 + 结算银行 + tax_status
//   Step4KYC.tsx           身份证 / 营业执照 / 法人面照 上传
//   Step5Review.tsx        Summary + ConsentBox + Submit
//   useApplyPartner.ts     state + submit hook
import * as React from "react";
import { useTranslation } from "react-i18next";
import { useSeo } from "@/hooks/useSeo";
import { Stepper } from "@/components/apply/Stepper";
import { Step1Basics } from "./Step1Basics";
import { Step2KYC } from "./Step2KYC";
import { Step3Bank, type Step3Form } from "./Step3Bank";
import { Step4KYC } from "./Step4KYC";
import { Step5Review, SubmittedScreen } from "./Step5Review";
import { useApplyPartner, CONSENT_VERSION } from "./useApplyPartner";

export function ApplyPartner(): JSX.Element {
  const { t } = useTranslation();
  useSeo({
    title: `${t("apply.title")} | ${t("app.title")}`,
    description: t("apply.subtitle"),
    canonical: "https://partner.tracenex.cn/apply-partner",
    robots: "index,follow",
  });

  const a = useApplyPartner();

  const stepInfo = React.useMemo(
    () => [
      { key: "contact", label: t("apply.step.contact") },
      { key: "company", label: t("apply.step.company") },
      { key: "scale", label: t("apply.step.scale") },
      { key: "bank", label: "银行 / 税务" },
      { key: "kyc", label: t("apply.step.kyc") },
      { key: "review", label: t("apply.step.review") },
    ],
    [t],
  );

  if (a.step === "done" && a.submittedId) {
    return <SubmittedScreen id={a.submittedId} onClear={a.navigateHome} />;
  }

  return (
    <section style={{ maxWidth: 720, margin: "0 auto" }}>
      <h1 style={{ color: "#fff", margin: 0 }}>{t("apply.title")}</h1>
      <p style={{ color: "#9ca3af" }}>{t("apply.subtitle")}</p>
      <Stepper steps={stepInfo} current={a.step === "done" ? "review" : a.step} />

      {a.step === "contact" ? (
        <Step1Basics
          defaultValues={{
            type: a.draft.type ?? "enterprise",
            contact_name: a.draft.contact_name ?? "",
            contact_phone: a.draft.contact_phone ?? "",
            contact_email: a.draft.contact_email ?? "",
            source_channel: a.draft.source_channel ?? "",
          }}
          onSubmit={(data) => {
            a.patchDraft(data);
            a.goNext();
          }}
        />
      ) : null}

      {a.step === "company" ? (
        <Step2KYC
          defaultValues={{
            type: a.draft.type ?? "enterprise",
            company_name: a.draft.company_name ?? "",
            unified_social_credit_code: a.draft.unified_social_credit_code ?? "",
            legal_person_id: a.draft.legal_person_id ?? "",
          }}
          onPrev={a.goPrev}
          onSubmit={(data) => {
            a.patchDraft(data);
            a.goNext();
          }}
        />
      ) : null}

      {a.step === "scale" || a.step === "bank" ? (
        // step3 合并 业务规模 + 银行；保留 "scale" / "bank" 两个 store 值兼容性
        <Step3Bank
          defaultValues={{
            expected_monthly_calls: a.draft.expected_monthly_calls ?? 0,
            expected_use_case: a.draft.expected_use_case ?? "",
            tax_status: a.draft.tax_status ?? "individual",
            settlement_bank_name: a.draft.settlement_bank_name ?? "",
            settlement_bank_account: a.draft.settlement_bank_account ?? "",
            settlement_account_holder: a.draft.settlement_account_holder ?? "",
          }}
          onPrev={a.goPrev}
          onSubmit={(data: Step3Form) => {
            a.patchDraft(data);
            // scale + bank 合并；直接跳到 kyc
            a.setStep("kyc");
          }}
        />
      ) : null}

      {a.step === "kyc" ? (
        <Step4KYC
          value={a.kycState}
          isEnterprise={(a.draft.type ?? "enterprise") === "enterprise"}
          onChange={a.setKycState}
          onPrev={() => a.setStep("bank")}
          onNext={a.goNext}
        />
      ) : null}

      {a.step === "review" ? (
        <Step5Review
          draft={a.draft}
          kyc={a.kycState}
          consent={a.consent}
          onConsent={a.setConsent}
          consentVersion={CONSENT_VERSION}
          onPrev={a.goPrev}
          submitting={a.submitting}
          onSubmit={a.submitApply}
        />
      ) : null}
    </section>
  );
}
