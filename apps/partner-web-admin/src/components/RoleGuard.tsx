// admin RoleGuard：staff JWT + IP 白名单 + step-up MFA + 22 verb 权限
import { Navigate, Outlet, useLocation } from "react-router-dom";
import { Banner, Spin } from "@douyinfe/semi-ui";
import { useTranslation } from "react-i18next";
import { useAuth } from "@/hooks/useAuth";

export function RoleGuard(): JSX.Element {
  const { me, loading } = useAuth();
  const loc = useLocation();
  const { t } = useTranslation();
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
  if (!me.ip_allowed) {
    return (
      <div style={{ padding: 32 }}>
        <Banner type="danger" description={t("auth.ip_blocked")} closeIcon={null} />
      </div>
    );
  }
  // admin 强制 MFA enrollment
  if (!me.mfa_enrolled && !loc.pathname.startsWith("/auth")) {
    return <Navigate to="/auth/mfa-enroll" replace />;
  }
  return <Outlet />;
}

interface PermissionGuardProps {
  verb: string;
  children: React.ReactNode;
  fallback?: React.ReactNode;
}
export function PermissionGuard({ verb, children, fallback }: PermissionGuardProps): JSX.Element {
  const { me } = useAuth();
  const { t } = useTranslation();
  if (!me?.permissions.includes(verb)) {
    return (
      <>
        {fallback ?? (
          <Banner type="warning" description={t("errors.perm.forbidden")} closeIcon={null} />
        )}
      </>
    );
  }
  return <>{children}</>;
}
