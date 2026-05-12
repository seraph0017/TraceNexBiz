// internal/saga/registry.go — saga step function registry (Fix-B' part 2 CRIT-B2).
//
// Background：Round-1 CRIT-B2 发现 Sweep 只 escalate / 不 re-run。本文件提供 Option A
// 决策：步骤函数全局注册（package init 期间 RegisterStep）；retry sweep 拿
// (Kind, StepName) 从 registry 查 fn → 调用方在第一次 Run 把 input bytes 持久化到
// saga_step.Payload；sweep re-decode 后 re-call。
//
// 选 Option A（registry）而非 Option B（每个 fn 包成 factory）：
//   - service 层 fn 只需在 init() 注册一次；写法跟普通闭包一致
//   - retry worker 无需知道每条 saga 的具体 closure；解耦
//   - 同名 step 不能跨 kind 复用（必须前缀 namespace，已是现状：topup.* / wallet.*）
//
// 调用顺序：
//
//	func init() { saga.RegisterStep(saga.KindCustomerTopup, "topup.fy", topupFy) }
//	...
//	sg.RunWithInput(ctx, "topup.fy", inputBytes, topupFy)   ← 业务层
//	orch.Sweep(ctx, 100)                                    ← retry worker；自动 lookup fn
//
// 永久错误：fn 返回 ErrPermanent（或 errors.Is(err, ErrPermanent)）→ Run 立即 escalate，
// 不计入 retry。
package saga

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"gorm.io/gorm"
)

// StepFunc 可重入步骤函数；input 是 RunWithInput 保存的字节串（约定 JSON）。
//
// 与 TxFn 的区别：携带 input，可在 retry sweep 中重放。fn 必须是确定性 idempotent：
// 给定相同 input，第 N 次执行业务效果等价于第 1 次（DB / 下游 idempotency-key 保证）。
type StepFunc func(ctx context.Context, tx *gorm.DB, input []byte) (any, error)

// ErrPermanent fn 返回此 sentinel（或 wrap）→ 不再 retry；立刻 escalate。
//
// 适用场景：上游确定性 4xx（如 customer 不存在）、业务规则冲突（金额超限）。
var ErrPermanent = errors.New("saga: permanent error, no retry")

type registryKey struct {
	kind Kind
	step string
}

var (
	stepRegistry   = make(map[registryKey]StepFunc)
	stepRegistryMu sync.RWMutex
)

// RegisterStep 注册一个 (kind, step) → fn 映射；duplicate 注册（同 fn pointer）幂等，
// 不同 fn 注册 panic（启动期错误立即暴露）。
//
// 用法（service 包 init）：
//
//	func init() {
//	    saga.RegisterStep(saga.KindCustomerTopup, "topup.fy", topupFy)
//	}
func RegisterStep(kind Kind, step string, fn StepFunc) {
	if kind == "" || step == "" || fn == nil {
		panic("saga.RegisterStep: kind/step/fn required")
	}
	k := registryKey{kind: kind, step: step}
	stepRegistryMu.Lock()
	defer stepRegistryMu.Unlock()
	if existing, ok := stepRegistry[k]; ok {
		// 同函数指针重注册视为幂等（dev hot-reload / 测试多次 init）。
		if sameFunc(existing, fn) {
			return
		}
		panic(fmt.Sprintf("saga.RegisterStep: duplicate (kind=%s, step=%s) with different fn", kind, step))
	}
	stepRegistry[k] = fn
}

// LookupStep 取已注册的 fn；找不到返 nil。retry sweep 用。
func LookupStep(kind Kind, step string) StepFunc {
	stepRegistryMu.RLock()
	defer stepRegistryMu.RUnlock()
	return stepRegistry[registryKey{kind: kind, step: step}]
}

// ResetRegistryForTest 仅测试使用；清空已注册步骤。
func ResetRegistryForTest() {
	stepRegistryMu.Lock()
	defer stepRegistryMu.Unlock()
	stepRegistry = make(map[registryKey]StepFunc)
}

// sameFunc 比较两个 StepFunc 是否指向同一底层函数。
//
// 注：Go 不允许 func 直接比较，但可以用 reflect.ValueOf(f).Pointer() 等价。
// 简化处理：保守地认为不同（dup-register-with-different-fn 必 panic）。
// 此处约定 init 期间不会用闭包注册同名步骤；只用函数字面量。
func sameFunc(a, b StepFunc) bool {
	return funcPC(a) == funcPC(b)
}
