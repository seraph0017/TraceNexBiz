// idempotency_db_fallback_test.go — Fix-B' part 2 CRIT-B3 中间件 DB 回退路径.
package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDBLookup 内存实现 IdempotencyDBLookup；测试用.
type fakeDBLookup struct {
	records map[string]*IdempotencyDBRecord
	err     error
}

func (f *fakeDBLookup) Find(_ context.Context, actorType string, actorID int64, key, endpoint string) (*IdempotencyDBRecord, error) {
	if f.err != nil {
		return nil, f.err
	}
	k := mkKey(actorType, actorID, key, endpoint)
	r, ok := f.records[k]
	if !ok {
		return nil, nil
	}
	return r, nil
}

func mkKey(at string, aid int64, key, endpoint string) string {
	return at + "|" + key + "|" + endpoint
}

// TestIdem_RedisDown_FallsBackToDB：Redis 故障但 DB 有记录 → middleware 回放 DB 内容.
func TestIdem_RedisDown_FallsBackToDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mr, err := miniredis.Run()
	require.NoError(t, err)
	rds := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1, DialTimeout: 200 * time.Millisecond})
	mr.Close() // simulate Redis outage

	store := &fakeDBLookup{records: map[string]*IdempotencyDBRecord{
		mkKey("anon", 0, "11111111-1111-1111-1111-111111111111", "POST /x"): {
			ResponseStatus: 200,
			ResponseBody:   []byte(`{"replayed":"from-db"}`),
		},
	}}
	r := gin.New()
	r.Use(IdempotencyWithDB(rds, time.Hour, store))
	r.POST("/x", func(c *gin.Context) {
		t.Fatal("handler must not run when DB has cached response")
	})
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set(HeaderIdemKey, "11111111-1111-1111-1111-111111111111")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, `{"replayed":"from-db"}`, w.Body.String())
	assert.Equal(t, "1", w.Header().Get(HeaderIdemReplay))
	assert.Equal(t, "db", w.Header().Get("X-Tnb-Idempotent-Source"))
}

// TestIdem_RedisDown_DBMiss_503：Redis 不可用 + DB miss → 503 fail-closed.
func TestIdem_RedisDown_DBMiss_503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mr, err := miniredis.Run()
	require.NoError(t, err)
	rds := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1, DialTimeout: 200 * time.Millisecond})
	mr.Close()

	store := &fakeDBLookup{records: map[string]*IdempotencyDBRecord{}}
	r := gin.New()
	r.Use(IdempotencyWithDB(rds, time.Hour, store))
	r.POST("/x", func(c *gin.Context) { c.Status(204) })
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set(HeaderIdemKey, "11111111-1111-1111-1111-111111111111")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 503, w.Code)
}

// TestIdem_RedisPending_DBHit：Redis 是 PENDING（并发请求正在跑），但 DB 已 commit → 回放.
func TestIdem_RedisPending_DBHit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rds, _ := mustRedis(t)
	// 预置 PENDING 值
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	const key = "22222222-2222-2222-2222-222222222222"
	redisKey := "idem:anon:0:" + key
	require.NoError(t, rds.Set(ctx, redisKey, "PENDING", time.Hour).Err())

	store := &fakeDBLookup{records: map[string]*IdempotencyDBRecord{
		mkKey("anon", 0, key, "POST /x"): {
			ResponseStatus: 200,
			ResponseBody:   []byte(`{"src":"db"}`),
		},
	}}
	r := gin.New()
	r.Use(IdempotencyWithDB(rds, time.Hour, store))
	r.POST("/x", func(c *gin.Context) { t.Fatal("must replay from DB") })
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set(HeaderIdemKey, key)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, `{"src":"db"}`, w.Body.String())
}

// TestIdem_DBLookupError_FallsThroughToRedisAvailable：DB store 错误 + Redis 可用 →
// 走 Redis 路径正常运行.
func TestIdem_DBLookupError_FallsThroughToRedisAvailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rds, _ := mustRedis(t)
	store := &fakeDBLookup{err: errors.New("db connection refused")}
	r := gin.New()
	r.Use(IdempotencyWithDB(rds, time.Hour, store))
	r.POST("/x", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set(HeaderIdemKey, "33333333-3333-3333-3333-333333333333")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// Redis SETNX 成功 → handler 跑 → 200 OK；DB lookup 错误不影响 fast-path.
	assert.Equal(t, 200, w.Code)
}
