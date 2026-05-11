// ApplyPartner —— 多步招商申请（PRD §4.1 场景 A/B happy path）
//
// 步骤：联系人 → 公司信息 → 业务规模 → KYC 上传 → 单独同意 + 提交 → 等待审核
//
// 状态：Zustand persist（localStorage）保存非敏感字段，KYC URL / 身份证 / consent 仅在内存
// 表单：每步一个 react-hook-form scope，用 zod resolver；切步前手动 validate
// 提交：useThrottledSubmit 防止重复点击 + Idempotency-Key（client.ts 自动注入）
import * as React from "react";
import { useNavigate } from "react-router-dom";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { zodResolver } from "@/lib/zodResolver";

import { useSeo } from "@/hooks/useSeo";
import { useThrottledSubmit } from "@/hooks/useThrottledSubmit";
import { useToast } from "@/components/Toast";
import { useApplyDraft, type ApplyStep, type ApplyDraftState } from "@/stores/applyDraft";
import { Field } from "@/components/apply/Field";
import { Stepper } from "@/components/apply/Stepper";
import { KycUploader } from "@/components/KycUploader";
import { ConsentBox } from "@/components/apply/ConsentBox";
import {
  ContactStepSchema,
  CompanyStepSchema,
  ScaleStepSchema,
  KycStepSchema,
  type ContactStep,
  type CompanyStep,
  type ScaleStep,
  type KycStep,
} from "@/schemas/applyPartner";
import { submitPartnerApply, ApiException } from "@/api";

const CONSENT_VERSION = "2026-05-pipl-v1";

const STEP_KEYS = ["contact", "company", "scale", "kyc", "review"] as const satisfies ReadonlyArray<ApplyStep>;

function nextStep(s: ApplyStep): ApplyStep {
  const i = STEP_KEYS.indexOf(s as (typeof STEP_KEYS)[number]);
  if (i < 0 || i === STEP_KEYS.length - 1) return s;
  return STEP_KEYS[i + 1] as ApplyStep;
}
function prevStep(s: ApplyStep): ApplyStep {
  const i = STEP_KEYS.indexOf(s as (typeof STEP_KEYS)[number]);
  if (i <= 0) return s;
  return STEP_KEYS[i - 1] as ApplyStep;
}

export function ApplyPartner(): JSX.Element {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const toast = useToast();
  useSeo({
    title: `${t("apply.title")} | ${t("app.title")}`,
    description: t("apply.subtitle"),
    canonical: "https://partner.tracenex.cn/apply-partner",
    robots: "index,follow",
  });

  const draft = useApplyDraft((s: ApplyDraftState) => s.draft);
  const step = useApplyDraft((s: ApplyDraftState) => s.step);
  const submittedId = useApplyDraft((s: ApplyDraftState) => s.submittedId);
  const setStep = useApplyDraft((s: ApplyDraftState) => s.setStep);
  const patchDraft = useApplyDraft((s: ApplyDraftState) => s.patchDraft);
  const clearDraft = useApplyDraft((s: ApplyDraftState) => s.clearDraft);
  const markSubmitted = useApplyDraft((s: ApplyDraftState) => s.markSubmitted);

  // KYC URL / consent 仅在内存（不写 localStorage）
  const [kycState, setKycState] = React.useState<KycStep>({
    id_front_url: "",
    id_back_url: "",
    business_license_url: "",
    legal_person_face_url: "",
  });
  const [consent, setConsent] = React.useState<{ consent_id: number; version: string } | null>(
    null,
  );

  const stepInfo = React.useMemo(
    () => [
      { key: "contact", label: t("apply.step.contact") },
      { key: "company", label: t("apply.step.company") },
      { key: "scale", label: t("apply.step.scale") },
      { key: "kyc", label: t("apply.step.kyc") },
      { key: "review", label: t("apply.step.review") },
    ],
    [t],
  );

  // Restore notification
  React.useEffect(() => {
    if (draft.contact_name && step !== "contact" && step !== "done") {
      toast.push({ severity: "info", text: t("apply.draft.restored") });
    }
    // eslint 禁用：仅启动时跑一次
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const submit = useThrottledSubmit(submitPartnerApply, { coolDownMs: 2000 });

  if (step === "done" && submittedId) {
    return (
      <SubmittedScreen
        id={submittedId}
        onClear={() => {
          clearDraft();
          navigate("/");
        }}
      />
    );
  }

  return (
    <section style={{ maxWidth: 720, margin: "0 auto" }}>
      <h1 style={{ color: "#fff", margin: 0 }}>{t("apply.title")}</h1>
      <p style={{ color: "#9ca3af" }}>{t("apply.subtitle")}</p>
      <Stepper steps={stepInfo} current={step === "done" ? "review" : step} />

      {step === "contact" ? (
        <ContactForm
          defaultValues={{
            type: draft.type ?? "enterprise",
            contact_name: draft.contact_name ?? "",
            contact_phone: draft.contact_phone ?? "",
            contact_email: draft.contact_email ?? "",
            source_channel: draft.source_channel ?? "",
          }}
          onSubmit={(data) => {
            patchDraft(data);
            setStep(nextStep(step));
          }}
        />
      ) : null}

      {step === "company" ? (
        <CompanyForm
          defaultValues={{
            type: draft.type ?? "enterprise",
            company_name: draft.company_name ?? "",
            unified_social_credit_code: draft.unified_social_credit_code ?? "",
            legal_person_id: draft.legal_person_id ?? "",
          }}
          onPrev={() => setStep(prevStep(step))}
          onSubmit={(data) => {
            patchDraft(data);
            setStep(nextStep(step));
          }}
        />
      ) : null}

      {step === "scale" ? (
        <ScaleForm
          defaultValues={{
            expected_monthly_calls: draft.expected_monthly_calls ?? 0,
            expected_use_case: draft.expected_use_case ?? "",
          }}
          onPrev={() => setStep(prevStep(step))}
          onSubmit={(data) => {
            patchDraft(data);
            setStep(nextStep(step));
          }}
        />
      ) : null}

      {step === "kyc" ? (
        <KycStepView
          value={kycState}
          isEnterprise={(draft.type ?? "enterprise") === "enterprise"}
          onChange={(v) => setKycState(v)}
          onPrev={() => setStep(prevStep(step))}
          onNext={() => setStep(nextStep(step))}
        />
      ) : null}

      {step === "review" ? (
        <ReviewStep
          draft={draft}
          kyc={kycState}
          consent={consent}
          onConsent={(c) => setConsent(c)}
          consentVersion={CONSENT_VERSION}
          onPrev={() => setStep(prevStep(step))}
          submitting={submit.state.submitting}
          onSubmit={async () => {
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
                company_name: draft.company_name,
                unified_social_credit_code: draft.unified_social_credit_code,
                expected_monthly_calls: draft.expected_monthly_calls,
                expected_use_case: draft.expected_use_case,
                source_channel: draft.source_channel,
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
          }}
        />
      ) : null}
    </section>
  );
}

// ===================== 子组件：每步一个 ===================== //

function ContactForm(props: {
  defaultValues: ContactStep;
  onSubmit: (data: ContactStep) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { register, handleSubmit, formState } = useForm<ContactStep>({
    defaultValues: props.defaultValues,
    resolver: zodResolver(ContactStepSchema),
  });
  return (
    <form onSubmit={handleSubmit(props.onSubmit)} noValidate>
      <div style={{ marginBottom: 12 }}>
        <label htmlFor="apply-type" style={{ display: "block", color: "#cbd5e1", marginBottom: 4 }}>
          {t("apply.field.type")} <span style={{ color: "#f87171" }}>*</span>
        </label>
        <select
          id="apply-type"
          {...register("type")}
          style={{
            width: "100%",
            padding: "8px 10px",
            background: "#0f1722",
            border: "1px solid #1f2937",
            borderRadius: 4,
            color: "#e5e7eb",
          }}
        >
          <option value="enterprise">{t("apply.field.type.enterprise")}</option>
          <option value="individual">{t("apply.field.type.individual")}</option>
        </select>
      </div>

      <Field
        id="apply-name"
        label={t("apply.field.contact_name")}
        required
        error={formState.errors.contact_name}
        registration={register("contact_name")}
        autoComplete="name"
      />
      <Field
        id="apply-phone"
        label={t("apply.field.contact_phone")}
        required
        type="tel"
        inputMode="tel"
        autoComplete="tel"
        error={formState.errors.contact_phone}
        registration={register("contact_phone")}
      />
      <Field
        id="apply-email"
        label={t("apply.field.contact_email")}
        required
        type="email"
        autoComplete="email"
        error={formState.errors.contact_email}
        registration={register("contact_email")}
      />
      <Field
        id="apply-source"
        label={t("apply.field.source")}
        registration={register("source_channel")}
      />

      <FormButtons next />
    </form>
  );
}

function CompanyForm(props: {
  defaultValues: CompanyStep;
  onPrev: () => void;
  onSubmit: (data: CompanyStep) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { register, handleSubmit, formState, watch } = useForm<CompanyStep>({
    defaultValues: props.defaultValues,
    resolver: zodResolver(CompanyStepSchema),
  });
  const isEnterprise = watch("type") === "enterprise";
  return (
    <form onSubmit={handleSubmit(props.onSubmit)} noValidate>
      <input type="hidden" {...register("type")} />
      {isEnterprise ? (
        <>
          <Field
            id="apply-company"
            label={t("apply.field.company_name")}
            required
            error={formState.errors.company_name}
            registration={register("company_name")}
          />
          <Field
            id="apply-uscc"
            label={t("apply.field.uscc")}
            required
            error={formState.errors.unified_social_credit_code}
            registration={register("unified_social_credit_code")}
          />
        </>
      ) : null}
      <Field
        id="apply-legal-id"
        label="法人 / 申请人身份证号"
        required
        error={formState.errors.legal_person_id}
        registration={register("legal_person_id")}
        hint="仅用于 KYC 资质审核；存储时进行信封加密"
      />
      <FormButtons next prev onPrev={props.onPrev} />
    </form>
  );
}

function ScaleForm(props: {
  defaultValues: ScaleStep;
  onPrev: () => void;
  onSubmit: (data: ScaleStep) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { register, handleSubmit, formState } = useForm<ScaleStep>({
    defaultValues: props.defaultValues,
    resolver: zodResolver(ScaleStepSchema),
  });
  return (
    <form onSubmit={handleSubmit(props.onSubmit)} noValidate>
      <Field
        id="apply-calls"
        label={t("apply.field.expected_calls")}
        required
        type="number"
        inputMode="numeric"
        error={formState.errors.expected_monthly_calls}
        registration={register("expected_monthly_calls", { valueAsNumber: true })}
      />
      <Field
        id="apply-usecase"
        label={t("apply.field.use_case")}
        required
        multiline
        error={formState.errors.expected_use_case}
        registration={register("expected_use_case")}
      />
      <FormButtons next prev onPrev={props.onPrev} />
    </form>
  );
}

function KycStepView(props: {
  value: KycStep;
  isEnterprise: boolean;
  onChange: (v: KycStep) => void;
  onPrev: () => void;
  onNext: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const update = (patch: Partial<KycStep>): void => {
    props.onChange({ ...props.value, ...patch });
  };
  const ready =
    props.value.id_front_url &&
    props.value.id_back_url &&
    props.value.legal_person_face_url &&
    (!props.isEnterprise || props.value.business_license_url);
  return (
    <div>
      <KycUploader
        kind="id_front"
        label={t("apply.kyc.id_front")}
        required
        value={props.value.id_front_url}
        onChange={(url) => update({ id_front_url: url })}
      />
      <KycUploader
        kind="id_back"
        label={t("apply.kyc.id_back")}
        required
        value={props.value.id_back_url}
        onChange={(url) => update({ id_back_url: url })}
      />
      {props.isEnterprise ? (
        <KycUploader
          kind="business_license"
          label={t("apply.kyc.business_license")}
          required
          maxBytes={10 * 1024 * 1024}
          value={props.value.business_license_url ?? ""}
          onChange={(url) => update({ business_license_url: url })}
        />
      ) : null}
      <KycUploader
        kind="legal_person_face"
        label={t("apply.kyc.legal_person_face")}
        required
        maxBytes={20 * 1024 * 1024}
        accept="video/mp4,image/jpeg,image/png"
        value={props.value.legal_person_face_url}
        onChange={(url) => update({ legal_person_face_url: url })}
      />
      <FormButtons next prev onPrev={props.onPrev} disabled={!ready} onNext={props.onNext} />
    </div>
  );
}

function ReviewStep(props: {
  draft: Partial<ContactStep & CompanyStep & ScaleStep>;
  kyc: KycStep;
  consent: { consent_id: number; version: string } | null;
  onConsent: (c: { consent_id: number; version: string }) => void;
  consentVersion: string;
  onPrev: () => void;
  onSubmit: () => void | Promise<void>;
  submitting: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <div>
      <Summary draft={props.draft} kyc={props.kyc} />
      <ConsentBox
        version={props.consentVersion}
        accepted={Boolean(props.consent)}
        onAccepted={props.onConsent}
      />
      <FormButtons
        prev
        onPrev={props.onPrev}
        submit
        submitLabel={props.submitting ? t("apply.submitting") : t("apply.submit")}
        onSubmit={props.onSubmit}
        disabled={!props.consent || props.submitting}
      />
    </div>
  );
}

function Summary(props: {
  draft: Partial<ContactStep & CompanyStep & ScaleStep>;
  kyc: KycStep;
}): JSX.Element {
  const { t } = useTranslation();
  // 只展示非敏感字段；身份证 / 上传 URL 用 "已上传" 表示
  const rows: Array<[string, string]> = [
    [t("apply.field.type"), props.draft.type ?? "—"],
    [t("apply.field.contact_name"), props.draft.contact_name ?? "—"],
    [t("apply.field.contact_email"), props.draft.contact_email ?? "—"],
    [t("apply.field.expected_calls"), String(props.draft.expected_monthly_calls ?? "—")],
  ];
  if (props.draft.type === "enterprise") {
    rows.push([t("apply.field.company_name"), props.draft.company_name ?? "—"]);
  }
  return (
    <dl style={{ background: "#11151c", border: "1px solid #1f2937", padding: 16, borderRadius: 6 }}>
      {rows.map(([k, v]) => (
        <div key={k} style={{ display: "flex", gap: 12, marginBottom: 6 }}>
          <dt style={{ color: "#9ca3af", width: 120 }}>{k}</dt>
          <dd style={{ color: "#e5e7eb", margin: 0 }}>{v}</dd>
        </div>
      ))}
      <div style={{ display: "flex", gap: 12 }}>
        <dt style={{ color: "#9ca3af", width: 120 }}>KYC</dt>
        <dd style={{ color: "#22c55e", margin: 0 }}>
          {Object.values(props.kyc).filter(Boolean).length} 项已上传
        </dd>
      </div>
    </dl>
  );
}

function FormButtons(props: {
  next?: boolean;
  prev?: boolean;
  submit?: boolean;
  submitLabel?: string;
  disabled?: boolean;
  onPrev?: () => void;
  onNext?: () => void;
  onSubmit?: () => void | Promise<void>;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <div style={{ display: "flex", gap: 8, marginTop: 16 }}>
      {props.prev ? (
        <button
          type="button"
          onClick={props.onPrev}
          style={{ padding: "8px 16px", background: "transparent", color: "#cbd5e1", border: "1px solid #2a2f36", borderRadius: 4, cursor: "pointer" }}
        >
          {t("apply.prev")}
        </button>
      ) : null}
      {props.next ? (
        <button
          type={props.onNext ? "button" : "submit"}
          onClick={props.onNext}
          disabled={props.disabled}
          style={{ padding: "8px 16px", background: "#2563eb", color: "#fff", border: 0, borderRadius: 4, cursor: props.disabled ? "not-allowed" : "pointer", opacity: props.disabled ? 0.5 : 1 }}
        >
          {t("apply.next")}
        </button>
      ) : null}
      {props.submit ? (
        <button
          type="button"
          onClick={() => props.onSubmit?.()}
          disabled={props.disabled}
          style={{ padding: "8px 16px", background: "#16a34a", color: "#fff", border: 0, borderRadius: 4, cursor: props.disabled ? "not-allowed" : "pointer", opacity: props.disabled ? 0.5 : 1 }}
        >
          {props.submitLabel ?? t("apply.submit")}
        </button>
      ) : null}
    </div>
  );
}

function SubmittedScreen({ id, onClear }: { id: number; onClear: () => void }): JSX.Element {
  const { t } = useTranslation();
  return (
    <section style={{ maxWidth: 600, margin: "64px auto", textAlign: "center" }}>
      <h1 style={{ color: "#22c55e" }}>{t("apply.success.title")}</h1>
      <p style={{ color: "#9ca3af" }}>{t("apply.success.body", { id })}</p>
      <button
        type="button"
        onClick={onClear}
        style={{ padding: "8px 16px", background: "#2563eb", color: "#fff", border: 0, borderRadius: 4, cursor: "pointer" }}
      >
        {t("common.back_home")}
      </button>
    </section>
  );
}
