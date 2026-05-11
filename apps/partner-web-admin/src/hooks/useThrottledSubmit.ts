// 节流提交 hook —— 与 storefront 同
import { useCallback, useRef, useState } from "react";

export interface ThrottledSubmitState<T> {
  submitting: boolean;
  error: Error | null;
  lastResult: T | null;
}

export function useThrottledSubmit<TArgs extends unknown[], TResult>(
  fn: (...args: TArgs) => Promise<TResult>,
  options: { coolDownMs?: number } = {},
): {
  submit: (...args: TArgs) => Promise<TResult | null>;
  state: ThrottledSubmitState<TResult>;
  reset: () => void;
} {
  const coolDownMs = options.coolDownMs ?? 1500;
  const lockRef = useRef(false);
  const lastFiredAtRef = useRef(0);
  const [state, setState] = useState<ThrottledSubmitState<TResult>>({
    submitting: false,
    error: null,
    lastResult: null,
  });

  const submit = useCallback(
    async (...args: TArgs): Promise<TResult | null> => {
      const now = Date.now();
      if (lockRef.current) return null;
      if (now - lastFiredAtRef.current < coolDownMs) return null;
      lockRef.current = true;
      lastFiredAtRef.current = now;
      setState((s) => ({ ...s, submitting: true, error: null }));
      try {
        const result = await fn(...args);
        setState({ submitting: false, error: null, lastResult: result });
        return result;
      } catch (err: unknown) {
        const error = err instanceof Error ? err : new Error(String(err));
        setState({ submitting: false, error, lastResult: null });
        throw error;
      } finally {
        lockRef.current = false;
      }
    },
    [fn, coolDownMs],
  );

  const reset = useCallback(() => {
    setState({ submitting: false, error: null, lastResult: null });
    lockRef.current = false;
    lastFiredAtRef.current = 0;
  }, []);

  return { submit, state, reset };
}
