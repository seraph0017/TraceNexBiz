// internal/saga/sweep_loop.go — retry worker driver (Fix-B' part 2 CRIT-B2).
//
// 业务 main / cmd/worker 包用 RunSweepLoop 启动 retry sweep goroutine。
// ticker 30s（saga.RetrySweepInterval）触发；ctx.Done() → 优雅退出。
//
// 多副本：分片由调用方在外层 wrap（per-replica hash(saga_id) % N）；本 loop 不感知。
package saga

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

// RunSweepLoop 阻塞循环执行 orch.Sweep；ctx 取消即返回。
//
// batch <= 0 走默认 100。interval <= 0 走 RetrySweepInterval。
//
// 错误：单次 sweep 失败仅 log；不抛出（避免单个 transient DB error 拖死整个 worker）。
func RunSweepLoop(ctx context.Context, orch Orchestrator, batch int, interval time.Duration) {
	if interval <= 0 {
		interval = RetrySweepInterval
	}
	if batch <= 0 {
		batch = 100
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			res, err := orch.Sweep(ctx, batch)
			if err != nil {
				log.Warn().Err(err).Msg("saga.Sweep failed")
				continue
			}
			if res.Scanned == 0 {
				continue
			}
			log.Info().
				Int("scanned", res.Scanned).
				Int("retried", res.Retried).
				Int("escalated", res.Escalated).
				Int("skipped", res.Skipped).
				Msg("saga.Sweep tick")
		}
	}
}
