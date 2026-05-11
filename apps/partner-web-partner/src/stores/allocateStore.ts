// allocate saga local state store —— 跟踪正在运行的 saga（中断后恢复）
// 不持久化 idempotency-key（一旦页面关闭 saga 由后台继续推进；server canonical）
import { create } from "zustand";

export type SagaPhase =
  | "idle"
  | "submitting"
  | "running"
  | "succeeded"
  | "failed_user"
  | "failed_system"
  | "pending_unknown"
  | "escalated";

interface AllocateState {
  phase: SagaPhase;
  sagaId: string | null;
  errorCode: string | null;
  setPhase: (phase: SagaPhase, sagaId?: string | null, errorCode?: string | null) => void;
  reset: () => void;
}

export const useAllocateStore = create<AllocateState>((set) => ({
  phase: "idle",
  sagaId: null,
  errorCode: null,
  setPhase: (phase, sagaId = null, errorCode = null) => set({ phase, sagaId, errorCode }),
  reset: () => set({ phase: "idle", sagaId: null, errorCode: null }),
}));
