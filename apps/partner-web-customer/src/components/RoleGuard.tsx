// 路由守卫 —— 客户站；customer 不强制 MFA（弱化版）；orphan 状态强制弹通知
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
  // orphan 状态需先选择后续方案（场景 I）
  if (
    me.status === "orphaned" &&
    !loc.pathname.startsWith("/orphan-notice") &&
    !loc.pathname.startsWith("/auth")
  ) {
    return <Navigate to="/orphan-notice" replace />;
  }
  return <Outlet />;
}
