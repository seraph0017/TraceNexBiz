// admin auth store
import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { StaffMe } from "@/api/admin";

interface AuthState {
  me: StaffMe | null;
  setMe: (me: StaffMe) => void;
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
      name: "tnbiz.admin.auth",
      partialize: (state) => ({ me: state.me }),
    },
  ),
);
