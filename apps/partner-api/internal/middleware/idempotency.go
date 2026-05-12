// Idempotency middleware（backend §8 / integration §1.2 / Round-1 P0 fail-closed）。
//
// 实现（Fix-B' part 2 CRIT-B3 后）：
//   - 仅 POST/PUT/PATCH 强制；GET/DELETE 跳过
//   - 必须带 Idempotency-Key header；缺失或非法 UUID → 400
//   - per-actor namespace key：idem:{actor_type}:{actor_id}:{key}
//   - Fast path：Redis SETNX 成功 → 运行 handler；成功后写 cache
//   - Replay：Redis 命中且为 JSON payload → 立即回放
//   - Slow path：Redis 命中但为 PENDING 或 corrupt → 查 DB（service 层 same-TX 写的
//     idempotency_record 是 source of truth），命中即回放
//   - Redis 不可用 → 503（fail-closed）；但若 DBStore 注入且能查到 → 仍可回放（DB-only mode）
//
// 注：service 层在业务 tx 内 InsertWithinTx → Redis 仅是 24h 缓存，不再是 source of truth。
// 即使 Redis 整段 outage，DB 仍能保证幂等。
package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// HeaderIdemKey state-changing endpoint 必带 idempotency-key header。
const HeaderIdemKey = "Idempotency-Key"

// HeaderIdemReplay replay 命中时的响应 marker（与 Fy-api 对齐）。
const HeaderIdemReplay = "X-Tnb-Idempotent-Replay"

// CtxKeyIdemKey / CtxKeyIdemReqHash / CtxKeyIdemActor 是 service 层取用的 ctx key.
const (
	CtxKeyIdemKey     = "idem_key"
	CtxKeyIdemReqHash = "idem_req_hash"
	CtxKeyIdemActor   = "idem_actor"
)

// IdempotencyDBLookup 抽象 DB 慢路径回放（Fix-B' part 2 CRIT-B3）。
//
// 当 Redis miss 或 corrupt 时，middleware 查 DB（service 层 same-TX 已写入 idempotency_record）。
// 返回 (nil, nil) 表示 not-found；error 表示 store 自身故障（fail-closed 503）。
//
// 实现注入示例：
//
//	store := mwidem.NewDBLookup(idemRepo)   // 自带 endpoint canonicalize
//	mw := middleware.IdempotencyWithDB(rdb, ttl, store)
type IdempotencyDBLookup interface {
	Find(ctx context.Context, actorType string, actorID int64, key, endpoint string) (*IdempotencyDBRecord, error)
}

// IdempotencyDBRecord 是 DB 回放使用的最小投影；不依赖 domain 包以免循环 import。
type IdempotencyDBRecord struct {
	ResponseStatus int
	ResponseBody   []byte // phase-1 plaintext；Fix-C KMS 上线后 middleware 增加 Decrypt 桩
}

// IdemRecord 存量记录（v0.2.2 兼容占位）。
type IdemRecord struct {
	Status         string
	RequestHash    string
	ResponseStatus int
	ResponseCipher []byte
	ResponseKeyID  string
	SagaID         string
}

// KMSDecryptor 抽象 envelope 加密 service（v0.2.2 兼容占位）。
type KMSDecryptor interface {
	Decrypt(cipher []byte, keyID string) ([]byte, error)
}

// uuidRe 宽松 UUID 校验。
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type idemPayload struct {
	Status int    `json:"s"`
	Body   []byte `json:"b"`
	TS     int64  `json:"t"`
}

// Idempotency 装配 SETNX-based 幂等 middleware（无 DB 回退；保留 backwards-compat）。
//
// ttl 推荐 24h（cfg.IdempotencyTTL）。
func Idempotency(rds *redis.Client, ttl time.Duration) gin.HandlerFunc {
	return IdempotencyWithDB(rds, ttl, nil)
}

// IdempotencyWithDB 装配 SETNX + DB 回退的幂等 middleware（Fix-B' part 2 CRIT-B3）.
//
// dbLookup 可为 nil（旧行为）；非 nil 时：
//   - Redis 命中 PENDING / corrupt → 查 DB
//   - Redis 不可用 → 先尝试 DB（DBStore 也失败才 503）
func IdempotencyWithDB(rds *redis.Client, ttl time.Duration, dbLookup IdempotencyDBLookup) gin.HandlerFunc {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
		default:
			c.Next()
			return
		}
		key := c.GetHeader(HeaderIdemKey)
		if key == "" || !uuidRe.MatchString(key) {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "idempotency_key_required"})
			return
		}
		atStr := c.GetString(CtxKeyActorType)
		aidNum, _ := c.Get(CtxKeyActorID)
		aid, _ := aidNum.(int64)
		if atStr == "" {
			atStr = "anon"
		}
		c.Set(CtxKeyIdemKey, key)
		c.Set(CtxKeyIdemActor, fmt.Sprintf("%s:%d", atStr, aid))

		endpoint := c.Request.Method + " " + c.FullPath()
		if c.FullPath() == "" {
			endpoint = c.Request.Method + " " + c.Request.URL.Path
		}

		// ---- Redis fast path ----
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		if rds == nil {
			// Redis 缺失 + 无 DB store → 503；有 DB store → 尝试 DB
			if dbLookup != nil {
				if r := tryReplayFromDB(c, ctx, dbLookup, atStr, aid, key, endpoint); r {
					return
				}
			}
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "idempotency_unavailable"})
			return
		}

		redisKey := fmt.Sprintf("idem:%s:%d:%s", atStr, aid, key)
		ok, err := rds.SetNX(ctx, redisKey, "PENDING", ttl).Result()
		if err != nil {
			// Redis 故障：若有 DB store，尝试回退（DB 是 source of truth）。
			if dbLookup != nil {
				if r := tryReplayFromDB(c, ctx, dbLookup, atStr, aid, key, endpoint); r {
					return
				}
			}
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "idempotency_unavailable"})
			return
		}

		if !ok {
			val, err := rds.Get(ctx, redisKey).Result()
			if err != nil {
				if dbLookup != nil {
					if r := tryReplayFromDB(c, ctx, dbLookup, atStr, aid, key, endpoint); r {
						return
					}
				}
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "idempotency_unavailable"})
				return
			}
			if val == "PENDING" || val == "" {
				// in-progress：另一并发请求正在跑。优先查 DB（已 commit 的话能回放），否则 202。
				if dbLookup != nil {
					if r := tryReplayFromDB(c, ctx, dbLookup, atStr, aid, key, endpoint); r {
						return
					}
				}
				c.AbortWithStatusJSON(http.StatusAccepted, gin.H{"status": "in_progress"})
				return
			}
			var p idemPayload
			if err := json.Unmarshal([]byte(val), &p); err != nil {
				if dbLookup != nil {
					if r := tryReplayFromDB(c, ctx, dbLookup, atStr, aid, key, endpoint); r {
						return
					}
				}
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "idempotency_corrupt"})
				return
			}
			c.Header(HeaderIdemReplay, "1")
			c.Data(p.Status, "application/json", p.Body)
			c.Abort()
			return
		}

		rec := &idemResponseRecorder{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
		c.Writer = rec
		c.Next()

		status := rec.Status()
		if status >= 200 && status < 300 {
			payload, _ := json.Marshal(idemPayload{Status: status, Body: rec.body.Bytes(), TS: time.Now().Unix()})
			_ = rds.Set(ctx, redisKey, payload, ttl).Err()
		} else {
			_ = rds.Del(ctx, redisKey).Err()
		}
	}
}

// tryReplayFromDB 尝试从 DB 取出已 commit 的 idempotency_record 并回放。
// 命中 → 写响应 + Abort + 返回 true；miss / 故障 → 返回 false（caller 决定 503/202/原样跑）。
func tryReplayFromDB(c *gin.Context, ctx context.Context, store IdempotencyDBLookup,
	actorType string, actorID int64, key, endpoint string) bool {
	rec, err := store.Find(ctx, actorType, actorID, key, endpoint)
	if err != nil || rec == nil {
		return false
	}
	c.Header(HeaderIdemReplay, "1")
	c.Header("X-Tnb-Idempotent-Source", "db")
	body := rec.ResponseBody
	if len(body) == 0 {
		body = []byte("{}")
	}
	status := rec.ResponseStatus
	if status == 0 {
		status = http.StatusOK
	}
	c.Data(status, "application/json", body)
	c.Abort()
	return true
}

type idemResponseRecorder struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (r *idemResponseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

func (r *idemResponseRecorder) WriteString(s string) (int, error) {
	r.body.WriteString(s)
	return r.ResponseWriter.WriteString(s)
}
