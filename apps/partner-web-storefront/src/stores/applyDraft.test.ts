import { describe, it, expect, beforeEach } from "vitest";
import { useApplyDraft } from "@/stores/applyDraft";

describe("applyDraft store", () => {
  beforeEach(() => {
    useApplyDraft.getState().clearDraft();
    localStorage.clear();
  });

  it("patchDraft 不直接 mutate 旧引用（immutability）", () => {
    const before = useApplyDraft.getState().draft;
    useApplyDraft.getState().patchDraft({ contact_name: "Alice" });
    const after = useApplyDraft.getState().draft;
    expect(after).not.toBe(before);
    expect(after.contact_name).toBe("Alice");
  });

  it("clearDraft 重置 step + draft + submittedId", () => {
    useApplyDraft.getState().patchDraft({ contact_name: "X" });
    useApplyDraft.getState().setStep("review");
    useApplyDraft.getState().markSubmitted(42);
    expect(useApplyDraft.getState().submittedId).toBe(42);
    expect(useApplyDraft.getState().step).toBe("done");
    useApplyDraft.getState().clearDraft();
    expect(useApplyDraft.getState().draft).toEqual({});
    expect(useApplyDraft.getState().step).toBe("contact");
    expect(useApplyDraft.getState().submittedId).toBeNull();
  });
});
