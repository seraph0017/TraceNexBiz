// 路由守卫 —— 校验 me 是 partner；未登录 → /auth/login
import { Navigate, Outlet, useLocation } from "react-router-dom";
import { Spin } from "@douyinfe/semi-ui";
import { useAuth } from "@/hooks/useAuth";

export function RoleGuard(): JSX.Element {
  const { me, loading } = useAuth();
  const loc = useLocation();
  if (loading) {
    return (
      <div style={{ padding: 64, textAlign: "center" }}>
        <Spin size="large" />
      </div>
    );
  }
  if (!me) {
    return <Navigate to="/auth/login" replace state={{ from: loc.pathname }} />;
  }
  // KYC 通过后强制 MFA（PRD §22.1 F-9 / frontend §6）
  if (me.kyc_status === "approved" && !me.mfa_enrolled && !loc.pathname.startsWith("/auth/mfa")) {
    return <Navigate to="/auth/mfa" replace />;
  }
  return <Outlet />;
}
