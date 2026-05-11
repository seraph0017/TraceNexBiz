// Package service is the partner-api business logic layer (W0 scaffold; signatures + TODO only).
//
// Layering rules (backend §2.1):
//   1. Service input/output uses DTO structs; do not expose domain.* directly to handler.
//   2. Service runs every state change inside bizDB.Transaction closure;
//      idempotency_record INSERT happens in the SAME transaction (backend §8.1 v0.2.2 / ADR-003).
//   3. All errors get wrapped with fmt.Errorf("%w", err) — never swallow.
//   4. PII fields are KMS-Encrypted at the service boundary, never inside repository.
//
// File organization (W1a/W1b/W1c implement):
//   - service/partner.go          partner lifecycle (M3-*)
//   - service/customer.go         customer (M2-*)
//   - service/wallet.go           Allocate saga (backend §5.3)
//   - service/pricing.go          markup validation (backend §5.4)
//   - service/revenue.go          outbox-driven (integration §3.3)
//   - service/settlement.go       monthly batch (backend §5.5)
//   - service/kyc.go              KMS-encrypted submission flow (backend §5.6)
//   - service/payment.go          topup saga (Phase 2A)
//   - service/invoice.go          fapiao + red-flush (Phase 2B)
//   - service/audit.go            hash chain sealer
//   - service/auth.go             login / refresh / mfa / step-up / password reset
//   - service/notification.go     outbox-driven dispatcher
//   - service/ticket.go           support ticket SLA
//   - service/pipl.go             user rights / complaint / erase saga (M13)
//   - service/content_safety.go   12377 reporter (Phase 2A)
package service

// Marker — keeps the package compilable while subfiles are being filled in by W1.
