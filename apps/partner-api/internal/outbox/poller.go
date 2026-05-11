// internal/outbox/poller.go — outbox 30d purge cron 入口（PRD §6 outbox.purge v0.2.2）.
//
// 30d 后物理 DELETE consume_log_outbox 已 consumed 行；单批 ≤ 10000；
// 连续 3 日跑空告警；连续 3 日残留 consumed_at < NOW()-31d 告警.
//
// leader 选举与 settlement-runner 同款（pkg/leader.SETNX）；ops 接 @platform-ops.
package outbox

import (
	"context"

	"gorm.io/gorm"
)

// PurgeService 30d 物理删除 cron.
type PurgeService struct {
	logDB *gorm.DB
}

// NewPurgeService 构造.
func NewPurgeService(logDB *gorm.DB) *PurgeService {
	return &PurgeService{logDB: logDB}
}

// PurgeOnce 单次执行；W1a 落具体 SQL（DELETE FROM consume_log_outbox WHERE status='consumed' AND consumed_at < NOW()-30d LIMIT 10000）.
//
// 当前为占位接口；保留方法签名以便 cron 注册不需要变更.
func (p *PurgeService) PurgeOnce(_ context.Context) (deleted int64, err error) {
	// TODO(W1a): real SQL；本 W1b 仅声明合约.
	return 0, nil
}
