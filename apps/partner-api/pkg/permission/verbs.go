// Package permission 落地 PRD §3.4 22-verbs × 6-actor 权限矩阵（W0 scaffold）.
//
// W1a 实现：
//   - matrix map[Verb]map[ActorType]bool
//   - dual-control flag: VerbSagaForceResolve / VerbConfigWriteSecurity
//   - elevated flag (step-up MFA window ≤ 15 min)
//   - CI gate `permission-matrix-check`：每个 router.Handle 必引用 permission.Verb（grep + AST）
package permission

// Verb 权限动词 enum（backend §7.4）.
type Verb string

const (
	VerbCustomerReadSelf      Verb = "customer.read_self"
	VerbCustomerCreate        Verb = "customer.create"
	VerbPartnerReadSelf       Verb = "partner.read_self"
	VerbPartnerCustomerList   Verb = "partner.customer.list"
	VerbPartnerCustomerAlloc  Verb = "partner.customer.allocate_quota"
	VerbStaffPartnerApprove   Verb = "staff.partner.approve"
	VerbStaffWalletAdjust     Verb = "staff.wallet.adjust"
	VerbStaffKYCApprove       Verb = "staff.kyc.approve"
	VerbStaffSettlementRun    Verb = "staff.settlement.run"
	VerbSagaForceResolve      Verb = "saga.force_resolve"        // dual-control + step-up
	VerbConfigWriteTrivial    Verb = "system.config_write.trivial"
	VerbConfigWriteSecurity   Verb = "system.config_write.security" // dual-control
	VerbAuditRead             Verb = "audit.read"
	VerbAuditReadElevated     Verb = "audit.read.elevated"
	// W1a per PRD §3.4 — fill remaining 22 verbs.
)

// Spec 单条 verb 元信息.
type Spec struct {
	Allowed       []ActorType
	Elevated      bool // 需 step-up MFA
	DualControl   bool // 需 X-Second-Approver-Token
}

// ActorType actor 类型枚举（与 JWT.actor_type 对齐）.
type ActorType string

const (
	ActorPartner  ActorType = "partner"
	ActorCustomer ActorType = "customer"
	ActorStaff    ActorType = "staff"
)

// Matrix W1a 实现完整 22 × 6 矩阵.
//
// 当前最小集合用于 build/test 通过；后续按 PRD §3.4 表格扩展。
var Matrix = map[Verb]Spec{
	VerbCustomerReadSelf:     {Allowed: []ActorType{ActorCustomer}},
	VerbPartnerReadSelf:      {Allowed: []ActorType{ActorPartner}},
	VerbPartnerCustomerAlloc: {Allowed: []ActorType{ActorPartner}},
	VerbSagaForceResolve:     {Allowed: []ActorType{ActorStaff}, Elevated: true, DualControl: true},
	VerbConfigWriteSecurity:  {Allowed: []ActorType{ActorStaff}, Elevated: true, DualControl: true},
	VerbConfigWriteTrivial:   {Allowed: []ActorType{ActorStaff}},
	VerbAuditReadElevated:    {Allowed: []ActorType{ActorStaff}, Elevated: true},
}
