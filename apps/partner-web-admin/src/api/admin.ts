// admin-api endpoints — typed wrappers
// 对齐 openapi/admin.yaml + HANDOFF-W1c
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

// ─── staff me ───────────────────────────────────────────────────────
export type StaffRole =
  | "super_admin"
  | "risk_admin"
  | "finance_admin"
  | "cs_admin"
  | "kyc_reviewer";

export interface StaffMe {
  id: number;
  username: string;
  email_masked: string;
  role: StaffRole;
  permissions: string[]; // 22 verb（PRD §3.4）
  ip_allowed: boolean;
  mfa_enrolled: boolean;
  step_up_until?: string; // step-up MFA 有效期
}
export async function getStaffMe(): Promise<StaffMe> {
  return unwrap(apiClient.get<ApiEnvelope<StaffMe>>("/api/admin/me"));
}

export async function stepUpMfa(payload: Record<string, unknown>): Promise<{ valid_until: string }> {
  return unwrap(apiClient.post<ApiEnvelope<{ valid_until: string }>>("/api/admin/mfa/step-up", payload));
}

// ─── partners ───────────────────────────────────────────────────────
export interface Partner {
  id: number;
  display_name: string;
  contact_email_masked: string;
  status: "active" | "suspended" | "terminating" | "terminated";
  kyc_status: string;
  terminated_at: string | null;
  grace_period_ends_at: string | null;
  created_at: string;
}
export interface PartnerDetail extends Partner {
  contact_phone_masked: string;
  bank_account_masked: string;
  monthly_gross: number;
  monthly_net: number;
  customers_count: number;
}
export async function listPartners(params: {
  page?: number;
  limit?: number;
  status?: string;
  q?: string;
}): Promise<{ items: Partner[]; meta?: PageMeta }> {
  const res = await apiClient.get<PaginatedEnvelope<Partner>>("/api/admin/partners", { params });
  return { items: res.data.data ?? [], meta: res.data.meta };
}
export async function getPartner(id: number): Promise<PartnerDetail> {
  return unwrap(apiClient.get<ApiEnvelope<PartnerDetail>>(`/api/admin/partners/${id}`));
}
export async function createPartner(input: {
  contact_name: string;
  contact_email: string;
  contact_phone: string;
}): Promise<Partner> {
  return unwrap(apiClient.post<ApiEnvelope<Partner>>("/api/admin/partners", input));
}
export async function terminatePartner(id: number, reason: string): Promise<{ grace_period_ends_at: string }> {
  return unwrap(
    apiClient.post<ApiEnvelope<{ grace_period_ends_at: string }>>(`/api/admin/partners/${id}/terminate`, { reason }),
  );
}

// ─── KYC review ─────────────────────────────────────────────────────
export interface KycSubmission {
  id: number;
  subject_kind: "partner" | "customer";
  subject_id: number;
  subject_name: string;
  status: "submitted" | "approved" | "rejected" | "frozen_yearly_limit";
  third_party_check?: { provider: string; status: string };
  created_at: string;
}
export async function listKyc(params: {
  page?: number;
  limit?: number;
  status?: string;
}): Promise<{ items: KycSubmission[]; meta?: PageMeta }> {
  const res = await apiClient.get<PaginatedEnvelope<KycSubmission>>("/api/admin/kyc", { params });
  return { items: res.data.data ?? [], meta: res.data.meta };
}
export async function getKyc(id: number): Promise<KycSubmission & { real_name_masked: string; id_card_masked: string; documents: { kind: string; url: string }[] }> {
  return unwrap(
    apiClient.get<
      ApiEnvelope<KycSubmission & { real_name_masked: string; id_card_masked: string; documents: { kind: string; url: string }[] }>
    >(`/api/admin/kyc/${id}`),
  );
}
export async function reviewKyc(id: number, input: { approve: boolean; reject_code?: string; reject_text?: string }): Promise<{ status: string }> {
  return unwrap(apiClient.post<ApiEnvelope<{ status: string }>>(`/api/admin/kyc/${id}/review`, input));
}
export async function callThirdPartyCheck(id: number): Promise<{ provider: string; status: string }> {
  return unwrap(
    apiClient.post<ApiEnvelope<{ provider: string; status: string }>>(`/api/admin/kyc/${id}/third-party`, {}),
  );
}

// ─── wallet（admin 调整） ───────────────────────────────────────────
export interface AdminWalletEntry {
  id: number;
  partner_id: number;
  partner_name: string;
  balance: number;
  updated_at: string;
}
export async function listWallets(params: { page?: number; limit?: number }): Promise<{ items: AdminWalletEntry[]; meta?: PageMeta }> {
  const res = await apiClient.get<PaginatedEnvelope<AdminWalletEntry>>("/api/admin/wallet", { params });
  return { items: res.data.data ?? [], meta: res.data.meta };
}
export async function adminTopupWallet(input: { partner_id: number; amount: number; reason: string }): Promise<{ saga_id: string }> {
  return unwrap(apiClient.post<ApiEnvelope<{ saga_id: string }>>("/api/admin/wallet/topup", input));
}

// ─── settlements ────────────────────────────────────────────────────
export interface SettlementBatch {
  id: number;
  period: string;
  status: "draft" | "locked" | "dispatched" | "settled";
  partners_count: number;
  total_amount: number;
  locked_at?: string;
  dispatched_at?: string;
}
export interface SettlementDetail extends SettlementBatch {
  rows: { partner_id: number; partner_name: string; gross: number; cost: number; net: number; tax: number; payable: number }[];
  receipt_url?: string;
}
export async function listSettlements(): Promise<SettlementBatch[]> {
  const res = await apiClient.get<PaginatedEnvelope<SettlementBatch>>("/api/admin/settlements");
  return res.data.data ?? [];
}
export async function getSettlement(id: number): Promise<SettlementDetail> {
  return unwrap(apiClient.get<ApiEnvelope<SettlementDetail>>(`/api/admin/settlements/${id}`));
}
export async function lockSettlement(id: number): Promise<SettlementBatch> {
  return unwrap(apiClient.post<ApiEnvelope<SettlementBatch>>(`/api/admin/settlements/${id}/lock`, {}));
}
export async function dispatchSettlement(id: number): Promise<SettlementBatch> {
  return unwrap(apiClient.post<ApiEnvelope<SettlementBatch>>(`/api/admin/settlements/${id}/dispatch`, {}));
}
export async function reconcileSettlement(id: number, receiptId: string): Promise<{ status: string }> {
  return unwrap(
    apiClient.post<ApiEnvelope<{ status: string }>>(`/api/admin/settlements/${id}/reconcile`, { receipt_id: receiptId }),
  );
}

// ─── refunds + red-flush ────────────────────────────────────────────
export interface Refund {
  id: number;
  origin_kind: "topup" | "settlement";
  origin_id: number;
  amount: number;
  status: "submitted" | "reviewing" | "approved" | "rejected" | "executing" | "completed";
  reason: string;
  created_at: string;
}
export async function listRefunds(): Promise<Refund[]> {
  const res = await apiClient.get<PaginatedEnvelope<Refund>>("/api/admin/refunds");
  return res.data.data ?? [];
}
export async function createRefund(input: { origin_kind: Refund["origin_kind"]; origin_id: number; amount: number; reason: string }): Promise<Refund> {
  return unwrap(apiClient.post<ApiEnvelope<Refund>>("/api/admin/refunds", input));
}
export async function reviewRefund(id: number, approve: boolean, note?: string): Promise<Refund> {
  return unwrap(apiClient.post<ApiEnvelope<Refund>>(`/api/admin/refunds/${id}/review`, { approve, note }));
}

export interface RedFlushInvoice {
  id: number;
  origin_invoice_id: number;
  amount: number;
  reason_code: string;
  status: "applying" | "approved" | "issued";
  created_at: string;
}
export async function listRedFlush(): Promise<RedFlushInvoice[]> {
  const res = await apiClient.get<PaginatedEnvelope<RedFlushInvoice>>("/api/admin/red-flush");
  return res.data.data ?? [];
}
export async function reviewInvoice(id: number, input: { approve: boolean; reject_code?: string; reject_text?: string }): Promise<{ status: string }> {
  return unwrap(apiClient.post<ApiEnvelope<{ status: string }>>(`/api/admin/invoice/${id}/review`, input));
}
export async function issueInvoice(id: number): Promise<{ status: string }> {
  return unwrap(apiClient.post<ApiEnvelope<{ status: string }>>(`/api/admin/invoice/${id}/issue`, {}));
}
export async function redFlushInvoice(id: number, input: { reason_code: string; reason_text?: string }): Promise<{ status: string }> {
  return unwrap(apiClient.post<ApiEnvelope<{ status: string }>>(`/api/admin/invoice/${id}/red-flush`, input));
}

// ─── audit log ──────────────────────────────────────────────────────
export interface AuditEntry {
  id: number;
  ts: string;
  actor_kind: string;
  actor_id: number;
  action: string;
  target: string;
  result: string;
  ip: string;
  trace_id: string;
  prev_hash?: string;
  hash?: string;
}
export async function listAudit(params: { page?: number; limit?: number; action?: string; actor?: string }): Promise<{ items: AuditEntry[]; meta?: PageMeta }> {
  const res = await apiClient.get<PaginatedEnvelope<AuditEntry>>("/api/admin/audit-log", { params });
  return { items: res.data.data ?? [], meta: res.data.meta };
}
export async function verifyAuditChain(start?: number, end?: number): Promise<{ ok: boolean; broken_at?: number; checked: number }> {
  return unwrap(
    apiClient.post<ApiEnvelope<{ ok: boolean; broken_at?: number; checked: number }>>("/api/admin/audit-log/verify", { start, end }),
  );
}

// ─── content safety / 12377 ────────────────────────────────────────
export interface ContentSafetyReport {
  id: number;
  source: string;
  content_excerpt: string;
  status: "pending" | "dispatching" | "submitted" | "failed";
  remote_ref?: string;
  retries: number;
  created_at: string;
}
export async function listContentReports(params: { page?: number; limit?: number; status?: string }): Promise<{ items: ContentSafetyReport[]; meta?: PageMeta }> {
  const res = await apiClient.get<PaginatedEnvelope<ContentSafetyReport>>("/api/admin/content-safety/reports", { params });
  return { items: res.data.data ?? [], meta: res.data.meta };
}
export async function getContentReport(id: number): Promise<ContentSafetyReport & { full_content: string; metadata: Record<string, unknown> }> {
  return unwrap(apiClient.get<ApiEnvelope<ContentSafetyReport & { full_content: string; metadata: Record<string, unknown> }>>(`/api/admin/content-safety/reports/${id}`));
}
export async function retryContentReport(id: number): Promise<ContentSafetyReport> {
  return unwrap(apiClient.post<ApiEnvelope<ContentSafetyReport>>(`/api/admin/content-safety/reports/${id}/retry`, {}));
}
export async function dispatchContentReports(batch: number): Promise<{ dispatched: number }> {
  return unwrap(apiClient.post<ApiEnvelope<{ dispatched: number }>>("/api/admin/content-safety/reports/dispatch", { batch }));
}

// ─── PIA + PIPL complaints ─────────────────────────────────────────
export interface PiaReport {
  id: number;
  scope: string;
  status: "draft" | "approved" | "published";
  generated_at: string;
  download_url?: string;
}
export async function listPia(): Promise<PiaReport[]> {
  const res = await apiClient.get<PaginatedEnvelope<PiaReport>>("/api/admin/pia");
  return res.data.data ?? [];
}
export async function generatePia(scope: string): Promise<PiaReport> {
  return unwrap(apiClient.post<ApiEnvelope<PiaReport>>("/api/admin/pia/generate", { scope }));
}

export interface PiplComplaint {
  id: number;
  customer_id: number;
  customer_name: string;
  kind: "export" | "delete" | "rectify" | "portability";
  status: "submitted" | "processing" | "resolved" | "rejected";
  created_at: string;
}
export async function listPiplComplaints(): Promise<PiplComplaint[]> {
  const res = await apiClient.get<PaginatedEnvelope<PiplComplaint>>("/api/admin/pipl-complaints");
  return res.data.data ?? [];
}
export async function getPiplComplaint(id: number): Promise<PiplComplaint & { detail: string; timeline: { ts: string; note: string }[] }> {
  return unwrap(apiClient.get<ApiEnvelope<PiplComplaint & { detail: string; timeline: { ts: string; note: string }[] }>>(`/api/admin/pipl-complaints/${id}`));
}
export async function resolvePiplComplaint(id: number, input: { decision: "resolved" | "rejected"; note: string }): Promise<PiplComplaint> {
  return unwrap(apiClient.post<ApiEnvelope<PiplComplaint>>(`/api/admin/pipl-complaints/${id}/resolve`, input));
}

// ─── biz settings + security settings ──────────────────────────────
export interface BizSetting {
  key: string;
  value: string;
  description?: string;
  updated_at: string;
}
export async function listBizSettings(): Promise<BizSetting[]> {
  const res = await apiClient.get<PaginatedEnvelope<BizSetting>>("/api/admin/biz-settings");
  return res.data.data ?? [];
}
export async function updateBizSetting(key: string, value: string): Promise<BizSetting> {
  return unwrap(apiClient.put<ApiEnvelope<BizSetting>>(`/api/admin/biz-settings/${key}`, { value }));
}

export interface SecuritySettings {
  ip_allowlist: string[];
  step_up_ttl_seconds: number;
  password_policy: string;
  watermark_enabled: boolean;
  session_max_seconds: number;
}
export async function getSecuritySettings(): Promise<SecuritySettings> {
  return unwrap(apiClient.get<ApiEnvelope<SecuritySettings>>("/api/admin/security"));
}
export async function updateSecuritySettings(input: Partial<SecuritySettings>): Promise<SecuritySettings> {
  return unwrap(apiClient.put<ApiEnvelope<SecuritySettings>>("/api/admin/security", input));
}

// ─── saga force-resolve（dual-control） ────────────────────────────
export interface ForceResolveInput {
  approver_token: string;
  approver_ip: string;
  outcome: "resolved" | "compensated";
  reason?: string;
}
export async function forceResolveSaga(sagaId: string, input: ForceResolveInput): Promise<{ status: string }> {
  return unwrap(
    apiClient.post<ApiEnvelope<{ status: string }>>(`/api/admin/saga/${sagaId}/force-resolve`, input),
  );
}
export interface SagaCandidate {
  saga_id: string;
  state: string;
  age_seconds: number;
  cooldown_until?: string; // 30min cooldown
  initiator_ip?: string;
}
export async function listEscalatedSagas(): Promise<SagaCandidate[]> {
  const res = await apiClient.get<PaginatedEnvelope<SagaCandidate>>("/api/admin/saga/escalated");
  return res.data.data ?? [];
}

// ─── staff CRUD ────────────────────────────────────────────────────
export interface Staff {
  id: number;
  username: string;
  email_masked: string;
  role: StaffRole;
  status: "active" | "disabled";
  last_login_at?: string;
  mfa_enrolled: boolean;
}
export async function listStaff(): Promise<Staff[]> {
  const res = await apiClient.get<PaginatedEnvelope<Staff>>("/api/admin/staff");
  return res.data.data ?? [];
}
export async function getStaff(id: number): Promise<Staff> {
  return unwrap(apiClient.get<ApiEnvelope<Staff>>(`/api/admin/staff/${id}`));
}
export async function createStaff(input: { username: string; password_hash: string; role: StaffRole; email?: string }): Promise<Staff> {
  return unwrap(apiClient.post<ApiEnvelope<Staff>>("/api/admin/staff", input));
}
export async function updateStaff(id: number, input: Partial<Staff>): Promise<Staff> {
  return unwrap(apiClient.put<ApiEnvelope<Staff>>(`/api/admin/staff/${id}`, input));
}
export async function disableStaff(id: number): Promise<Staff> {
  return unwrap(apiClient.post<ApiEnvelope<Staff>>(`/api/admin/staff/${id}/disable`, {}));
}

export { genUUID };
