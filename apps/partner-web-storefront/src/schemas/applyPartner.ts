// 招商申请 zod schemas —— 集中管理
// frontend §7.1 / PRD M1-05/06
import { z } from "zod";

// 中国大陆 11 位手机号；保留 +86 前缀（后端会规范化）
const PHONE_RE = /^(?:\+?86)?1[3-9]\d{9}$/;
// 18 位身份证（最后一位 X 大小写不敏感）
const ID_CARD_RE = /^[1-9]\d{5}(18|19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]$/;
// 18 位统一社会信用代码（粗校验：数字 + 大写字母）
const USCC_RE = /^[0-9A-HJ-NPQRTUWXY]{18}$/;

export const PartnerTypeSchema = z.enum(["individual", "enterprise"]);

export const ContactStepSchema = z.object({
  type: PartnerTypeSchema,
  contact_name: z.string().trim().min(2, "validation.too_short").max(40, "validation.too_long"),
  contact_phone: z.string().regex(PHONE_RE, "validation.phone"),
  contact_email: z.string().email("validation.email").max(120),
  source_channel: z.string().max(40).optional().or(z.literal("")),
});

export const CompanyStepSchema = z
  .object({
    type: PartnerTypeSchema,
    company_name: z.string().trim().max(120).optional().or(z.literal("")),
    unified_social_credit_code: z.string().optional().or(z.literal("")),
    legal_person_id: z.string().optional().or(z.literal("")),
  })
  .superRefine((data, ctx) => {
    if (data.type === "enterprise") {
      if (!data.company_name || data.company_name.length < 2) {
        ctx.addIssue({
          path: ["company_name"],
          code: z.ZodIssueCode.custom,
          message: "validation.required",
        });
      }
      if (!data.unified_social_credit_code || !USCC_RE.test(data.unified_social_credit_code)) {
        ctx.addIssue({
          path: ["unified_social_credit_code"],
          code: z.ZodIssueCode.custom,
          message: "validation.uscc",
        });
      }
    }
    // 个人 / 企业都需要法人 / 申请人身份证
    if (!data.legal_person_id || !ID_CARD_RE.test(data.legal_person_id)) {
      ctx.addIssue({
        path: ["legal_person_id"],
        code: z.ZodIssueCode.custom,
        message: "validation.idcard",
      });
    }
  });

export const ScaleStepSchema = z.object({
  expected_monthly_calls: z
    .union([z.number().int().positive(), z.string().regex(/^\d+$/)])
    .transform((v) => (typeof v === "string" ? Number(v) : v))
    .refine((n) => n > 0 && n <= 1_000_000_000, "validation.positive_int"),
  expected_use_case: z.string().trim().min(10, "validation.too_short").max(500, "validation.too_long"),
});

export const KycStepSchema = z.object({
  id_front_url: z.string().url(),
  id_back_url: z.string().url(),
  business_license_url: z.string().url().optional().or(z.literal("")),
  legal_person_face_url: z.string().url(),
});

export const ConsentStepSchema = z.object({
  consent_id: z.number().int().positive(),
  consent_version: z.string().min(1),
  granted: z.literal(true, {
    errorMap: () => ({ message: "apply.consent.required" }),
  }),
});

export const ApplyDraftSchema = ContactStepSchema.merge(
  z.object({
    company_name: z.string().optional().or(z.literal("")),
    unified_social_credit_code: z.string().optional().or(z.literal("")),
    legal_person_id: z.string().optional().or(z.literal("")),
    expected_monthly_calls: z.number().int().positive().optional(),
    expected_use_case: z.string().optional().or(z.literal("")),
    id_front_url: z.string().url().optional().or(z.literal("")),
    id_back_url: z.string().url().optional().or(z.literal("")),
    business_license_url: z.string().url().optional().or(z.literal("")),
    legal_person_face_url: z.string().url().optional().or(z.literal("")),
    consent_id: z.number().int().positive().optional(),
    consent_version: z.string().optional().or(z.literal("")),
    granted: z.boolean().optional(),
  }),
);

export type PartnerType = z.infer<typeof PartnerTypeSchema>;
export type ContactStep = z.infer<typeof ContactStepSchema>;
export type CompanyStep = z.infer<typeof CompanyStepSchema>;
export type ScaleStep = z.infer<typeof ScaleStepSchema>;
export type KycStep = z.infer<typeof KycStepSchema>;
export type ConsentStep = z.infer<typeof ConsentStepSchema>;
export type ApplyDraft = z.infer<typeof ApplyDraftSchema>;

/** PII 脱敏：仅用于 review 步骤 / 草稿恢复提示 */
export function maskIdCard(id: string): string {
  if (id.length < 8) return "***";
  return `${id.slice(0, 4)}**********${id.slice(-4)}`;
}

export function maskPhone(phone: string): string {
  const digits = phone.replace(/\D/g, "");
  if (digits.length < 7) return "***";
  return `${digits.slice(0, 3)}****${digits.slice(-4)}`;
}
