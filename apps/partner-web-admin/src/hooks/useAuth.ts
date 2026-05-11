// admin useAuth
import { useCallback, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import * as api from "@/api/admin";
import { useAuthStore } from "@/stores/authStore";

export function useAuth(): {
  me: api.StaffMe | undefined;
  loading: boolean;
  login: (input: api.LoginInput) => Promise<api.LoginResp>;
  logout: (scope?: "current" | "all") => Promise<void>;
} {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const setMe = useAuthStore((s) => s.setMe);
  const clear = useAuthStore((s) => s.clear);

  const { data: me, isLoading } = useQuery({
    queryKey: ["admin", "me"],
    queryFn: () => api.getStaffMe(),
    staleTime: 60_000,
    retry: 0,
  });

  useEffect(() => {
    if (me) setMe(me);
  }, [me, setMe]);

  const login = useCallback(
    async (input: api.LoginInput) => {
      const res = await api.login(input);
      await qc.invalidateQueries({ queryKey: ["admin", "me"] });
      return res;
    },
    [qc],
  );

  const logout = useCallback(
    async (scope: "current" | "all" = "current") => {
      try {
        await api.logout(scope);
      } finally {
        clear();
        qc.clear();
        navigate("/auth/login", { replace: true });
      }
    },
    [clear, navigate, qc],
  );

  useEffect(() => {
    const handler = (): void => {
      clear();
      navigate("/auth/login", { replace: true });
    };
    window.addEventListener("tnbiz:auth:expired", handler);
    return () => window.removeEventListener("tnbiz:auth:expired", handler);
  }, [clear, navigate]);

  return { me, loading: isLoading, login, logout };
}
