import { describe, it, expect } from "vitest";
import { fenToYuan, maskBankAccount, maskEmail, maskIdCard, maskPhone } from "./pii";

describe("pii mask helpers", () => {
  it("masks phone middle 4 digits", () => {
    expect(maskPhone("13812345678")).toBe("138****5678");
  });
  it("masks short phone safely", () => {
    expect(maskPhone("123")).toBe("****");
    expect(maskPhone("")).toBe("");
  });
  it("masks email", () => {
    expect(maskEmail("alice@example.com")).toBe("a***e@example.com");
    expect(maskEmail("a@x.com")).toBe("*@x.com");
  });
  it("masks id card", () => {
    expect(maskIdCard("110101199001011234")).toBe("1101**********1234");
  });
  it("masks bank account", () => {
    expect(maskBankAccount("6225 7634 1234 5678")).toBe("**** **** **** 5678");
  });
  it("converts fen to yuan with thousands separator", () => {
    expect(fenToYuan(0)).toBe("0.00");
    expect(fenToYuan(123456)).toBe("1,234.56");
  });
});
