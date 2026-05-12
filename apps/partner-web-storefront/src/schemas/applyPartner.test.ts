import { describe, it, expect } from "vitest";
import {
  ContactStepSchema,
  CompanyStepSchema,
  ScaleStepSchema,
  KycStepSchema,
  BankStepSchema,
  TaxStatusSchema,
  ConsentStepSchema,
  maskIdCard,
  maskPhone,
} from "@/schemas/applyPartner";

describe("applyPartner schemas", () => {
  describe("ContactStepSchema", () => {
    it("接受合法手机号 / 邮箱", () => {
      const r = ContactStepSchema.safeParse({
        type: "enterprise",
        contact_name: "张三",
        contact_phone: "13800138000",
        contact_email: "user@example.com",
        source_channel: "",
      });
      expect(r.success).toBe(true);
    });

    it("拒绝非法手机号", () => {
      const r = ContactStepSchema.safeParse({
        type: "individual",
        contact_name: "张三",
        contact_phone: "12345",
        contact_email: "user@example.com",
      });
      expect(r.success).toBe(false);
    });

    it("拒绝非法邮箱", () => {
      const r = ContactStepSchema.safeParse({
        type: "individual",
        contact_name: "张三",
        contact_phone: "13800138000",
        contact_email: "not-an-email",
      });
      expect(r.success).toBe(false);
    });
  });

  describe("CompanyStepSchema", () => {
    it("企业必须填 USCC + company_name", () => {
      const r = CompanyStepSchema.safeParse({
        type: "enterprise",
        company_name: "TraceNex 科技",
        unified_social_credit_code: "91110108MA00ABCD12",
        legal_person_id: "11010119900101001X",
      });
      expect(r.success).toBe(true);
    });

    it("企业空 company_name 报错", () => {
      const r = CompanyStepSchema.safeParse({
        type: "enterprise",
        legal_person_id: "11010119900101001X",
      });
      expect(r.success).toBe(false);
    });

    it("个人不强制 company_name 但要身份证", () => {
      const r = CompanyStepSchema.safeParse({
        type: "individual",
        legal_person_id: "11010119900101001X",
      });
      expect(r.success).toBe(true);
    });

    it("非法身份证报错", () => {
      const r = CompanyStepSchema.safeParse({
        type: "individual",
        legal_person_id: "abc",
      });
      expect(r.success).toBe(false);
    });
  });

  describe("ScaleStepSchema", () => {
    it("接受字符串数字", () => {
      const r = ScaleStepSchema.safeParse({
        expected_monthly_calls: "100000",
        expected_use_case: "我们希望接入 GPT-4 用于客服自动化处理",
      });
      expect(r.success).toBe(true);
      if (r.success) expect(r.data.expected_monthly_calls).toBe(100000);
    });

    it("拒绝 0", () => {
      const r = ScaleStepSchema.safeParse({
        expected_monthly_calls: 0,
        expected_use_case: "我们希望接入 GPT-4 用于客服自动化处理",
      });
      expect(r.success).toBe(false);
    });

    it("拒绝过短 use_case", () => {
      const r = ScaleStepSchema.safeParse({
        expected_monthly_calls: 100,
        expected_use_case: "短",
      });
      expect(r.success).toBe(false);
    });
  });

  describe("KycStepSchema", () => {
    it("个人可以不填执照", () => {
      const r = KycStepSchema.safeParse({
        id_front_url: "https://oss.example.com/a.jpg",
        id_back_url: "https://oss.example.com/b.jpg",
        business_license_url: "",
        legal_person_face_url: "https://oss.example.com/c.mp4",
      });
      expect(r.success).toBe(true);
    });

    it("缺人脸链接报错", () => {
      const r = KycStepSchema.safeParse({
        id_front_url: "https://oss.example.com/a.jpg",
        id_back_url: "https://oss.example.com/b.jpg",
        legal_person_face_url: "",
      });
      expect(r.success).toBe(false);
    });
  });

  describe("PII mask", () => {
    it("身份证脱敏只保留首 4 + 末 4", () => {
      expect(maskIdCard("11010119900101001X")).toBe("1101**********001X");
    });
    it("手机号脱敏只保留首 3 + 末 4", () => {
      expect(maskPhone("13800138000")).toBe("138****8000");
    });
    it("过短直接 ***", () => {
      expect(maskIdCard("123")).toBe("***");
      expect(maskPhone("123")).toBe("***");
    });
  });

  describe("BankStepSchema (Fix-C item 5/6)", () => {
    it("接受合法 12-19 位银行账号 + tax_status", () => {
      const r = BankStepSchema.safeParse({
        tax_status: "individual",
        settlement_bank_name: "中国工商银行 北京海淀支行",
        settlement_bank_account: "6222021234567890123",
        settlement_account_holder: "张三",
      });
      expect(r.success).toBe(true);
    });

    it("拒绝过短银行账号", () => {
      const r = BankStepSchema.safeParse({
        tax_status: "individual",
        settlement_bank_name: "中国工商银行",
        settlement_bank_account: "123",
        settlement_account_holder: "张三",
      });
      expect(r.success).toBe(false);
    });

    it("tax_status 必须是 5 值枚举", () => {
      const r = BankStepSchema.safeParse({
        tax_status: "freelancer",
        settlement_bank_name: "中国工商银行",
        settlement_bank_account: "6222021234567890123",
        settlement_account_holder: "张三",
      });
      expect(r.success).toBe(false);
    });

    it("接受所有 5 个 tax_status 值", () => {
      const okValues = ["individual", "sole_proprietor", "partnership", "llc", "corp"] as const;
      for (const v of okValues) {
        expect(TaxStatusSchema.safeParse(v).success).toBe(true);
      }
      expect(TaxStatusSchema.safeParse("xyz").success).toBe(false);
    });
  });

  describe("ConsentStepSchema (Fix-C item 7)", () => {
    it("接受完整 consent + consent_text_version", () => {
      const r = ConsentStepSchema.safeParse({
        consent_id: 1,
        consent_version: "2026-05-pipl-v1",
        consent_text_version: "2026-05-pipl-v1",
        granted: true,
      });
      expect(r.success).toBe(true);
    });

    it("granted 必须 true", () => {
      const r = ConsentStepSchema.safeParse({
        consent_id: 1,
        consent_version: "v1",
        consent_text_version: "v1",
        granted: false,
      });
      expect(r.success).toBe(false);
    });

    it("缺 consent_text_version 报错", () => {
      const r = ConsentStepSchema.safeParse({
        consent_id: 1,
        consent_version: "v1",
        granted: true,
      });
      expect(r.success).toBe(false);
    });
  });
});
