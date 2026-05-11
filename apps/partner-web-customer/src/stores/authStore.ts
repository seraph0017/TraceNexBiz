// auth store —— 仅持久化 me 的非敏感字段；token 在 httpOnly cookie
import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { CustomerMe } from "@/api/customer";

interface AuthState {
  me: CustomerMe | null;
  setMe: (me: CustomerMe) => void;
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
      name: "tnbiz.customer.auth",
      partialize: (state) => ({ me: state.me }),
    },
  ),
);
