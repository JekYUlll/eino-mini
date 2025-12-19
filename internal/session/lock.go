package session

import (
	"context"
	"os"
	"time"

	"github.com/google/uuid"
)

func getDurationEnv(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return def
	}
	return d
}

// AcquireLock：给某个 convID 上锁，返回 token（解锁时要带 token）
// 用 SET key value NX PX 实现。
func (s *Store) AcquireLock(ctx context.Context, convID string) (token string, ok bool, err error) {
	ttl := getDurationEnv("CHAT_LOCK_TTL", 20*time.Second)
	key := "chat:lock:" + convID
	token = uuid.NewString()

	ok, err = s.rdb.SetNX(ctx, key, token, ttl).Result()
	return token, ok, err
}

// ReleaseLock：只允许持有 token 的请求解锁（Lua 校验 value）
// 防止 A 的锁被 B 解掉。
func (s *Store) ReleaseLock(ctx context.Context, convID, token string) error {
	key := "chat:lock:" + convID

	// KEYS[1]=key, ARGV[1]=token
	script := `
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
else
  return 0
end
`
	_, err := s.rdb.Eval(ctx, script, []string{key}, token).Result()
	return err
}
