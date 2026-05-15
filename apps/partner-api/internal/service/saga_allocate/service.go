// Package allocate 实现 M3-04 渠道商→客户分配额度 saga（integration §4.4）.
//
// 5 步：
//
//  1. wallet.deduct        partner_wallet.balance -= amount（乐观锁 Version）
//  2. wallet.hold          INSERT wallet_hold(saga_id, amount, status='held')
//  3. fyapi.topup          调 Fy-api /api/internal/user/topup（透传 Idempotency-Key=saga_id）
//  4. wallet.release       UPDATE wallet_hold SET status='committed' + log committed
//  5. revenue.log          INSERT partner_wallet_log(allocate_to_customer)
//
// 失败补偿对称（4→1 反向）；fyapi 5xx → ErrSagaUnknown，retry worker 探活；
// 探活 30 attempts 仍未知 → escalate 到 admin（场景 D / saga §4.5）.
//
// 本文件只暴露 Step 接口 + Service skeleton；具体 wallet / fyapi 调用由 W1a/W1b 注入.
package allocate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/saga"
)

// 错误.
var (
	ErrInsufficientBalance = errors.New("allocate: insufficient partner balance")
	ErrAmountInvalid       = errors.New("allocate: amount must be positive")
)

// Request M3-04 入参.
type Request struct {
	SagaID     string // 必须 UUIDv7（即 idempotency-key）
	PartnerID  int64
	CustomerID int64
	Amount     int64
	OperatorID int64
	TraceID    string
}

// Validate 入参校验.
func (r Request) Validate() error {
	if !saga.IsValidUUIDv7(r.SagaID) {
		return fmt.Errorf("allocate: saga_id must be UUIDv7")
	}
	if r.PartnerID == 0 || r.CustomerID == 0 || r.OperatorID == 0 {
		return fmt.Errorf("allocate: partner/customer/operator required")
	}
	if r.Amount <= 0 {
		return ErrAmountInvalid
	}
	return nil
}

// WalletPort 业务依赖（W1a/W1b 实现）.
type WalletPort interface {
	Deduct(ctx context.Context, partnerID, amount int64, sagaID string) error
	Hold(ctx context.Context, partnerID, amount int64, sagaID string) error
	CommitHold(ctx context.Context, sagaID string) error
	ReleaseHold(ctx context.Context, sagaID string) error
	Refund(ctx context.Context, partnerID, amount int64, sagaID string) error
	WriteLog(ctx context.Context, req Request) error
}

// FyAPIPort fyapi 调用抽象（避免循环 import）.
type FyAPIPort interface {
	TopupCustomer(ctx context.Context, customerFyUserID int64, amount int64, idemKey, traceID string) error
	RefundCustomer(ctx context.Context, customerFyUserID int64, amount int64, idemKey, traceID string) error
}

// CustomerLookup 客户查询（→ fy_user_id）.
type CustomerLookup interface {
	FyUserID(ctx context.Context, partnerID, customerID int64) (int64, error)
}

// Service 编排 5 步 saga.
type Service struct {
	orch       saga.Orchestrator
	wallet     WalletPort
	fyapi      FyAPIPort
	lookup     CustomerLookup
	idemDB     *gorm.DB
	idemInsert saga.IdempotencyInserter
}

// NewService 构造.
func NewService(o saga.Orchestrator, w WalletPort, f FyAPIPort, l CustomerLookup) *Service {
	s := &Service{orch: o, wallet: w, fyapi: f, lookup: l}
	activeService.Store(s)
	return s
}

// WithIdempotencyStore enables DB-backed idempotency for Allocate.
func (s *Service) WithIdempotencyStore(db *gorm.DB, inserter saga.IdempotencyInserter) *Service {
	s.idemDB = db
	s.idemInsert = inserter
	return s
}

// 步骤名常量（PII scrubber audit 用 string match，需稳定）.
const (
	StepDeduct  = "wallet.deduct"
	StepHold    = "wallet.hold"
	StepFyTopup = "fyapi.topup"
	StepCommit  = "wallet.commit_hold"
	StepLog     = "wallet.log"
)

type stepInput struct {
	Request  Request `json:"request"`
	FyUserID int64   `json:"fy_user_id,omitempty"`
}

var activeService atomic.Pointer[Service]

func init() {
	saga.RegisterStep(saga.KindWalletAllocate, StepDeduct, runDeduct)
	saga.RegisterStep(saga.KindWalletAllocate, StepHold, runHold)
	saga.RegisterStep(saga.KindWalletAllocate, StepFyTopup, runFyTopup)
	saga.RegisterStep(saga.KindWalletAllocate, StepCommit, runCommit)
	saga.RegisterStep(saga.KindWalletAllocate, StepLog, runLog)
}

// Run 推进 saga；committed step 自动跳过.
//
// 失败处理：
//   - StepDeduct/StepHold 失败 → 直接返回（DB 一致性自然 rollback）
//   - StepFyTopup 失败：补偿 ReleaseHold + Refund
//   - StepCommit 失败：补偿 ReleaseHold + Refund + Fy-api refund
func (s *Service) Run(ctx context.Context, req Request) error {
	return s.withIdempotency(ctx, req, func() error {
		return s.run(ctx, req)
	})
}

func (s *Service) run(ctx context.Context, req Request) error {
	if err := req.Validate(); err != nil {
		return err
	}
	sg, err := s.orch.NewSaga(req.SagaID, saga.KindWalletAllocate)
	if err != nil {
		return err
	}

	fyUserID, err := s.lookup.FyUserID(ctx, req.PartnerID, req.CustomerID)
	if err != nil {
		return fmt.Errorf("allocate: lookup customer: %w", err)
	}
	input, err := encodeStepInput(stepInput{Request: req, FyUserID: fyUserID})
	if err != nil {
		return err
	}

	if _, err := sg.RunWithInput(ctx, StepDeduct, input, runDeduct); err != nil {
		return err
	}
	if _, err := sg.RunWithInput(ctx, StepHold, input, runHold); err != nil {
		// hold 失败 → 把 deduct 退回
		_, compErr := sg.Compensate(ctx, StepDeduct, func(_ *gorm.DB) (any, error) {
			return nil, s.wallet.Refund(ctx, req.PartnerID, req.Amount, req.SagaID)
		})
		return wrapCompensationError("hold", err, compErr)
	}
	if _, err := sg.RunWithInput(ctx, StepFyTopup, input, runFyTopup); err != nil {
		// fyapi 失败 → 补偿：释放 hold + 退回 deduct
		_, releaseErr := sg.Compensate(ctx, StepHold, func(_ *gorm.DB) (any, error) {
			return nil, s.wallet.ReleaseHold(ctx, req.SagaID)
		})
		_, refundErr := sg.Compensate(ctx, StepDeduct, func(_ *gorm.DB) (any, error) {
			return nil, s.wallet.Refund(ctx, req.PartnerID, req.Amount, req.SagaID)
		})
		return wrapCompensationError("fyapi topup", err, errors.Join(releaseErr, refundErr))
	}
	if _, err := sg.RunWithInput(ctx, StepCommit, input, runCommit); err != nil {
		_, fyErr := sg.Compensate(ctx, StepFyTopup, func(_ *gorm.DB) (any, error) {
			return nil, s.fyapi.RefundCustomer(ctx, fyUserID, req.Amount, req.SagaID, req.TraceID)
		})
		_, releaseErr := sg.Compensate(ctx, StepHold, func(_ *gorm.DB) (any, error) {
			return nil, s.wallet.ReleaseHold(ctx, req.SagaID)
		})
		_, refundErr := sg.Compensate(ctx, StepDeduct, func(_ *gorm.DB) (any, error) {
			return nil, s.wallet.Refund(ctx, req.PartnerID, req.Amount, req.SagaID)
		})
		return wrapCompensationError("commit hold", err, errors.Join(fyErr, releaseErr, refundErr))
	}
	if _, err := sg.RunWithInput(ctx, StepLog, input, runLog); err != nil {
		return err
	}
	return nil
}

func (s *Service) withIdempotency(ctx context.Context, req Request, fn func() error) error {
	if s.idemDB == nil || s.idemInsert == nil || req.SagaID == "" {
		return fn()
	}
	rec := saga.NewIdempotencyRecord("partner", req.PartnerID, req.SagaID, "POST /partner/wallet/allocate", req.TraceID, `{"success":true}`, 200, time.Now().UTC())
	return saga.WithIdempotency(ctx, s.idemDB, s.idemInsert, rec, func(_ *gorm.DB) error {
		return fn()
	})
}

func runDeduct(ctx context.Context, _ *gorm.DB, input []byte) (any, error) {
	s, in, err := decodeRegisteredStep(input)
	if err != nil {
		return nil, err
	}
	return nil, s.wallet.Deduct(ctx, in.Request.PartnerID, in.Request.Amount, in.Request.SagaID)
}

func runHold(ctx context.Context, _ *gorm.DB, input []byte) (any, error) {
	s, in, err := decodeRegisteredStep(input)
	if err != nil {
		return nil, err
	}
	return nil, s.wallet.Hold(ctx, in.Request.PartnerID, in.Request.Amount, in.Request.SagaID)
}

func runFyTopup(ctx context.Context, _ *gorm.DB, input []byte) (any, error) {
	s, in, err := decodeRegisteredStep(input)
	if err != nil {
		return nil, err
	}
	return nil, s.fyapi.TopupCustomer(ctx, in.FyUserID, in.Request.Amount, in.Request.SagaID, in.Request.TraceID)
}

func runCommit(ctx context.Context, _ *gorm.DB, input []byte) (any, error) {
	s, in, err := decodeRegisteredStep(input)
	if err != nil {
		return nil, err
	}
	return nil, s.wallet.CommitHold(ctx, in.Request.SagaID)
}

func runLog(ctx context.Context, _ *gorm.DB, input []byte) (any, error) {
	s, in, err := decodeRegisteredStep(input)
	if err != nil {
		return nil, err
	}
	return nil, s.wallet.WriteLog(ctx, in.Request)
}

func encodeStepInput(in stepInput) ([]byte, error) {
	b, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("allocate: encode step input: %w", err)
	}
	return b, nil
}

func decodeRegisteredStep(input []byte) (*Service, stepInput, error) {
	s := activeService.Load()
	if s == nil {
		return nil, stepInput{}, fmt.Errorf("allocate: registered step executor unavailable")
	}
	var in stepInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, stepInput{}, fmt.Errorf("allocate: decode step input: %w", err)
	}
	if err := in.Request.Validate(); err != nil {
		return nil, stepInput{}, err
	}
	return s, in, nil
}

func wrapCompensationError(stage string, cause, compErr error) error {
	if compErr == nil {
		return cause
	}
	return fmt.Errorf("allocate: %s compensation failed: %w", stage, errors.Join(cause, compErr))
}
