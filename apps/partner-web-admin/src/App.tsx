import { Routes, Route, Navigate } from "react-router-dom";
import { ErrorBoundary } from "@/components/ErrorBoundary";
import { Layout } from "@/components/Layout";
import { RoleGuard } from "@/components/RoleGuard";
import { Login, MfaEnroll } from "@/pages/Auth";
import { Partners, PartnerNew, PartnerDetailPage } from "@/pages/Partners";
import { KycList, KycDetail } from "@/pages/Kyc";
import { Wallet, WalletTopup } from "@/pages/Wallet";
import { Settlements, SettlementDetailPage } from "@/pages/Settlements";
import { Refunds, RedFlush } from "@/pages/Refunds";
import { AuditLog } from "@/pages/AuditLog";
import { ContentSafetyReports, ContentSafetyReportDetail } from "@/pages/ContentSafety";
import { Pia, PiplComplaints, PiplComplaintDetail } from "@/pages/PiaPipl";
import { BizSettings, SecuritySettings, SagaForceResolve } from "@/pages/System";
import { StaffList, StaffDetail } from "@/pages/Staff";
import { NotFound } from "@/pages/NotFound";

export function App(): JSX.Element {
  return (
    <ErrorBoundary>
      <Routes>
        <Route path="/auth/login" element={<Login />} />
        <Route path="/auth/mfa-enroll" element={<MfaEnroll />} />
        <Route element={<RoleGuard />}>
          <Route element={<Layout />}>
            <Route path="/" element={<Navigate to="/partners" replace />} />
            <Route path="/partners" element={<Partners />} />
            <Route path="/partners/new" element={<PartnerNew />} />
            <Route path="/partners/:id" element={<PartnerDetailPage />} />
            <Route path="/kyc" element={<KycList />} />
            <Route path="/kyc/:id" element={<KycDetail />} />
            <Route path="/wallet" element={<Wallet />} />
            <Route path="/wallet/topup" element={<WalletTopup />} />
            <Route path="/settlements" element={<Settlements />} />
            <Route path="/settlements/:id" element={<SettlementDetailPage />} />
            <Route path="/refunds" element={<Refunds />} />
            <Route path="/red-flush" element={<RedFlush />} />
            <Route path="/audit-log" element={<AuditLog />} />
            <Route path="/content-safety/reports" element={<ContentSafetyReports />} />
            <Route path="/content-safety/reports/:id" element={<ContentSafetyReportDetail />} />
            <Route path="/pia" element={<Pia />} />
            <Route path="/pipl-complaints" element={<PiplComplaints />} />
            <Route path="/pipl-complaints/:id" element={<PiplComplaintDetail />} />
            <Route path="/system/security" element={<SecuritySettings />} />
            <Route path="/system/biz-settings" element={<BizSettings />} />
            <Route path="/saga/force-resolve" element={<SagaForceResolve />} />
            <Route path="/staff" element={<StaffList />} />
            <Route path="/staff/:id" element={<StaffDetail />} />
            <Route path="*" element={<NotFound />} />
          </Route>
        </Route>
      </Routes>
    </ErrorBoundary>
  );
}
