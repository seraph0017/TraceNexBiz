// Step 1：联系人信息
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { zodResolver } from "@/lib/zodResolver";
import { Field } from "@/components/apply/Field";
import { ContactStepSchema, type ContactStep } from "@/schemas/applyPartner";
import { FormButtons } from "./FormButtons";

export function Step1Basics(props: {
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
