import { describe, it, expect } from "vitest";
import { useAllocateStore } from "./allocateStore";

describe("allocate store", () => {
  it("transitions phases without mutating", () => {
    useAllocateStore.getState().reset();
    const before = useAllocateStore.getState();
    useAllocateStore.getState().setPhase("running", "saga-1");
    const after = useAllocateStore.getState();
    expect(after.phase).toBe("running");
    expect(after.sagaId).toBe("saga-1");
    // 同一对象引用应被替换（zustand 通过 set 创建新 state）
    expect(before).not.toBe(after);
    useAllocateStore.getState().reset();
    expect(useAllocateStore.getState().phase).toBe("idle");
    expect(useAllocateStore.getState().sagaId).toBeNull();
  });
});
