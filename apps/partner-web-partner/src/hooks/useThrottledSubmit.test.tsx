// useThrottledSubmit tests for partner app
import { describe, it, expect, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useThrottledSubmit } from "@/hooks/useThrottledSubmit";

describe("useThrottledSubmit (partner)", () => {
  it("第一次提交执行；锁定期间第二次返 null", async () => {
    let count = 0;
    const fn = vi.fn(async () => {
      await new Promise((r) => setTimeout(r, 30));
      count += 1;
      return count;
    });
    const { result } = renderHook(() => useThrottledSubmit(fn, { coolDownMs: 10 }));
    let r1: number | null = null;
    let r2: number | null = null;
    await act(async () => {
      const p1 = result.current.submit();
      const p2 = result.current.submit();
      [r1, r2] = await Promise.all([p1, p2]);
    });
    expect(r1).toBe(1);
    expect(r2).toBe(null);
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it("捕获错误且状态可重置", async () => {
    const fn = vi.fn(async () => {
      throw new Error("boom");
    });
    const { result } = renderHook(() => useThrottledSubmit(fn, { coolDownMs: 1 }));
    await act(async () => {
      await expect(result.current.submit()).rejects.toThrow("boom");
    });
    expect(result.current.state.error?.message).toBe("boom");
    act(() => result.current.reset());
    expect(result.current.state.error).toBeNull();
  });
});
