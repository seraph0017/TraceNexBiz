// Package validator 提供 service 层入参校验骨架（W0 scaffold）.
//
// W1a/b 实现：与 frontend zod schema 共享 contract（packages/validators，PRD §16.4）.
//
// 设计：
//   - 每个 DTO 在 service/handler 间转换时调 validator.Struct(in)
//   - 复杂跨字段约束（valid_from < valid_to / markup ∈ [1.0, 5.0]）走 Custom rule
//   - go-playground/validator 已被 Fy-api / Gin 标准引入，复用即可.
package validator

// W1a：在此挂 init() 注册 customRules：
//   - markup_bound (1.0..5.0)
//   - period_overlap (DB roundtrip)
//   - chinese_idcard
//   - uscc (统一社会信用代码)
//   - bank_account_4last_visible
