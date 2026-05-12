// Step 4：KYC 上传 - 身份证 / 营业执照 / 法人面照
import { useTranslation } from "react-i18next";
import { KycUploader } from "@/components/KycUploader";
import type { KycStep } from "@/schemas/applyPartner";
import { FormButtons } from "./FormButtons";

export function Step4KYC(props: {
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
