// useAuth —— 暴露登录/登出/me 数据 + silent refresh + tab session 同步
// 使用 partner-api endpoints；`tnbiz:auth:expired` 由 client interceptor 派发
import { useCallback, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import * as api from "@/api/partner";
import { useAuthStore } from "@/stores/authStore";

export function useAuth(): {
  me: api.PartnerMe | undefined;
  loading: boolean;
  login: (input: api.LoginInput) => Promise<api.LoginResp>;
  logout: (scope?: "current" | "all") => Promise<void>;
} {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const setMe = useAuthStore((s) => s.setMe);
  const clear = useAuthStore((s) => s.clear);

  const { data: me, isLoading } = useQuery({
    queryKey: ["partner", "me"],
    queryFn: () => api.getPartnerMe(),
    staleTime: 60_000,
    retry: 0,
  });

  useEffect(() => {
    if (me) setMe(me);
  }, [me, setMe]);

  const login = useCallback(
    async (input: api.LoginInput) => {
      const res = await api.login(input);
      await qc.invalidateQueries({ queryKey: ["partner", "me"] });
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

  // 监听 silent-refresh 失败 → 跳登录
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
