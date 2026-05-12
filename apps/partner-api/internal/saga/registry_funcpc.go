// internal/saga/registry_funcpc.go — func pointer-equality helper.
//
// 拆出独立文件以便测试时 mock；reflect 调用在 hot path 之外，对 init 期 dup-register 检测足够。
package saga

import "reflect"

// funcPC 返回 fn 底层指针；同函数 → 同 PC。
func funcPC(fn StepFunc) uintptr {
	if fn == nil {
		return 0
	}
	return reflect.ValueOf(fn).Pointer()
}
