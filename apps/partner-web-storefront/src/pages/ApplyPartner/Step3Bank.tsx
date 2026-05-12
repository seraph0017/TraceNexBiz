// Step 3：业务规模 + 结算银行账户 + tax_status
// PRD §15.4 个人渠道商代扣代缴 / Fix-C item 5 (tax_status enum) / item 6 (bank_account HMAC blind_index)
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { zodResolver } from "@/lib/zodResolver";
import { z } from "zod";
import { Field } from "@/components/apply/Field";
import {
  ScaleStepSchema,
  BankStepSchema,
  type TaxStatus,
} from "@/schemas/applyPartner";
import { FormButtons } from "./FormButtons";

const Step3Schema = ScaleStepSchema.merge(BankStepSchema);
type Step3Form = z.infer<typeof Step3Schema>;

const TAX_STATUS_OPTIONS: Array<{ value: TaxStatus; label: string; hint?: string }> = [
  { value: "individual", label: "个人 (劳务报酬代扣代缴)", hint: "PRD §15.4 默认；平台代扣劳务税" },
  { value: "sole_proprietor", label: "个体工商户" },
  { value: "partnership", label: "合伙企业" },
  { value: "llc", label: "有限责任公司" },
  { value: "corp", label: "股份有限公司" },
];

export function Step3Bank(props: {
  defaultValues: Step3Form;
  onPrev: () => void;
  onSubmit: (data: Step3Form) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { register, handleSubmit, formState } = useForm<Step3Form>({
    defaultValues: props.defaultValues,
    resolver: zodResolver(Step3Schema),
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

      <div style={{ marginTop: 24, marginBottom: 12 }}>
        <h3 style={{ color: "#e5e7eb", fontSize: 16, margin: "0 0 4px" }}>结算账户</h3>
        <p style={{ color: "#9ca3af", fontSize: 13, margin: 0 }}>
          用于平台向您发放分润；银行账号 KMS 加密 + HMAC blind_index 落库 (Fix-C item 6)
        </p>
      </div>

      <div style={{ marginBottom: 12 }}>
        <label htmlFor="apply-tax-status" style={{ display: "block", color: "#cbd5e1", marginBottom: 4 }}>
          税务身份 (tax_status) <span style={{ color: "#f87171" }}>*</span>
        </label>
        <select
          id="apply-tax-status"
          {...register("tax_status")}
          style={{
            width: "100%",
            padding: "8px 10px",
            background: "#0f1722",
            border: "1px solid #1f2937",
            borderRadius: 4,
            color: "#e5e7eb",
          }}
        >
          {TAX_STATUS_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
        {formState.errors.tax_status ? (
          <span style={{ color: "#f87171", fontSize: 12 }}>{String(formState.errors.tax_status.message)}</span>
        ) : null}
      </div>

      <Field
        id="apply-bank-name"
        label="开户银行名称"
        required
        error={formState.errors.settlement_bank_name}
        registration={register("settlement_bank_name")}
        hint="例：中国工商银行 北京海淀支行"
      />
      <Field
        id="apply-bank-account"
        label="银行账号 (settlement_bank_account)"
        required
        inputMode="numeric"
        error={formState.errors.settlement_bank_account}
        registration={register("settlement_bank_account")}
        hint="12-19 位数字；落库前 HMAC 计算 blind_index"
      />
      <Field
        id="apply-bank-holder"
        label="账户持有人姓名"
        required
        error={formState.errors.settlement_account_holder}
        registration={register("settlement_account_holder")}
        hint="必须与法人 / 申请人姓名一致"
      />

      <FormButtons next prev onPrev={props.onPrev} />
    </form>
  );
}

export type { Step3Form };
