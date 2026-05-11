// Package pubsub 封装异步消息总线（Aliyun MNS / Redis Pub/Sub）。
//
// 引用：integration §1.6 影子模式 + integration §3 option_update / user_update。
//
// W0 ：仅定义接口；W1d 接实际 MNS / Redis Pub/Sub。
package pubsub

import (
	"context"
	"errors"
)

// Publisher 发布事件到 topic。
type Publisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// Subscriber 订阅 topic；handler 在独立 goroutine 中接收。
type Subscriber interface {
	Subscribe(ctx context.Context, topic string, handler MessageHandler) error
	Close() error
}

// MessageHandler ：返回 error 表示处理失败，pubsub 框架决定重投递策略。
type MessageHandler func(ctx context.Context, payload []byte) error

// ErrNotImplemented W0 stub 错误。
var ErrNotImplemented = errors.New("pubsub: stub not implemented; W1d agent to wire MNS / Redis Pub/Sub")

// Stub W0 实现：no-op。
type Stub struct{}

func NewStub() *Stub { return &Stub{} }

func (s *Stub) Publish(_ context.Context, _ string, _ []byte) error {
	// TODO(W1d): per integration §1.6 接 MNS（影子模式）+ §3 option_update / user_update Redis Pub/Sub
	return nil
}

func (s *Stub) Subscribe(_ context.Context, _ string, _ MessageHandler) error {
	return nil
}

func (s *Stub) Close() error { return nil }
