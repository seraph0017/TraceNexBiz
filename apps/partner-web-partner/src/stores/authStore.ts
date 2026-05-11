// auth store —— 仅持久化 me 的非敏感字段；不存 token（token 在 httpOnly cookie）
import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { PartnerMe } from "@/api/partner";

interface AuthState {
  me: PartnerMe | null;
  setMe: (me: PartnerMe) => void;
  clear: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      me: null,
      setMe: (me) => set({ me }),
      clear: () => set({ me: null }),
    }),
    {
      name: "tnbiz.partner.auth",
      // 仅持久化 me 的安全字段（已是 *_masked，不含明文 PII）；不持久化 token
      partialize: (state) => ({ me: state.me }),
    },
  ),
);
