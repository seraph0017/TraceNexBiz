// Step 2：公司信息 + 法人 ID + 业务规模合并展示（PRD §15.4 个人 / 企业分支）
// 拆分前 Company + Scale 两步合并为一个 KYC 前置；保留 react-hook-form 体验
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { zodResolver } from "@/lib/zodResolver";
import { Field } from "@/components/apply/Field";
import {
  CompanyStepSchema,
  type CompanyStep,
} from "@/schemas/applyPartner";
import { FormButtons } from "./FormButtons";

export function Step2KYC(props: {
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
            hint="business_license_number — 18 位统一社会信用代码"
          />
        </>
      ) : null}
      <Field
        id="apply-legal-id"
        label="法人 / 申请人身份证号（legal_representative_id）"
        required
        error={formState.errors.legal_person_id}
        registration={register("legal_person_id")}
        hint="仅用于 KYC 资质审核；存储时进行 KMS 信封加密"
      />
      <FormButtons next prev onPrev={props.onPrev} />
    </form>
  );
}
