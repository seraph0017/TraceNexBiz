// 申请草稿持久化 store —— 中断恢复
// 使用 Zustand persist + localStorage；immutable 通过 spread + immer-style 函数式 update
import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";
import type { ApplyDraft } from "@/schemas/applyPartner";

export type ApplyStep = "contact" | "company" | "scale" | "bank" | "kyc" | "review" | "done";

export interface ApplyDraftState {
  step: ApplyStep;
  draft: Partial<ApplyDraft>;
  /** 提交成功后服务端返回的 partner application id */
  submittedId: number | null;
  setStep(step: ApplyStep): void;
  patchDraft(patch: Partial<ApplyDraft>): void;
  clearDraft(): void;
  markSubmitted(id: number): void;
}

const STORAGE_KEY = "tnbiz.storefront.apply-draft.v1";

export const useApplyDraft = create<ApplyDraftState>()(
  persist(
    (set) => ({
      step: "contact",
      draft: {},
      submittedId: null,
      setStep: (step) => set((s) => ({ ...s, step })),
      patchDraft: (patch) =>
        set((s) => {
          // 不直接 mutate s.draft；返回新对象
          const next: Partial<ApplyDraft> = { ...s.draft, ...patch };
          return { ...s, draft: next };
        }),
      clearDraft: () =>
        set(() => ({ step: "contact", draft: {}, submittedId: null })),
      markSubmitted: (id) =>
        set((s) => ({ ...s, submittedId: id, step: "done" })),
    }),
    {
      name: STORAGE_KEY,
      storage: createJSONStorage(() => localStorage),
      // 仅持久化非敏感字段；身份证 / 上传 URL / 银行账号 不写入 localStorage
      partialize: (s) => ({
        step: s.step,
        submittedId: s.submittedId,
        draft: {
          type: s.draft.type,
          contact_name: s.draft.contact_name,
          contact_phone: s.draft.contact_phone,
          contact_email: s.draft.contact_email,
          source_channel: s.draft.source_channel,
          company_name: s.draft.company_name,
          unified_social_credit_code: s.draft.unified_social_credit_code,
          expected_monthly_calls: s.draft.expected_monthly_calls,
          expected_use_case: s.draft.expected_use_case,
          tax_status: s.draft.tax_status,
          settlement_bank_name: s.draft.settlement_bank_name,
          settlement_account_holder: s.draft.settlement_account_holder,
          // settlement_bank_account / legal_person_id 故意不持久化（PII）
        },
      }),
      version: 1,
    },
  ),
);
