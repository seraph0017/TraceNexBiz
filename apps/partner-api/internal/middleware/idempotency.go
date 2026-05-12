// Idempotency middleware（backend §8 / integration §1.2 / Round-1 P0 fail-closed）。
//
// 实现简化版（W1a）：redis SETNX 24h TTL + 响应 replay。
//   - 仅 POST/PUT/PATCH 强制；GET/DELETE 跳过
//   - 必须带 Idempotency-Key header；缺失或非法 UUID → 400
//   - per-actor namespace key：idem:{actor_type}:{actor_id}:{key}
//   - SETNX 成功 → 运行 handler，记录响应到 redis 24h
//   - SETNX 失败 → 取出已存响应回放（X-Tnb-Idempotent-Replay: 1）
//   - redis 任何错误 → 503（fail-closed）
//
// 注：v0.2.2 的 KMS envelope 加密 + saga_step 状态分流由 W1b service 层在更复杂场景接管；
// W1a middleware 仅保证 idempotent 语义不丢失。
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

// IdemRepoReader 兼容 v0.2.2 设计（W1b service 层接管时使用）。
type IdemRepoReader interface {
	Find(c *gin.Context, actorType string, actorID int64, key, endpoint string) (*IdemRecord, error)
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

// Idempotency 装配 SETNX-based 幂等 middleware。
//
// ttl 推荐 24h（cfg.IdempotencyTTL）。
func Idempotency(rds *redis.Client, ttl time.Duration) gin.HandlerFunc {
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
		if rds == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "idempotency_unavailable"})
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
		redisKey := fmt.Sprintf("idem:%s:%d:%s", atStr, aid, key)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		ok, err := rds.SetNX(ctx, redisKey, "PENDING", ttl).Result()
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "idempotency_unavailable"})
			return
		}
		c.Set(CtxKeyIdemKey, key)
		c.Set(CtxKeyIdemActor, fmt.Sprintf("%s:%d", atStr, aid))

		if !ok {
			val, err := rds.Get(ctx, redisKey).Result()
			if err != nil {
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "idempotency_unavailable"})
				return
			}
			if val == "PENDING" || val == "" {
				c.AbortWithStatusJSON(http.StatusAccepted, gin.H{"status": "in_progress"})
				return
			}
			var p idemPayload
			if err := json.Unmarshal([]byte(val), &p); err != nil {
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
