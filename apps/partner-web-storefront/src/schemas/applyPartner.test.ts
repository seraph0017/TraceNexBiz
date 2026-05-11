import { describe, it, expect } from "vitest";
import {
  ContactStepSchema,
  CompanyStepSchema,
  ScaleStepSchema,
  KycStepSchema,
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
});
