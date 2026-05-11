import { describe, it, expect } from "vitest";
import { zodResolver } from "@/lib/zodResolver";
import { z } from "zod";

describe("zodResolver", () => {
  it("成功时返 values 无 errors", async () => {
    const schema = z.object({ a: z.string() });
    const r = zodResolver(schema);
    const out = await r({ a: "x" }, undefined, { fields: {}, shouldUseNativeValidation: false });
    expect(out.values).toEqual({ a: "x" });
    expect(Object.keys(out.errors)).toHaveLength(0);
  });

  it("失败时返 errors 按字段聚合", async () => {
    const schema = z.object({ a: z.string().min(3, "too_short"), b: z.number() });
    const r = zodResolver(schema);
    const out = await r({ a: "x", b: "y" }, undefined, {
      fields: {},
      shouldUseNativeValidation: false,
    });
    expect((out.errors as Record<string, { message: string }>).a?.message).toBe("too_short");
    expect((out.errors as Record<string, { message: string }>).b?.message).toBeTruthy();
  });
});
