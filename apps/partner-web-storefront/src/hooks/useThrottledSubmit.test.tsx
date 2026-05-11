import { describe, it, expect, vi } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import { useThrottledSubmit } from "@/hooks/useThrottledSubmit";

describe("useThrottledSubmit", () => {
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
    expect(r2).toBeNull();
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it("失败时设置 error 状态", async () => {
    const fn = vi.fn(async () => {
      throw new Error("boom");
    });
    const { result } = renderHook(() => useThrottledSubmit(fn));
    await act(async () => {
      await expect(result.current.submit()).rejects.toThrow("boom");
    });
    await waitFor(() => {
      expect(result.current.state.error?.message).toBe("boom");
    });
  });

  it("reset 清空状态", async () => {
    const fn = vi.fn(async () => 1);
    const { result } = renderHook(() => useThrottledSubmit(fn));
    await act(async () => {
      await result.current.submit();
    });
    expect(result.current.state.lastResult).toBe(1);
    act(() => result.current.reset());
    expect(result.current.state.lastResult).toBeNull();
  });
});
