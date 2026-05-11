import { Routes, Route, Navigate } from "react-router-dom";
import { ErrorBoundary } from "@/components/ErrorBoundary";
import { Layout } from "@/components/Layout";
import { RoleGuard } from "@/components/RoleGuard";
import { Login } from "@/pages/Login";
import { Forgot, Reset } from "@/pages/Auth";
import { Dashboard } from "@/pages/Dashboard";
import { Balance } from "@/pages/Balance";
import { Topup, TopupStatusPage } from "@/pages/Topup";
import { ApiKeys } from "@/pages/ApiKeys";
import { Usage } from "@/pages/Usage";
import { Kyc } from "@/pages/Kyc";
import { Tickets, TicketDetail } from "@/pages/Tickets";
import { OrphanNotice } from "@/pages/OrphanNotice";
import { PiplRights } from "@/pages/PiplRights";
import { SwitchPartner } from "@/pages/SwitchPartner";
import { Invoices, InvoiceDetail } from "@/pages/Invoice";
import { Settings, Consent } from "@/pages/Settings";
import { NotFound } from "@/pages/NotFound";

export function App(): JSX.Element {
  return (
    <ErrorBoundary>
      <Routes>
        <Route path="/auth/login" element={<Login />} />
        <Route path="/auth/forgot" element={<Forgot />} />
        <Route path="/auth/reset/:token" element={<Reset />} />
        <Route element={<RoleGuard />}>
          <Route element={<Layout />}>
            <Route path="/" element={<Navigate to="/dashboard" replace />} />
            <Route path="/dashboard" element={<Dashboard />} />
            <Route path="/balance" element={<Balance />} />
            <Route path="/topup" element={<Topup />} />
            <Route path="/topup/:id" element={<TopupStatusPage />} />
            <Route path="/api-keys" element={<ApiKeys />} />
            <Route path="/usage" element={<Usage />} />
            <Route path="/kyc" element={<Kyc />} />
            <Route path="/tickets" element={<Tickets />} />
            <Route path="/tickets/:id" element={<TicketDetail />} />
            <Route path="/orphan-notice" element={<OrphanNotice />} />
            <Route path="/pipl-rights" element={<PiplRights />} />
            <Route path="/switch-partner" element={<SwitchPartner />} />
            <Route path="/invoice" element={<Invoices />} />
            <Route path="/invoice/:id" element={<InvoiceDetail />} />
            <Route path="/settings" element={<Settings />} />
            <Route path="/consent" element={<Consent />} />
            <Route path="*" element={<NotFound />} />
          </Route>
        </Route>
      </Routes>
    </ErrorBoundary>
  );
}
