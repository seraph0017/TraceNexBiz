// public endpoints —— storefront 不需鉴权即可调用的 5 个 path
//
// W1c 落 OpenAPI 后切 orval 自动生成；当前手写以便业务开发不阻塞。
// 路径前缀由 partner-api routes.go 决定（W1a HANDOFF §1.2 + W1c）：
//   public/auth/* / public/partner/apply / public/customer/register
// storefront 公共数据另增（W1c 暂未列入 admin.yaml，frontend §3.1 必须）：
//   GET /api/public/models           —— 模型白名单 + 价格 + 备案号
//   GET /api/public/biz_setting/footer  —— 9 个备案号
//   GET /api/public/legal/:doc       —— 法律 markdown
//   POST /api/public/kyc/presign     —— OSS direct upload
import { apiClient, unwrap } from "./client";

export interface PublicModel {
  id: string;
  display_name: string;
  vendor: string;
  context_window: number;
  /** 输入价格（元 / 千 token） */
  price_input_per_1k: string;
  /** 输出价格（元 / 千 token） */
  price_output_per_1k: string;
  /** 算法备案号（生成式 AI 模型必填） */
  algorithm_filing_no?: string;
  /** 是否上架（false 时 storefront 隐藏） */
  enabled: boolean;
  description?: string;
}

export interface PublicModelsResponse {
  models: PublicModel[];
  /** 当 ICP 经营许可证未拿到时 storefront 显示"招商内测" */
  icp_license_active: boolean;
}

export async function fetchPublicModels(): Promise<PublicModelsResponse> {
  return unwrap(apiClient.get<{ success: boolean; data: PublicModelsResponse | null; error: null }>("/api/public/models"));
}

export interface ComplianceFooterDTO {
  icp_record_no?: string;
  icp_license_no?: string;
  public_security_filing_no?: string;
  gen_ai_filing_no?: string;
  algorithm_filing_no?: string;
  deep_synthesis_filing_no?: string;
  dpo_email?: string;
  dpo_phone?: string;
  report_phone_12377_link?: string;
}

export async function fetchComplianceFooter(): Promise<ComplianceFooterDTO> {
  return unwrap(
    apiClient.get<{ success: boolean; data: ComplianceFooterDTO | null; error: null }>(
      "/api/public/biz_setting/footer",
    ),
  );
}

export interface LegalDoc {
  slug: string;
  title: string;
  /** markdown 文本 —— 渲染前必须 sanitize */
  body_markdown: string;
  /** 最近一次更新（用于"生效日期"） */
  updated_at: string;
  version: string;
}

export async function fetchLegalDoc(slug: string): Promise<LegalDoc> {
  return unwrap(
    apiClient.get<{ success: boolean; data: LegalDoc | null; error: null }>(
      `/api/public/legal/${encodeURIComponent(slug)}`,
    ),
  );
}

// ============== 招商申请 ==============

export type PartnerApplyType = "individual" | "enterprise";

export interface ConsentRecord {
  /** 服务端 consent_log.id —— 招商申请提交前必须先创建 */
  consent_id: number;
  /** 用户勾选时的版本号（PIPL §15.5 audit） */
  version: string;
}

export interface PartnerApplyRequest {
  type: PartnerApplyType;
  contact_name: string;
  contact_phone: string;
  contact_email: string;
  /** PIPL 必收 —— 单独同意 ID */
  consent_id: number;
  /** PRD §15.5 / Fix-C item 7：consent 文本版本号，后端 audit 校验 */
  consent_text_version?: string;
  /** Fy-api 用户 ID（注册或登录后获得；招商申请允许游客先填） */
  fy_user_id?: number;
  /** 公司信息（type=enterprise 必填） */
  company_name?: string;
  unified_social_credit_code?: string;
  /** 法人 / 申请人身份证（KMS 信封加密落库） */
  legal_person_id?: string;
  /** 业务规模 */
  expected_monthly_calls?: number;
  expected_use_case?: string;
  source_channel?: string;
  /** 税务身份枚举（Fix-C item 5） */
  tax_status?: "individual" | "sole_proprietor" | "partnership" | "llc" | "corp";
  /** 结算银行（Fix-C item 6：account 走 HMAC blind_index） */
  settlement_bank_name?: string;
  settlement_bank_account?: string;
  settlement_account_holder?: string;
}

export interface PartnerApplyResponse {
  id: number;
  status: "applied";
}

export async function submitPartnerApply(
  body: PartnerApplyRequest,
): Promise<PartnerApplyResponse> {
  return unwrap(
    apiClient.post<{ success: boolean; data: PartnerApplyResponse | null; error: null }>(
      "/api/public/partner/apply",
      body,
    ),
  );
}

// ============== Consent 落库 ==============

export interface ConsentSubmitRequest {
  /** consent template key（partner_apply / pipl_personal_info / marketing 等） */
  scope: string;
  version: string;
  /** 用户 IP / UA 由后端通过 request 自动收集 */
  granted: boolean;
}

export async function submitConsent(body: ConsentSubmitRequest): Promise<ConsentRecord> {
  return unwrap(
    apiClient.post<{ success: boolean; data: ConsentRecord | null; error: null }>(
      "/api/public/consent",
      body,
    ),
  );
}

// ============== KYC OSS presign（招商申请上传执照 / 身份证 / 法人面照） ==============

export type KycUploadKind = "id_front" | "id_back" | "business_license" | "legal_person_face";

export interface KycPresignRequest {
  kind: KycUploadKind;
  /** 浏览器侧 file.type；后端二次校验 magic byte */
  content_type: string;
  /** byte —— 后端按类型 cap（身份证 5MB / 营业执照 10MB） */
  size: number;
}

export interface KycPresignResponse {
  /** 已签名 PUT URL（5 分钟有效） */
  upload_url: string;
  /** 上传后下载链接（KYC 提交时填入对应字段） */
  object_url: string;
  /** 必须以 header 形式带上的额外字段（如 x-oss-meta-scope） */
  required_headers: Record<string, string>;
  expires_at: string;
}

export async function presignKycUpload(body: KycPresignRequest): Promise<KycPresignResponse> {
  return unwrap(
    apiClient.post<{ success: boolean; data: KycPresignResponse | null; error: null }>(
      "/api/public/kyc/presign",
      body,
    ),
  );
}
