// customer-api endpoints — typed wrappers，对齐 HANDOFF-W1a/W1b/W1c
// 后端 endpoint base：/api/customer 与 /api/public/auth；/api/partner/* 不被 customer 调用
import { apiClient, unwrap, genUUID } from "./client";
import type { ApiEnvelope, PaginatedEnvelope, PageMeta } from "./types";

// ─── 鉴权（与 partner / admin 共用 /api/public/auth） ─────────────────
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
export async function forgotPassword(email: string): Promise<void> {
  await apiClient.post("/api/public/auth/password/forgot", { email });
}
export async function resetPassword(token: string, newPassword: string): Promise<void> {
  await apiClient.post("/api/public/auth/password/reset", { token, new_password: newPassword });
}

// ─── customer self ──────────────────────────────────────────────────
export interface CustomerMe {
  id: number;
  fy_user_id: number;
  display_name: string;
  email_masked: string;
  phone_masked: string;
  status: "active" | "suspended" | "orphaned" | "transferring";
  partner_id: number | null;
  partner_name: string | null;
  partner_terminated_at: string | null; // 终止时间，与 30d 宽限有关（场景 I）
  kyc_status: "none" | "submitted" | "approved" | "rejected";
  consent_pipl_signed: boolean;
}
export async function getCustomerMe(): Promise<CustomerMe> {
  return unwrap(apiClient.get<ApiEnvelope<CustomerMe>>("/api/customer/me"));
}

// ─── dashboard ──────────────────────────────────────────────────────
export interface DashboardSummary {
  balance: number; // 分
  monthly_calls: number;
  monthly_cost: number; // 分
  remaining_quota: number;
  active_models: number;
  trend_30d: { date: string; calls: number; cost: number }[];
  data_as_of: string;
}
export async function getDashboard(): Promise<DashboardSummary> {
  return unwrap(apiClient.get<ApiEnvelope<DashboardSummary>>("/api/customer/dashboard"));
}

// ─── balance / wallet ───────────────────────────────────────────────
export interface BalanceSnapshot {
  balance: number;
  currency: string;
  updated_at: string;
}
export async function getBalance(): Promise<BalanceSnapshot> {
  return unwrap(apiClient.get<ApiEnvelope<BalanceSnapshot>>("/api/customer/balance"));
}

// ─── topup（场景 D / §7.5） ─────────────────────────────────────────
export interface TopupIntent {
  intent_id: string;
  redirect_url: string;
  saga_id: string;
}
export async function startTopup(amount: number): Promise<TopupIntent> {
  return unwrap(apiClient.post<ApiEnvelope<TopupIntent>>("/api/customer/topup", { amount }));
}
export interface TopupStatus {
  status: "processing" | "funded" | "pending_unknown" | "escalated" | "failed";
  saga_id: string;
  amount: number;
}
export async function getTopupStatus(id: string): Promise<TopupStatus> {
  return unwrap(apiClient.get<ApiEnvelope<TopupStatus>>(`/api/customer/topup/${id}`));
}

// ─── api keys（PRD §M3-03，customer 自己的 sk-key） ─────────────────
export interface ApiKey {
  id: number;
  name: string;
  prefix: string; // 前 8 字符
  last_used_at: string | null;
  status: "active" | "revoked";
  created_at: string;
}
export interface NewApiKeyResp extends ApiKey {
  raw_key: string; // 一次性返回；服务端不存
}
export async function listApiKeys(): Promise<ApiKey[]> {
  const res = await apiClient.get<PaginatedEnvelope<ApiKey>>("/api/customer/api-keys");
  return res.data.data ?? [];
}
export async function createApiKey(name: string): Promise<NewApiKeyResp> {
  return unwrap(apiClient.post<ApiEnvelope<NewApiKeyResp>>("/api/customer/api-keys", { name }));
}
export async function revokeApiKey(id: number): Promise<void> {
  await apiClient.delete(`/api/customer/api-keys/${id}`);
}

// ─── usage ──────────────────────────────────────────────────────────
export interface UsageRow {
  date: string;
  model: string;
  calls: number;
  prompt_tokens: number;
  completion_tokens: number;
  cost: number; // 分
}
export interface UsageFilter {
  page?: number;
  limit?: number;
  start?: string;
  end?: string;
  model?: string;
}
export async function listUsage(f: UsageFilter): Promise<{ items: UsageRow[]; meta?: PageMeta }> {
  const res = await apiClient.get<PaginatedEnvelope<UsageRow>>("/api/customer/usage", { params: f });
  return { items: res.data.data ?? [], meta: res.data.meta };
}

// ─── KYC ────────────────────────────────────────────────────────────
export interface KycStatus {
  status: CustomerMe["kyc_status"];
  reject_reason?: string;
  submitted_at?: string;
  approved_at?: string;
}
export async function getKycStatus(): Promise<KycStatus> {
  return unwrap(apiClient.get<ApiEnvelope<KycStatus>>("/api/customer/kyc/status"));
}
export async function submitKyc(payload: Record<string, unknown>): Promise<{ id: number; status: string }> {
  return unwrap(apiClient.post<ApiEnvelope<{ id: number; status: string }>>("/api/customer/kyc", payload));
}

// ─── tickets ────────────────────────────────────────────────────────
export interface Ticket {
  id: number;
  subject: string;
  status: "open" | "pending" | "resolved" | "closed";
  priority: "low" | "normal" | "high" | "urgent";
  target: "platform" | "partner";
  created_at: string;
  updated_at: string;
}
export interface TicketMessage {
  id: number;
  author: "partner" | "customer" | "staff";
  body: string;
  created_at: string;
}
export interface TicketDetail extends Ticket {
  body: string;
  messages: TicketMessage[];
}
export async function listTickets(): Promise<Ticket[]> {
  const res = await apiClient.get<PaginatedEnvelope<Ticket>>("/api/customer/tickets");
  return res.data.data ?? [];
}
export async function getTicket(id: number): Promise<TicketDetail> {
  return unwrap(apiClient.get<ApiEnvelope<TicketDetail>>(`/api/customer/tickets/${id}`));
}
export async function createTicket(input: {
  subject: string;
  body: string;
  priority?: Ticket["priority"];
  target: Ticket["target"];
}): Promise<Ticket> {
  return unwrap(apiClient.post<ApiEnvelope<Ticket>>("/api/customer/tickets", input));
}
export async function replyTicket(id: number, body: string): Promise<TicketMessage> {
  return unwrap(
    apiClient.post<ApiEnvelope<TicketMessage>>(`/api/customer/tickets/${id}/reply`, { body }),
  );
}

// ─── orphan / switch partner（场景 H / I） ──────────────────────────
export interface OrphanState {
  orphaned_at: string | null;
  grace_period_ends_at: string | null;
  options: ("adopt" | "direct" | "switch")[];
  candidate_partners: { id: number; name: string }[];
}
export async function getOrphanState(): Promise<OrphanState> {
  return unwrap(apiClient.get<ApiEnvelope<OrphanState>>("/api/customer/orphan"));
}
export async function chooseOrphanOption(input: {
  decision: "adopt" | "direct" | "switch";
  target_partner_id?: number;
}): Promise<{ status: string }> {
  return unwrap(
    apiClient.post<ApiEnvelope<{ status: string }>>("/api/customer/orphan/decide", input),
  );
}

export interface SwitchPartnerRequest {
  id: number;
  current_partner_id: number;
  target_partner_id: number;
  status: "submitted" | "approved" | "rejected" | "applied";
  created_at: string;
}
export async function listSwitchRequests(): Promise<SwitchPartnerRequest[]> {
  const res = await apiClient.get<PaginatedEnvelope<SwitchPartnerRequest>>(
    "/api/customer/switch-partner",
  );
  return res.data.data ?? [];
}
export async function submitSwitchRequest(targetPartnerId: number): Promise<SwitchPartnerRequest> {
  return unwrap(
    apiClient.post<ApiEnvelope<SwitchPartnerRequest>>("/api/customer/switch-partner", {
      target_partner_id: targetPartnerId,
    }),
  );
}
export async function confirmSwitchRequest(id: number): Promise<SwitchPartnerRequest> {
  return unwrap(
    apiClient.post<ApiEnvelope<SwitchPartnerRequest>>(`/api/customer/switch-partner/${id}/confirm`, {}),
  );
}

// ─── invoice（PRD §7.8 + M8 红冲） ──────────────────────────────────
export interface Invoice {
  id: number;
  amount: number;
  status: "applying" | "issued" | "rejected" | "red_flushed";
  type: "blue" | "red"; // 红冲
  origin_invoice_id?: number;
  title: string;
  tax_no: string;
  applied_at: string;
  issued_at?: string;
  pdf_url?: string;
}
export async function listInvoices(): Promise<Invoice[]> {
  const res = await apiClient.get<PaginatedEnvelope<Invoice>>("/api/customer/invoice");
  return res.data.data ?? [];
}
export async function getInvoice(id: number): Promise<Invoice> {
  return unwrap(apiClient.get<ApiEnvelope<Invoice>>(`/api/customer/invoice/${id}`));
}
export async function applyInvoice(input: {
  amount: number;
  title: string;
  tax_no: string;
  email?: string;
}): Promise<Invoice> {
  return unwrap(apiClient.post<ApiEnvelope<Invoice>>("/api/customer/invoice", input));
}
export async function applyRedFlush(originId: number, reasonCode: string, reasonText?: string): Promise<Invoice> {
  return unwrap(
    apiClient.post<ApiEnvelope<Invoice>>(`/api/customer/invoice/${originId}/red-flush-apply`, {
      reason_code: reasonCode,
      reason_text: reasonText,
    }),
  );
}

// ─── settings + consent（PIPL §44-§47） ─────────────────────────────
export interface CustomerSettings {
  display_name: string;
  email_masked: string;
  phone_masked: string;
  notify_email_enabled: boolean;
  notify_inapp_enabled: boolean;
  preferred_locale: "zh-CN" | "en-US";
}
export async function getSettings(): Promise<CustomerSettings> {
  return unwrap(apiClient.get<ApiEnvelope<CustomerSettings>>("/api/customer/settings"));
}
export async function updateSettings(input: Partial<CustomerSettings>): Promise<CustomerSettings> {
  return unwrap(
    apiClient.put<ApiEnvelope<CustomerSettings>>("/api/customer/settings", input),
  );
}

export interface ConsentRecord {
  id: number;
  scope: string; // pipl_collection / pipl_sharing / marketing 等
  granted: boolean;
  signed_at: string;
  text_version: string;
}
export async function listConsents(): Promise<ConsentRecord[]> {
  const res = await apiClient.get<PaginatedEnvelope<ConsentRecord>>("/api/customer/consent");
  return res.data.data ?? [];
}
export async function revokeConsent(scope: string): Promise<ConsentRecord> {
  return unwrap(apiClient.post<ApiEnvelope<ConsentRecord>>("/api/customer/consent/revoke", { scope }));
}

// ─── PIPL 权利（场景 Q：导出 / 删除 / 撤回） ────────────────────────
export interface PiplRequest {
  id: number;
  kind: "export" | "delete" | "rectify" | "portability";
  status: "submitted" | "processing" | "completed" | "rejected";
  download_url?: string;
  created_at: string;
  resolved_at?: string;
}
export async function listPiplRequests(): Promise<PiplRequest[]> {
  const res = await apiClient.get<PaginatedEnvelope<PiplRequest>>("/api/customer/pipl");
  return res.data.data ?? [];
}
export async function submitPiplRequest(kind: PiplRequest["kind"], reason?: string): Promise<PiplRequest> {
  return unwrap(
    apiClient.post<ApiEnvelope<PiplRequest>>("/api/customer/pipl", { kind, reason }),
  );
}

export { genUUID };
