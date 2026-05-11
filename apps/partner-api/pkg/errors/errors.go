// Package errors 落地 backend §11 错误码系统。
//
// 设计：
//   - AppError 含 Code / HTTP / MessageZh / MessageEn / TraceID / Cause / Details
//   - 启动 init 校验每个 Code 必有 zh / en 双语（缺失 panic）
//   - service / repository 显式 wrap：fmt.Errorf("ctx: %w", err)；handler 层 unwrap 取 AppError
//
// W0 scaffold：只给 enum + 基础类型；W1a 接 i18n 文案表 + handler 端响应组装。
package errors

import (
	"errors"
	"fmt"
	"net/http"
)

// Code 业务错误码（backend §11.1）。
type Code string

// 鉴权
const (
	CodeAuthJWTRevoked       Code = "BIZ_AUTH_JWT_REVOKED"
	CodeAuthStepUpRequired   Code = "BIZ_AUTH_STEP_UP_REQUIRED"
	CodePermForbidden        Code = "BIZ_PERM_FORBIDDEN"
	CodePermDualControlReq   Code = "BIZ_PERM_DUAL_CONTROL_REQUIRED"
	CodeRevocationStoreDown  Code = "BIZ_AUTH_REVOCATION_STORE_DOWN"
)

// 资源 / 校验
const (
	CodeResNotFound          Code = "BIZ_RES_NOT_FOUND" // 越权也走这里（PRD §16.3）
	CodeValidAmountRange     Code = "BIZ_VALID_AMOUNT_OUT_OF_RANGE"
)

// 幂等 / saga
const (
	CodeIdemKeyRequired         Code = "BIZ_IDEM_KEY_REQUIRED"
	CodeIdemReusedDifferentBody Code = "BIZ_IDEM_REUSED_DIFFERENT_BODY"
	CodeSagaOrphanedPending     Code = "BIZ_SAGA_ORPHANED_PENDING"
	CodeSagaStuckUnknown        Code = "BIZ_SAGA_STUCK_UNKNOWN"
	CodeSagaForceResolved       Code = "BIZ_SAGA_FORCE_RESOLVED"
)

// 钱包 / 定价
const (
	CodeWalletInsufficient   Code = "BIZ_WALLET_INSUFFICIENT_AVAILABLE"
	CodeWalletVersionMismatch Code = "BIZ_WALLET_VERSION_MISMATCH"
	CodePricingOverlapWindow Code = "BIZ_PRICING_OVERLAP_WINDOW"
	CodePricingMarkupBound   Code = "BIZ_PRICING_MARKUP_BOUND"
)

// Fy-api 上游
const (
	CodeFyAPI5xx        Code = "BIZ_FYAPI_5XX"
	CodeFyAPITimeout    Code = "BIZ_FYAPI_TIMEOUT"
	CodeFyAPIUserErased Code = "BIZ_FYAPI_USER_ERASED"
)

// 系统层（backend §11.1 5xxxx 段位）
const (
	CodeSysDBDown    Code = "SYS_DB_DOWN"
	CodeSysFyAPIDown Code = "SYS_FYAPI_DOWN"
	CodeSysKMSDown   Code = "SYS_KMS_DOWN"
	CodeSysPanic     Code = "SYS_PANIC"
)

// AppError partner-api 统一错误模型（per integration §8.1 envelope）.
type AppError struct {
	Code    Code
	HTTP    int
	Cause   error
	TraceID string
	Details map[string]any
}

// Error 实现 error 接口；不暴露 Cause 给 message_en（防 PII 泄漏，由 i18n 表静态文案）。
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %v", e.Code, e.Cause)
	}
	return fmt.Sprintf("[%s]", e.Code)
}

// Unwrap errors.Is/As 兼容.
func (e *AppError) Unwrap() error { return e.Cause }

// Wrap 把 cause 包装为 AppError；W1a 实现：根据 code 查表 -> http status / i18n。
func Wrap(cause error, code Code) *AppError {
	return &AppError{Code: code, HTTP: defaultHTTP(code), Cause: cause}
}

// defaultHTTP 默认 HTTP 映射；W1a 增补完整 code → http 表。
func defaultHTTP(code Code) int {
	switch code {
	case CodeResNotFound:
		return http.StatusNotFound
	case CodeAuthJWTRevoked, CodeAuthStepUpRequired:
		return http.StatusUnauthorized
	case CodePermForbidden, CodePermDualControlReq:
		return http.StatusForbidden
	case CodeIdemKeyRequired, CodeValidAmountRange, CodePricingMarkupBound:
		return http.StatusBadRequest
	case CodeIdemReusedDifferentBody:
		return http.StatusConflict
	case CodeSysDBDown, CodeSysKMSDown, CodeRevocationStoreDown:
		return http.StatusServiceUnavailable
	case CodeFyAPI5xx, CodeSysFyAPIDown:
		return http.StatusBadGateway
	case CodeSagaOrphanedPending, CodeSysPanic:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// AsAppError errors.As 简易封装。
func AsAppError(err error) (*AppError, bool) {
	var ae *AppError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}
