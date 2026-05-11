// Idempotency middleware（backend §8.1 v0.2.2 重写：middleware 只做 SELECT 命中 + 重放，不写表）。
//
// 设计要点（CRITICAL，与 ADR-003 一致）：
//   1. 命中 completed → KMS Decrypt(response_cipher) 并 c.JSON 重放
//   2. 命中 pending → 查 saga_step.status，in_progress 返 202，否则 500 orphaned
//   3. 命中但 request_hash 不同 → 409 BIZ_IDEM_REUSED_DIFFERENT_BODY
//   4. 未命中 → 注入 responseRecorder 到 c.Writer，c.Set ctxKeyIdemKey/Hash/Actor，c.Next()
//   5. **不调 repo.Insert**；service 层在业务 TX 闭包内自己 idemRepo.Insert(tx, ...)
//
// W0 scaffold：仅给 wiring；命中检查 / 解密 / responseRecorder 由 W1a 实现。
package middleware

import (
	"github.com/gin-gonic/gin"
)

// HeaderIdemKey state-changing endpoint 必带 idempotency-key header。
const HeaderIdemKey = "Idempotency-Key"

// CtxKeyIdemKey / CtxKeyIdemReqHash / CtxKeyIdemActor 是 service 层取用的 ctx key.
const (
	CtxKeyIdemKey     = "idem_key"
	CtxKeyIdemReqHash = "idem_req_hash"
	CtxKeyIdemActor   = "idem_actor"
)

// IdemRepoReader 由 internal/idempotency 提供的只读视图（middleware 仅 SELECT）。
type IdemRepoReader interface {
	Find(c *gin.Context, actorType string, actorID int64, key, endpoint string) (*IdemRecord, error)
}

// IdemRecord 存量记录。
type IdemRecord struct {
	Status         string
	RequestHash    string
	ResponseStatus int
	ResponseCipher []byte
	ResponseKeyID  string
	SagaID         string
}

// KMSDecryptor 抽象 envelope 加密 service.
type KMSDecryptor interface {
	Decrypt(cipher []byte, keyID string) ([]byte, error)
}

// Idempotency 装配只读检查 + responseRecorder 注入。
//
// W1a 实现：见 backend §8.1 v0.2.2 完整代码块（含 Insert 由 service 层闭包内执行的 invariant）。
func Idempotency(_ IdemRepoReader, _ KMSDecryptor) gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO(W1a): per backend §8.1 v0.2.2 — SELECT-only check, response replay,
		//            inject responseRecorder into c.Writer; service layer Insert in business TX.
		c.Next()
	}
}
