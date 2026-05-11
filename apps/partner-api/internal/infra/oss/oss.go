// Package oss 封装阿里云 OSS / S3 兼容操作。
//
// 引用：backend §9.4 + ADR-017 v0.2 + Security CRITICAL-4。
//
// 关键约束：
//   - PresignPut 必须强 MIME / size / TTL 限制；bucket policy 拒绝不匹配
//   - HEAD 二次校验 + magic-byte 检测；不信任 Content-Type
//   - presigned URL TTL ≤ 300s；max size ≤ 10MB（image only）
//
// W0：定义接口 + stub；W1d 接实际 SDK + magic-byte 校验 hook。
package oss

import (
	"context"
	"errors"
	"time"
)

// AllowedMime 是合法 MIME 白名单；CI AST scan 校验调用方传值必须是字面量。
var AllowedMime = []string{
	"image/jpeg",
	"image/png",
	"image/webp",
	"application/pdf",
}

// PresignRequest 强约束输入（per ADR-017 v0.2）。
type PresignRequest struct {
	Bucket      string
	Key         string
	AllowedMime []string // 必须是 AllowedMime 子集；service 层断言
	MaxBytes    int64    // ≤ 10MB
	TTL         time.Duration // ≤ 300s
}

// PresignResult 给前端用于直传。
type PresignResult struct {
	URL     string
	Headers map[string]string // Content-Type / Content-Length / x-oss-content-md5 等
	Method  string            // PUT
	Expires time.Time
}

// Service 是 OSS 操作入口。
type Service interface {
	PresignPut(ctx context.Context, req PresignRequest) (*PresignResult, error)
	PresignGet(ctx context.Context, bucket, key string, ttl time.Duration) (string, error)

	// HeadObject + magic-byte 二次校验 hook（per ADR-017 v0.2）。
	// 调用方在 KYC 提交时必须调用此方法验证用户已上传的对象。
	VerifyMagicBytes(ctx context.Context, bucket, key string, expectedMime string) error

	// 异步病毒扫触发（W1d：接 ClamAV sidecar 或阿里云内容安全文件扫）
	EnqueueVirusScan(ctx context.Context, bucket, key string) error
}

// ErrNotImplemented W0 stub 错误。
var ErrNotImplemented = errors.New("oss: stub not implemented; W1d agent to wire Aliyun OSS / LocalStack S3")

// Stub 是 W0 占位；本地 dev 用 LocalStack 时由 W1d 替换。
type Stub struct{}

func NewStub() Service { return &Stub{} }

func (s *Stub) PresignPut(_ context.Context, req PresignRequest) (*PresignResult, error) {
	// TODO(W1d): per backend §9.4 调 oss SDK Presign + 强 Content-Type / Content-Length / x-oss-content-md5 签入
	if req.MaxBytes <= 0 || req.MaxBytes > 10*1024*1024 {
		return nil, errors.New("oss: maxBytes must be in (0, 10MB]")
	}
	if req.TTL <= 0 || req.TTL > 5*time.Minute {
		return nil, errors.New("oss: ttl must be in (0, 5m]")
	}
	if len(req.AllowedMime) == 0 {
		return nil, errors.New("oss: allowedMime is empty")
	}
	return &PresignResult{
		URL:     "http://localhost:4566/" + req.Bucket + "/" + req.Key,
		Method:  "PUT",
		Expires: time.Now().Add(req.TTL),
		Headers: map[string]string{},
	}, nil
}

func (s *Stub) PresignGet(_ context.Context, bucket, key string, ttl time.Duration) (string, error) {
	_ = ttl
	return "http://localhost:4566/" + bucket + "/" + key, nil
}

func (s *Stub) VerifyMagicBytes(_ context.Context, _, _, _ string) error {
	// TODO(W1d): HEAD object + 读 first 8 bytes + 匹配 expectedMime 的 magic
	return nil
}

func (s *Stub) EnqueueVirusScan(_ context.Context, _, _ string) error {
	// TODO(W1d): 入 SQS / Pub/Sub
	return nil
}
