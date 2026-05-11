// internal/saga/force_resolve.go — dual-control force-resolve 决策逻辑.
//
// 业务规则（per backend §4.5 escalation + Q-A dual-control）：
//   1. 必须两位不同 staff（actor != approver）
//   2. cooldown：同一 saga 上次决策距今 ≥ 30min
//   3. 双方 IP 不能在同一 /24 网段（防同一办公室合谋）
//   4. 一次性 token：admin 后台先签发 token，30min TTL，单次消费
//   5. 必须显式 target（committed / compensated / released_pessimistic）
//   6. 写 audit_log（dual-control SEC CRIT-5），由 caller 落
//
// 本文件只做纯决策（输入 → ForceResolveDecision）；落库 + audit 由 W1c handler 调用。
package saga

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// ForceResolveRequest 调用方组装的入参（IP / 时间戳来自 staff session + token store）。
type ForceResolveRequest struct {
	SagaID         string
	StepName       string
	Target         Status
	ActorID        int64  // 提交人 staff_id
	ActorIP        string // 提交人 IP
	ApproverID     int64  // 审核人 staff_id
	ApproverIP     string // 审核人 IP
	TokenIssuedAt  time.Time
	TokenConsumed  bool // 调用方先 SETNX 标记已消费；本函数仅校验
	LastResolvedAt *time.Time
	Now            time.Time
	Reason         string // 必填，进 audit_log.diff_redacted
}

// 决策错误（service 层映射 BIZ_* code）。
var (
	ErrSameActor          = errors.New("saga: dual-control requires different actor and approver")
	ErrCooldownViolated   = errors.New("saga: dual-control cooldown not elapsed")
	ErrIPSameSubnet       = errors.New("saga: actor and approver in same /24 subnet")
	ErrTokenExpired       = errors.New("saga: dual-control token expired")
	ErrTokenAlreadyUsed   = errors.New("saga: dual-control token already consumed")
	ErrInvalidTarget      = errors.New("saga: invalid force-resolve target")
	ErrEmptyReason        = errors.New("saga: force-resolve reason required")
	ErrInvalidIP          = errors.New("saga: invalid IP address")
)

// 默认窗口（与 backend §4.5 + integration §17.5 对齐）。
const (
	ForceResolveTokenTTL = 30 * time.Minute
)

// ValidateForceResolve 纯函数：输入 → error（nil = 通过）.
//
// 调用方在通过后才执行 repo.ForceResolve(...) + audit_log.Append(...)。
func ValidateForceResolve(r ForceResolveRequest) error {
	if !IsValidUUIDv7(r.SagaID) {
		return fmt.Errorf("saga: saga_id must be UUIDv7: %w", ErrInvalidTarget)
	}
	if r.StepName == "" {
		return fmt.Errorf("saga: step_name required: %w", ErrInvalidTarget)
	}
	if !isValidForceTarget(r.Target) {
		return fmt.Errorf("saga: target=%s not allowed: %w", r.Target, ErrInvalidTarget)
	}
	if r.ActorID == 0 || r.ApproverID == 0 {
		return ErrSameActor
	}
	if r.ActorID == r.ApproverID {
		return ErrSameActor
	}
	if strings.TrimSpace(r.Reason) == "" {
		return ErrEmptyReason
	}
	if r.TokenConsumed {
		return ErrTokenAlreadyUsed
	}
	if r.Now.Sub(r.TokenIssuedAt) > ForceResolveTokenTTL {
		return ErrTokenExpired
	}
	if r.LastResolvedAt != nil {
		if r.Now.Sub(*r.LastResolvedAt) < ForceResolveCooldown {
			return ErrCooldownViolated
		}
	}
	if err := assertDifferentSubnet(r.ActorIP, r.ApproverIP); err != nil {
		return err
	}
	return nil
}

// isValidForceTarget 仅允许收口态（不能 force 到 in_progress / pending）。
func isValidForceTarget(t Status) bool {
	switch t {
	case StatusCommitted, StatusCompensated, StatusReleasedPessimistic:
		return true
	default:
		return false
	}
}

// assertDifferentSubnet 检验 actor/approver IP 不在同一 /24（IPv4）或 /48（IPv6）子网。
func assertDifferentSubnet(a, b string) error {
	an, err := parseIPv4OrV6(a)
	if err != nil {
		return fmt.Errorf("actor IP: %w", err)
	}
	bn, err := parseIPv4OrV6(b)
	if err != nil {
		return fmt.Errorf("approver IP: %w", err)
	}
	if subnetEqual(an, bn) {
		return ErrIPSameSubnet
	}
	return nil
}

func parseIPv4OrV6(s string) (net.IP, error) {
	ip := net.ParseIP(strings.TrimSpace(s))
	if ip == nil {
		return nil, ErrInvalidIP
	}
	return ip, nil
}

// subnetEqual 同 /24（IPv4）或 /48（IPv6）→ true.
func subnetEqual(a, b net.IP) bool {
	if v4a := a.To4(); v4a != nil {
		v4b := b.To4()
		if v4b == nil {
			return false
		}
		mask := net.CIDRMask(24, 32)
		return v4a.Mask(mask).Equal(v4b.Mask(mask))
	}
	mask := net.CIDRMask(48, 128)
	return a.Mask(mask).Equal(b.Mask(mask))
}

// nowUTC 抽出便于单测覆盖。
func nowUTC() time.Time { return time.Now().UTC() }
