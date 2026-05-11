import { describe, it, expect } from "vitest";
import { z } from "zod";
import { zodResolver } from "./zodResolver";

const Schema = z.object({
  amount: z.number().int().min(1),
  note: z.string().max(8).optional(),
});

describe("zodResolver", () => {
  it("returns parsed values on success", async () => {
    const r = await zodResolver(Schema)({ amount: 100, note: "hi" }, undefined, {
      fields: {},
      criteriaMode: "firstError",
      shouldUseNativeValidation: false,
    } as never);
    expect(r.values).toEqual({ amount: 100, note: "hi" });
    expect(r.errors).toEqual({});
  });
  it("returns errors on failure", async () => {
    const r = await zodResolver(Schema)({ amount: 0 }, undefined, {
      fields: {},
      criteriaMode: "firstError",
      shouldUseNativeValidation: false,
    } as never);
    expect(r.errors).toHaveProperty("amount");
  });
});
