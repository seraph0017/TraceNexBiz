// Package invoice 发票（PRD §7.8 / Compliance HIGH-6 / MED-17）.
//
// 流程：
//
//	customer / partner POST /invoice  → INSERT invoice_application(status=applied)
//	staff (finance_admin) review       → reviewing → issuing
//	全电发票 API 出票成功                → issued + invoice_url + fapiao_serial
//	M8 红冲                              → red_flushing → red_flushed
//
// 不变量：
//   - 销售方主体（seller_entity_id + seller_tax_no）必须 service 层注入，不能由前端控制
//   - 税号：18 位统一社会信用代码（个人发票 title_type=1 无税号）
//   - archive_expires_at = issued_at + 10y（PRD §7.8 留存）
//   - 金额对账：sum(invoice_application.amount where issued) == sum(revenue_log.amount where billable)；
//     由 cron settlement.run 校验，service 层只保证写入正确性
package invoice

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ErrInvalidTaxNo 税号不合法.
var ErrInvalidTaxNo = errors.New("invoice: invalid tax number")

// ErrInvalidStateTransition 状态机非法迁移.
var ErrInvalidStateTransition = errors.New("invoice: invalid state transition")

// ErrSellerEntityRequired 销售方主体必须由 service 注入.
var ErrSellerEntityRequired = errors.New("invoice: seller entity required")

// ErrInvalidAmount 金额非法.
var ErrInvalidAmount = errors.New("invoice: invalid amount")

// ErrTitleNotFound 发票抬头不存在.
var ErrTitleNotFound = errors.New("invoice: title not found")

// ErrApplicationNotFound 申请不存在.
var ErrApplicationNotFound = errors.New("invoice: application not found")

// 18 位统一社会信用代码 + 历史 15 位税号兼容.
var taxNoRegex = regexp.MustCompile(`^([0-9A-HJ-NPQRTUWXY]{18}|[0-9]{15})$`)

// ValidateTaxNo 校验税号格式（统一社会信用代码 18 位 / 历史 15 位）.
func ValidateTaxNo(s string) error {
	s = strings.TrimSpace(strings.ToUpper(s))
	if !taxNoRegex.MatchString(s) {
		return ErrInvalidTaxNo
	}
	return nil
}

// 合法状态迁移图（key = "from->to"）.
var validTransitions = map[string]struct{}{
	"applied->reviewing":      {},
	"reviewing->issuing":      {},
	"reviewing->rejected":     {},
	"issuing->issued":         {},
	"issuing->rejected":       {},
	"issued->red_flushing":    {},
	"red_flushing->red_flushed": {},
}

func canTransition(from, to string) bool {
	_, ok := validTransitions[from+"->"+to]
	return ok
}

// Title 发票抬头（值对象；service 层只读）.
type Title struct {
	ID        int64
	OwnerType string
	OwnerID   int64
	TitleType int8 // 1=个人 2=企业
	Title     string
	TaxNumber string
	BankInfo  string
	IsDefault bool
}

// Application 发票申请实例（service 层值对象）.
type Application struct {
	ID                int64
	ApplicantType     string
	ApplicantID       int64
	TitleID           int64
	SellerEntityID    int64
	SellerTaxNo       string
	Amount            int64
	Period            string
	Status            string
	InvoiceURL        string
	FapiaoSerial      string
	MailAddress       string
	RedFlushRequestID *int64
	AppliedAt         time.Time
	IssuedAt          *time.Time
	ArchiveExpiresAt  time.Time
	Notes             string
	RejectReasonCode  string
	RejectReasonText  string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// RedFlushRequest 红冲请求.
type RedFlushRequest struct {
	ID                int64
	OriginalInvoiceID int64
	RedFapiaoSerial   string
	ReasonCode        string
	ReasonText        string
	Status            string
	RequestedBy       int64
	RequestedAt       time.Time
	ConfirmedAt       *time.Time
	CompletedAt       *time.Time
}

// SellerProfile 销售方主体配置（biz_setting 注入）.
type SellerProfile struct {
	EntityID int64
	TaxNo    string
	Name     string
}

// FapiaoGateway 全电发票网关（接口；M8 接百望 / 航信 / 腾讯电子发票 SDK）.
type FapiaoGateway interface {
	Issue(ctx context.Context, app *Application, title *Title) (invoiceURL, fapiaoSerial string, err error)
	RedFlush(ctx context.Context, original *Application, reason string) (redSerial string, err error)
}

// StubFapiaoGateway 本地 stub.
type StubFapiaoGateway struct {
	mu     sync.Mutex
	serial int64
}

// Issue stub 出票.
func (g *StubFapiaoGateway) Issue(ctx context.Context, app *Application, title *Title) (string, string, error) {
	if title == nil {
		return "", "", ErrTitleNotFound
	}
	if title.TitleType == 2 {
		if err := ValidateTaxNo(title.TaxNumber); err != nil {
			return "", "", err
		}
	}
	g.mu.Lock()
	g.serial++
	s := g.serial
	g.mu.Unlock()
	return fmt.Sprintf("stub://invoice/%d.pdf", s), fmt.Sprintf("FP%010d", s), nil
}

// RedFlush stub 红冲.
func (g *StubFapiaoGateway) RedFlush(ctx context.Context, original *Application, reason string) (string, error) {
	g.mu.Lock()
	g.serial++
	s := g.serial
	g.mu.Unlock()
	return fmt.Sprintf("RF%010d", s), nil
}

// Repo 持久化抽象.
type Repo interface {
	GetTitle(ctx context.Context, ownerType string, ownerID, titleID int64) (*Title, error)
	InsertApplication(ctx context.Context, app *Application) (int64, error)
	GetApplication(ctx context.Context, id int64) (*Application, error)
	UpdateApplication(ctx context.Context, id int64, updater func(Application) Application) (*Application, error)
	InsertRedFlush(ctx context.Context, rf *RedFlushRequest) (int64, error)
	UpdateRedFlush(ctx context.Context, id int64, updater func(RedFlushRequest) RedFlushRequest) (*RedFlushRequest, error)
}

// Service 发票业务.
type Service struct {
	repo    Repo
	gateway FapiaoGateway
	seller  SellerProfile
	clock   func() time.Time
}

// NewService 构造.
func NewService(repo Repo, gw FapiaoGateway, seller SellerProfile) *Service {
	return &Service{repo: repo, gateway: gw, seller: seller, clock: time.Now}
}

// ApplyInput 用户提交发票申请.
type ApplyInput struct {
	ApplicantType string
	ApplicantID   int64
	TitleID       int64
	Amount        int64
	Period        string
	MailAddress   string
	Notes         string
}

// Apply 创建 invoice_application（status=applied）.
func (s *Service) Apply(ctx context.Context, in ApplyInput) (*Application, error) {
	if in.Amount <= 0 {
		return nil, ErrInvalidAmount
	}
	if s.seller.EntityID == 0 || s.seller.TaxNo == "" {
		return nil, ErrSellerEntityRequired
	}
	title, err := s.repo.GetTitle(ctx, in.ApplicantType, in.ApplicantID, in.TitleID)
	if err != nil {
		return nil, fmt.Errorf("invoice: get title: %w", err)
	}
	if title.TitleType == 2 {
		if err := ValidateTaxNo(title.TaxNumber); err != nil {
			return nil, err
		}
	}
	now := s.clock()
	app := &Application{
		ApplicantType:    in.ApplicantType,
		ApplicantID:      in.ApplicantID,
		TitleID:          in.TitleID,
		SellerEntityID:   s.seller.EntityID,
		SellerTaxNo:      s.seller.TaxNo,
		Amount:           in.Amount,
		Period:           in.Period,
		Status:           "applied",
		MailAddress:      in.MailAddress,
		Notes:            in.Notes,
		AppliedAt:        now,
		ArchiveExpiresAt: now.AddDate(10, 0, 0),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	id, err := s.repo.InsertApplication(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("invoice: insert: %w", err)
	}
	app.ID = id
	return app, nil
}

// Review (finance_admin) 审核：applied → reviewing → issuing/rejected.
func (s *Service) Review(ctx context.Context, id int64, approve bool, rejectCode, rejectText string) (*Application, error) {
	app, err := s.repo.GetApplication(ctx, id)
	if err != nil {
		return nil, err
	}
	if app.Status == "applied" {
		if _, err := s.repo.UpdateApplication(ctx, id, func(a Application) Application {
			a.Status = "reviewing"
			a.UpdatedAt = s.clock()
			return a
		}); err != nil {
			return nil, err
		}
		app.Status = "reviewing"
	}
	if !canTransition(app.Status, ternary(approve, "issuing", "rejected")) {
		return nil, ErrInvalidStateTransition
	}
	return s.repo.UpdateApplication(ctx, id, func(a Application) Application {
		if approve {
			a.Status = "issuing"
		} else {
			a.Status = "rejected"
			a.RejectReasonCode = rejectCode
			a.RejectReasonText = rejectText
		}
		a.UpdatedAt = s.clock()
		return a
	})
}

// Issue 调全电发票 API 出票，issuing → issued.
func (s *Service) Issue(ctx context.Context, id int64) (*Application, error) {
	app, err := s.repo.GetApplication(ctx, id)
	if err != nil {
		return nil, err
	}
	if app.Status != "issuing" {
		return nil, ErrInvalidStateTransition
	}
	title, err := s.repo.GetTitle(ctx, app.ApplicantType, app.ApplicantID, app.TitleID)
	if err != nil {
		return nil, err
	}
	url, serial, err := s.gateway.Issue(ctx, app, title)
	if err != nil {
		return nil, fmt.Errorf("invoice: gateway issue: %w", err)
	}
	now := s.clock()
	return s.repo.UpdateApplication(ctx, id, func(a Application) Application {
		a.Status = "issued"
		a.InvoiceURL = url
		a.FapiaoSerial = serial
		a.IssuedAt = &now
		a.UpdatedAt = now
		return a
	})
}

// RedFlush M8 红冲：issued → red_flushing → red_flushed.
//
// 严格约束：
//   - 仅可对 issued 状态发起
//   - reason_code 必填
//   - 红字发票号串通 fapiao 网关取（gateway.RedFlush）
//   - 一次性 — original_invoice_id 已有未完成红冲申请时拒绝
func (s *Service) RedFlush(ctx context.Context, originalID int64, reasonCode, reasonText string, requestedBy int64) (*Application, *RedFlushRequest, error) {
	if reasonCode == "" {
		return nil, nil, errors.New("invoice: reason_code required")
	}
	app, err := s.repo.GetApplication(ctx, originalID)
	if err != nil {
		return nil, nil, err
	}
	if app.Status != "issued" {
		return nil, nil, ErrInvalidStateTransition
	}
	now := s.clock()
	rf := &RedFlushRequest{
		OriginalInvoiceID: originalID,
		ReasonCode:        reasonCode,
		ReasonText:        reasonText,
		Status:            "pending",
		RequestedBy:       requestedBy,
		RequestedAt:       now,
	}
	rfID, err := s.repo.InsertRedFlush(ctx, rf)
	if err != nil {
		return nil, nil, err
	}
	rf.ID = rfID
	if _, err := s.repo.UpdateApplication(ctx, originalID, func(a Application) Application {
		a.Status = "red_flushing"
		a.RedFlushRequestID = &rfID
		a.UpdatedAt = now
		return a
	}); err != nil {
		return nil, nil, err
	}
	redSerial, err := s.gateway.RedFlush(ctx, app, reasonText)
	if err != nil {
		return nil, nil, fmt.Errorf("invoice: gateway red_flush: %w", err)
	}
	rfFinal, err := s.repo.UpdateRedFlush(ctx, rfID, func(r RedFlushRequest) RedFlushRequest {
		r.RedFapiaoSerial = redSerial
		r.Status = "completed"
		t := s.clock()
		r.CompletedAt = &t
		return r
	})
	if err != nil {
		return nil, nil, err
	}
	final, err := s.repo.UpdateApplication(ctx, originalID, func(a Application) Application {
		a.Status = "red_flushed"
		a.UpdatedAt = s.clock()
		return a
	})
	if err != nil {
		return nil, nil, err
	}
	return final, rfFinal, nil
}

// MemoryRepo 内存实现（测试用）.
type MemoryRepo struct {
	mu        sync.Mutex
	titles    map[int64]*Title
	apps      map[int64]*Application
	rfs       map[int64]*RedFlushRequest
	nextApp   int64
	nextRF    int64
	idemSeen  sync.Map
	serialSeq atomic.Int64
}

// NewMemoryRepo 构造.
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{
		titles: make(map[int64]*Title),
		apps:   make(map[int64]*Application),
		rfs:    make(map[int64]*RedFlushRequest),
	}
}

// PutTitle 测试 fixture.
func (r *MemoryRepo) PutTitle(t *Title) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *t
	r.titles[t.ID] = &cp
}

// GetTitle scope by owner.
func (r *MemoryRepo) GetTitle(ctx context.Context, ownerType string, ownerID, titleID int64) (*Title, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.titles[titleID]
	if !ok || t.OwnerType != ownerType || t.OwnerID != ownerID {
		return nil, ErrTitleNotFound
	}
	cp := *t
	return &cp, nil
}

// InsertApplication.
func (r *MemoryRepo) InsertApplication(ctx context.Context, app *Application) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextApp++
	app.ID = r.nextApp
	cp := *app
	r.apps[app.ID] = &cp
	return app.ID, nil
}

// GetApplication.
func (r *MemoryRepo) GetApplication(ctx context.Context, id int64) (*Application, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.apps[id]
	if !ok {
		return nil, ErrApplicationNotFound
	}
	cp := *a
	return &cp, nil
}

// UpdateApplication immutable updater.
func (r *MemoryRepo) UpdateApplication(ctx context.Context, id int64, updater func(Application) Application) (*Application, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.apps[id]
	if !ok {
		return nil, ErrApplicationNotFound
	}
	next := updater(*a)
	r.apps[id] = &next
	cp := next
	return &cp, nil
}

// InsertRedFlush.
func (r *MemoryRepo) InsertRedFlush(ctx context.Context, rf *RedFlushRequest) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextRF++
	rf.ID = r.nextRF
	cp := *rf
	r.rfs[rf.ID] = &cp
	return rf.ID, nil
}

// UpdateRedFlush immutable updater.
func (r *MemoryRepo) UpdateRedFlush(ctx context.Context, id int64, updater func(RedFlushRequest) RedFlushRequest) (*RedFlushRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rf, ok := r.rfs[id]
	if !ok {
		return nil, errors.New("invoice: red_flush not found")
	}
	next := updater(*rf)
	r.rfs[id] = &next
	cp := next
	return &cp, nil
}

func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}
