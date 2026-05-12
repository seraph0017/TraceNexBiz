// useApplyPartner —— 单点封装 step navigation + submit + draft 同步
// 把 store/router/submit 业务逻辑从 view 中拆出来，view 仅渲染。
import * as React from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useThrottledSubmit } from "@/hooks/useThrottledSubmit";
import { useToast } from "@/components/Toast";
import {
  useApplyDraft,
  type ApplyStep,
  type ApplyDraftState,
} from "@/stores/applyDraft";
import {
  KycStepSchema,
  type KycStep,
} from "@/schemas/applyPartner";
import { submitPartnerApply, ApiException } from "@/api";

export const CONSENT_VERSION = "2026-05-pipl-v1";

export const STEP_KEYS = [
  "contact",
  "company",
  "scale",
  "bank",
  "kyc",
  "review",
] as const satisfies ReadonlyArray<ApplyStep>;

export function nextStep(s: ApplyStep): ApplyStep {
  const i = STEP_KEYS.indexOf(s as (typeof STEP_KEYS)[number]);
  if (i < 0 || i === STEP_KEYS.length - 1) return s;
  return STEP_KEYS[i + 1] as ApplyStep;
}

export function prevStep(s: ApplyStep): ApplyStep {
  const i = STEP_KEYS.indexOf(s as (typeof STEP_KEYS)[number]);
  if (i <= 0) return s;
  return STEP_KEYS[i - 1] as ApplyStep;
}

export interface UseApplyPartnerResult {
  // store
  draft: ApplyDraftState["draft"];
  step: ApplyStep;
  submittedId: number | null;
  setStep: ApplyDraftState["setStep"];
  patchDraft: ApplyDraftState["patchDraft"];
  clearDraft: ApplyDraftState["clearDraft"];
  // KYC (in-memory)
  kycState: KycStep;
  setKycState: React.Dispatch<React.SetStateAction<KycStep>>;
  // consent
  consent: { consent_id: number; version: string } | null;
  setConsent: React.Dispatch<
    React.SetStateAction<{ consent_id: number; version: string } | null>
  >;
  // submit
  submitting: boolean;
  submitApply: () => Promise<void>;
  // navigation
  goNext: () => void;
  goPrev: () => void;
  // misc
  navigateHome: () => void;
}

export function useApplyPartner(): UseApplyPartnerResult {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const toast = useToast();

  const draft = useApplyDraft((s: ApplyDraftState) => s.draft);
  const step = useApplyDraft((s: ApplyDraftState) => s.step);
  const submittedId = useApplyDraft((s: ApplyDraftState) => s.submittedId);
  const setStep = useApplyDraft((s: ApplyDraftState) => s.setStep);
  const patchDraft = useApplyDraft((s: ApplyDraftState) => s.patchDraft);
  const clearDraft = useApplyDraft((s: ApplyDraftState) => s.clearDraft);
  const markSubmitted = useApplyDraft((s: ApplyDraftState) => s.markSubmitted);

  const [kycState, setKycState] = React.useState<KycStep>({
    id_front_url: "",
    id_back_url: "",
    business_license_url: "",
    legal_person_face_url: "",
  });
  const [consent, setConsent] = React.useState<
    { consent_id: number; version: string } | null
  >(null);

  // 启动时恢复提示
  React.useEffect(() => {
    if (draft.contact_name && step !== "contact" && step !== "done") {
      toast.push({ severity: "info", text: t("apply.draft.restored") });
    }
    // 仅启动跑一次
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const submit = useThrottledSubmit(submitPartnerApply, { coolDownMs: 2000 });

  const submitApply = React.useCallback(async () => {
    if (!consent) {
      toast.push({ severity: "warning", text: t("apply.consent.required") });
      return;
    }
    const validKyc = KycStepSchema.safeParse(kycState);
    if (!validKyc.success) {
      toast.push({ severity: "warning", text: t("errors.valid.invalid_input") });
      setStep("kyc");
      return;
    }
    try {
      const result = await submit.submit({
        type: (draft.type ?? "enterprise") as "individual" | "enterprise",
        contact_name: draft.contact_name ?? "",
        contact_phone: draft.contact_phone ?? "",
        contact_email: draft.contact_email ?? "",
        consent_id: consent.consent_id,
        consent_text_version: consent.version,
        company_name: draft.company_name,
        unified_social_credit_code: draft.unified_social_credit_code,
        legal_person_id: draft.legal_person_id,
        expected_monthly_calls: draft.expected_monthly_calls,
        expected_use_case: draft.expected_use_case,
        source_channel: draft.source_channel,
        // 银行 / 税务（Fix-C item 5/6）
        tax_status: draft.tax_status,
        settlement_bank_name: draft.settlement_bank_name,
        settlement_bank_account: draft.settlement_bank_account,
        settlement_account_holder: draft.settlement_account_holder,
      });
      if (result) {
        markSubmitted(result.id);
      }
    } catch (err: unknown) {
      const msg =
        err instanceof ApiException
          ? err.message || t("errors.unknown")
          : err instanceof Error
            ? err.message
            : t("errors.unknown");
      toast.push({ severity: "error", text: msg });
    }
  }, [consent, kycState, draft, submit, markSubmitted, setStep, t, toast]);

  return {
    draft,
    step,
    submittedId,
    setStep,
    patchDraft,
    clearDraft,
    kycState,
    setKycState,
    consent,
    setConsent,
    submitting: submit.state.submitting,
    submitApply,
    goNext: () => setStep(nextStep(step)),
    goPrev: () => setStep(prevStep(step)),
    navigateHome: () => {
      clearDraft();
      navigate("/");
    },
  };
}
