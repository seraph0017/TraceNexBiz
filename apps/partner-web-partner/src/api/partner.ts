// partner-api endpoints — typed wrappers
// 字段对齐 HANDOFF-W1a/W1b/W1c。后端不存在的字段（如 dashboard 聚合）走 Fy-api 直接读 mock 兜底。
import { apiClient, unwrap, genUUID } from "./client";
import type { ApiEnvelope, PaginatedEnvelope, PageMeta } from "./types";

// ─── 鉴权 ───────────────────────────────────────────────────────────
export interface LoginInput {
  site: "partner" | "customer" | "admin";
  handle: string;
  password: string;
  otp?: string;
  device_fingerprint?: string;
}
export interface LoginResp {
  actor_type: string;
  actor_id: number;
  fy_user_id: number;
  expires_at: string;
}
export async function login(input: LoginInput): Promise<LoginResp> {
  return unwrap(apiClient.post<ApiEnvelope<LoginResp>>("/api/public/auth/login", input));
}
export async function logout(scope: "current" | "all" = "current"): Promise<void> {
  await apiClient.post("/api/public/auth/logout", { scope });
}
export async function refresh(): Promise<{ expires_at: string }> {
  return unwrap(apiClient.post<ApiEnvelope<{ expires_at: string }>>("/api/public/auth/refresh", {}));
}
export interface MfaEnrollResp {
  challenge: string;
  rp_id: string;
}
export async function mfaEnroll(): Promise<MfaEnrollResp> {
  return unwrap(apiClient.post<ApiEnvelope<MfaEnrollResp>>("/api/partner/mfa/enroll", {}));
}
export async function mfaVerify(payload: Record<string, unknown>): Promise<{ verified: boolean }> {
  return unwrap(
    apiClient.post<ApiEnvelope<{ verified: boolean }>>("/api/partner/mfa/verify", payload),
  );
}

// ─── partner self ───────────────────────────────────────────────────
export interface PartnerMe {
  id: number;
  type: "individual" | "enterprise";
  status: string;
  contact_name: string;
  contact_phone_masked: string;
  contact_email_masked: string;
  kyc_status: "none" | "submitted" | "approved" | "rejected" | "frozen_yearly_limit";
  kyc_reject_reason?: string;
  mfa_enrolled: boolean;
}
export async function getPartnerMe(): Promise<PartnerMe> {
  return unwrap(apiClient.get<ApiEnvelope<PartnerMe>>("/api/partner/me"));
}

// ─── dashboard ──────────────────────────────────────────────────────
export interface DashboardSummary {
  balance: number; // 分
  available: number;
  held_total: number;
  open_holds_count: number;
  monthly_gross: number;
  monthly_cost: number;
  monthly_net: number;
  customers_active: number;
  customers_new: number;
  customers_churn: number;
  kyc_due_within_30d: number;
  data_as_of: string;
  outbox_lag_seconds?: number;
  trend_30d: { date: string; net: number }[];
}
export async function getDashboard(): Promise<DashboardSummary> {
  return unwrap(apiClient.get<ApiEnvelope<DashboardSummary>>("/api/partner/dashboard"));
}

// ─── wallet ─────────────────────────────────────────────────────────
export interface WalletSnapshot {
  wallet: {
    partner_id: number;
    balance: number;
    currency: string;
    updated_at: string;
  };
  held_total: number;
  available: number;
  open_holds_count: number;
}
export async function getWallet(): Promise<WalletSnapshot> {
  return unwrap(apiClient.get<ApiEnvelope<WalletSnapshot>>("/api/partner/wallet"));
}

export interface WalletLog {
  id: number;
  type: string;
  amount: number;
  balance_after: number;
  ref: string;
  note: string;
  created_at: string;
}
export async function getWalletLogs(params: {
  page?: number;
  limit?: number;
  type?: string;
}): Promise<{ items: WalletLog[]; meta?: PageMeta }> {
  const res = await apiClient.get<PaginatedEnvelope<WalletLog>>("/api/partner/wallet/logs", { params });
  if (!res.data.success) throw new Error(res.data.error?.code ?? "unknown");
  return { items: res.data.data ?? [], meta: res.data.meta };
}

export interface WalletHold {
  id: number;
  saga_id: string;
  amount: number;
  status: "held" | "released" | "captured";
  reason: string;
  created_at: string;
}
export async function getWalletHolds(): Promise<WalletHold[]> {
  const res = await apiClient.get<PaginatedEnvelope<WalletHold>>("/api/partner/wallet/holds");
  return res.data.data ?? [];
}

// ─── customers ──────────────────────────────────────────────────────
export interface CustomerListItem {
  id: number;
  fy_user_id: number;
  display_name: string;
  email_masked: string;
  status: string;
  monthly_calls: number;
  remaining_quota: number;
  created_at: string;
}
export interface CustomerListFilter {
  page?: number;
  limit?: number;
  status?: string;
  q?: string;
}
export async function listCustomers(
  f: CustomerListFilter,
): Promise<{ items: CustomerListItem[]; meta?: PageMeta }> {
  const res = await apiClient.get<PaginatedEnvelope<CustomerListItem>>("/api/partner/customers", {
    params: f,
  });
  return { items: res.data.data ?? [], meta: res.data.meta };
}

export interface CustomerDetail extends CustomerListItem {
  quota_total: number;
  quota_used: number;
  ticket_open_count: number;
  channel_pricing_override?: number;
}
export async function getCustomer(id: number): Promise<CustomerDetail> {
  return unwrap(apiClient.get<ApiEnvelope<CustomerDetail>>(`/api/partner/customers/${id}`));
}

export interface CustomerUsageRow {
  date: string;
  calls: number;
  cost: number;
}
export async function getCustomerUsage(id: number): Promise<CustomerUsageRow[]> {
  const res = await apiClient.get<PaginatedEnvelope<CustomerUsageRow>>(
    `/api/partner/customers/${id}/usage`,
  );
  return res.data.data ?? [];
}

// ─── allocate (saga) ────────────────────────────────────────────────
export interface AllocateInput {
  customer_id: number;
  amount: number;
  note?: string;
}
export interface SagaState {
  saga_id: string;
  state: "pending" | "succeeded" | "failed" | "compensating" | "unknown" | "escalated";
  steps: { name: string; status: string; updated_at: string }[];
  error_code?: string;
  trace_id: string;
}
export async function startAllocate(input: AllocateInput): Promise<{ saga_id: string }> {
  // idempotency-key 由 client 自动生成
  return unwrap(
    apiClient.post<ApiEnvelope<{ saga_id: string }>>("/api/partner/allocate", input),
  );
}
export async function getSagaState(sagaId: string): Promise<SagaState> {
  return unwrap(apiClient.get<ApiEnvelope<SagaState>>(`/api/partner/saga/${sagaId}`));
}

// ─── invitations ────────────────────────────────────────────────────
export interface Invitation {
  id: number;
  code: string;
  type: "permanent" | "one_time" | "limited";
  usage_limit?: number;
  used_count: number;
  status: "active" | "used_up" | "revoked";
  created_at: string;
}
export async function listInvitations(): Promise<Invitation[]> {
  const res = await apiClient.get<PaginatedEnvelope<Invitation>>("/api/partner/invitation");
  return res.data.data ?? [];
}
export async function createInvitation(input: {
  type: Invitation["type"];
  usage_limit?: number;
}): Promise<Invitation> {
  return unwrap(apiClient.post<ApiEnvelope<Invitation>>("/api/partner/invitation", input));
}
export async function revokeInvitation(id: number): Promise<void> {
  await apiClient.delete(`/api/partner/invitation/${id}`);
}

// ─── pricing ────────────────────────────────────────────────────────
export interface PricingRule {
  model_id: string;
  model_name: string;
  base_per_million: number; // 平台基价（分/百万 token）
  markup_bps: number; // partner markup（万分比）
  effective_from: string;
}
export async function listPricing(): Promise<PricingRule[]> {
  const res = await apiClient.get<PaginatedEnvelope<PricingRule>>("/api/partner/pricing");
  return res.data.data ?? [];
}
export async function updatePricing(
  modelId: string,
  markupBps: number,
): Promise<PricingRule> {
  return unwrap(
    apiClient.put<ApiEnvelope<PricingRule>>(`/api/partner/pricing/${modelId}`, { markup_bps: markupBps }),
  );
}

// ─── statements ─────────────────────────────────────────────────────
export interface Statement {
  id: number;
  period: string; // YYYY-MM
  gross: number;
  cost: number;
  net: number;
  status: "draft" | "issued" | "paid";
  issued_at?: string;
}
export async function listStatements(): Promise<Statement[]> {
  const res = await apiClient.get<PaginatedEnvelope<Statement>>("/api/partner/statements");
  return res.data.data ?? [];
}
export interface StatementDetail extends Statement {
  line_items: { id: number; description: string; quantity: number; unit_price: number; amount: number }[];
  invoice_id?: number;
}
export async function getStatement(id: number): Promise<StatementDetail> {
  return unwrap(apiClient.get<ApiEnvelope<StatementDetail>>(`/api/partner/statements/${id}`));
}
export async function applyInvoice(input: { statement_id: number; title_id: number }): Promise<{ id: number }> {
  return unwrap(apiClient.post<ApiEnvelope<{ id: number }>>("/api/partner/invoice/apply", input));
}

// ─── disputes ───────────────────────────────────────────────────────
export interface Dispute {
  id: number;
  account_kind: "fy_account" | "tn_account";
  reason: string;
  amount: number;
  status: "submitted" | "reviewing" | "accepted" | "rejected";
  created_at: string;
}
export async function listDisputes(): Promise<Dispute[]> {
  const res = await apiClient.get<PaginatedEnvelope<Dispute>>("/api/partner/disputes");
  return res.data.data ?? [];
}
export async function createDispute(input: {
  account_kind: Dispute["account_kind"];
  amount: number;
  reason: string;
  evidence_url?: string;
}): Promise<Dispute> {
  return unwrap(apiClient.post<ApiEnvelope<Dispute>>("/api/partner/disputes", input));
}
export async function getDispute(id: number): Promise<Dispute> {
  return unwrap(apiClient.get<ApiEnvelope<Dispute>>(`/api/partner/disputes/${id}`));
}

// ─── tickets ────────────────────────────────────────────────────────
export interface Ticket {
  id: number;
  subject: string;
  status: "open" | "pending" | "resolved" | "closed";
  priority: "low" | "normal" | "high" | "urgent";
  customer_id?: number;
  created_at: string;
  updated_at: string;
}
export interface TicketMessage {
  id: number;
  author: "partner" | "customer" | "staff";
  body: string;
  attachments?: { url: string; name: string }[];
  created_at: string;
}
export interface TicketDetail extends Ticket {
  body: string;
  messages: TicketMessage[];
}
export async function listTickets(): Promise<Ticket[]> {
  const res = await apiClient.get<PaginatedEnvelope<Ticket>>("/api/partner/tickets");
  return res.data.data ?? [];
}
export async function getTicket(id: number): Promise<TicketDetail> {
  return unwrap(apiClient.get<ApiEnvelope<TicketDetail>>(`/api/partner/tickets/${id}`));
}
export async function createTicket(input: {
  subject: string;
  body: string;
  priority?: Ticket["priority"];
  customer_id?: number;
}): Promise<Ticket> {
  return unwrap(apiClient.post<ApiEnvelope<Ticket>>("/api/partner/tickets", input));
}
export async function replyTicket(id: number, body: string): Promise<TicketMessage> {
  return unwrap(
    apiClient.post<ApiEnvelope<TicketMessage>>(`/api/partner/tickets/${id}/reply`, { body }),
  );
}

// ─── settings ───────────────────────────────────────────────────────
export interface PartnerSettings {
  contact_name: string;
  contact_phone_masked: string;
  contact_email_masked: string;
  bank_account_masked: string;
  notify_email_enabled: boolean;
  notify_inapp_enabled: boolean;
}
export async function getSettings(): Promise<PartnerSettings> {
  return unwrap(apiClient.get<ApiEnvelope<PartnerSettings>>("/api/partner/settings"));
}
export async function updateSettings(input: Partial<PartnerSettings>): Promise<PartnerSettings> {
  return unwrap(apiClient.put<ApiEnvelope<PartnerSettings>>("/api/partner/settings", input));
}

// ─── KYC ────────────────────────────────────────────────────────────
export interface KycStatus {
  status: PartnerMe["kyc_status"];
  reject_reason?: string;
  submitted_at?: string;
  approved_at?: string;
  next_review_due_at?: string;
}
export async function getKycStatus(): Promise<KycStatus> {
  return unwrap(apiClient.get<ApiEnvelope<KycStatus>>("/api/partner/kyc/status"));
}
export async function resubmitKyc(payload: Record<string, unknown>): Promise<{ id: number; status: string }> {
  return unwrap(apiClient.post<ApiEnvelope<{ id: number; status: string }>>("/api/partner/kyc", payload));
}

// ─── topup intent（充值） ───────────────────────────────────────────
export interface TopupIntent {
  intent_id: string;
  redirect_url: string;
  saga_id: string;
}
export async function startTopup(amount: number): Promise<TopupIntent> {
  return unwrap(
    apiClient.post<ApiEnvelope<TopupIntent>>("/api/partner/wallet/topup", { amount }),
  );
}
export async function getTopupIntent(id: string): Promise<{
  status: "processing" | "funded" | "pending_unknown" | "escalated" | "failed";
  saga_id: string;
}> {
  return unwrap(apiClient.get<ApiEnvelope<{ status: "processing" | "funded" | "pending_unknown" | "escalated" | "failed"; saga_id: string }>>(
    `/api/partner/wallet/topup/${id}`,
  ));
}

export { genUUID };
