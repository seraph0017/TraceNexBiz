// 渠道商后台路由树（per frontend §3.2 partner 部分）
//
// 关键 invariant：
//   - partner JWT；KYC pass + WebAuthn 注册（per backend §7.5 v0.2 强制）
//   - allocate-quota 必须带 idempotency-key（per frontend §7.4 / backend §5.3 saga）
//   - 客户列表 BOLA 强制 partner_id scope（per overview I-3.1）
import { Navigate, Route, Routes } from "react-router-dom";
import { Layout } from "@/components/Layout";
import { RoleGuard } from "@/components/RoleGuard";
import { Login } from "@/pages/Login";
import { Mfa } from "@/pages/Mfa";
import { Dashboard } from "@/pages/Dashboard";
import { Customers } from "@/pages/Customers";
import { NewCustomer } from "@/pages/NewCustomer";
import { CustomerDetail } from "@/pages/CustomerDetail";
import { Allocate } from "@/pages/Allocate";
import { Invitations } from "@/pages/Invitations";
import { Pricing } from "@/pages/Pricing";
import { Wallet } from "@/pages/Wallet";
import { WalletTopup } from "@/pages/WalletTopup";
import { Statements, StatementDetail } from "@/pages/Statements";
import { Disputes, DisputeDetail } from "@/pages/Disputes";
import { Tickets, TicketDetail } from "@/pages/Tickets";
import { Settings } from "@/pages/Settings";
import { Kyc } from "@/pages/Kyc";
import { NotFound } from "@/pages/NotFound";

export function App(): JSX.Element {
  return (
    <Routes>
      <Route path="/auth/login" element={<Login />} />
      <Route path="/auth/mfa" element={<Mfa />} />
      <Route element={<RoleGuard />}>
        <Route element={<Layout />}>
          <Route path="/" element={<Navigate to="/dashboard" replace />} />
          <Route path="/dashboard" element={<Dashboard />} />
          <Route path="/customers" element={<Customers />} />
          <Route path="/customers/new" element={<NewCustomer />} />
          <Route path="/customers/:id" element={<CustomerDetail />} />
          <Route path="/allocate" element={<Allocate />} />
          <Route path="/invitations" element={<Invitations />} />
          <Route path="/pricing" element={<Pricing />} />
          <Route path="/wallet" element={<Wallet />} />
          <Route path="/wallet/topup" element={<WalletTopup />} />
          <Route path="/statements" element={<Statements />} />
          <Route path="/statements/:id" element={<StatementDetail />} />
          <Route path="/disputes" element={<Disputes />} />
          <Route path="/disputes/:id" element={<DisputeDetail />} />
          <Route path="/tickets" element={<Tickets />} />
          <Route path="/tickets/:id" element={<TicketDetail />} />
          <Route path="/settings" element={<Settings />} />
          <Route path="/kyc" element={<Kyc />} />
        </Route>
      </Route>
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}
