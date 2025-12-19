package session

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var ErrConflict = errors.New("session update conflict, please retry")

type Message struct {
	ID       string `json:"id,omitempty"`
	ParentID string `json:"parent_id,omitempty"` // assistant 对应的 user id
	Role     string `json:"role"`
	Content  string `json:"content"`
}

type Store struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewStore() (*Store, error) {
	ttlStr := os.Getenv("CHAT_SESSION_TTL")
	if ttlStr == "" {
		ttlStr = "30m"
	}
	ttl, err := time.ParseDuration(ttlStr)
	if err != nil {
		return nil, err
	}

	db := 0
	if v := os.Getenv("REDIS_DB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			db = n
		}
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_ADDR"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       db,
	})

	return &Store{
		rdb: rdb,
		ttl: ttl,
	}, nil
}

func (s *Store) NewConversationID() string {
	return uuid.NewString()
}

func (s *Store) key(id string) string {
	return "chat_session:" + id
}

func (s *Store) Load(ctx context.Context, id string) ([]Message, error) {
	return s.loadMessages(ctx, s.key(id))
}

// Save 弃用，仅用于调试
func (s *Store) Save(ctx context.Context, id string, msgs []Message) error {
	return s.saveMessages(ctx, s.key(id), msgs)
}

// Update: 用 WATCH/MULTI 保证 “读-改-写” 在并发下不会丢更新。
// updater 接收当前 msgs（可能为空），返回更新后的 msgs。
func (s *Store) Update(
	ctx context.Context,
	id string,
	updater func(cur []Message) ([]Message, error),
) ([]Message, error) {

	key := s.key(id)

	var out []Message
	err := s.rdb.Watch(ctx, func(tx *redis.Tx) error {
		cur, err := s.loadMessagesCtx(ctx, tx, key)
		if err != nil {
			return err
		}

		// 2) 让调用方基于 cur 生成新值
		next, err := updater(cur)
		if err != nil {
			return err
		}

		// 3) MULTI/EXEC 提交（带 TTL）
		_, err = tx.TxPipelined(ctx, func(p redis.Pipeliner) error {
			return s.writeMessages(ctx, p, key, next)
		})
		if err != nil {
			// 如果 key 在 WATCH 后被别人改过，这里会返回 redis.TxFailedErr
			if errors.Is(err, redis.TxFailedErr) {
				return ErrConflict
			}
			return err
		}

		out = next
		return nil
	}, key)

	if err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateWithRetry: retry Update on ErrConflict up to n times.
func (s *Store) UpdateWithRetry(
	ctx context.Context,
	id string,
	n int,
	updater func(cur []Message) ([]Message, error),
) ([]Message, error) {
	var lastErr error
	for i := 0; i < n; i++ {
		out, err := s.Update(ctx, id, updater)
		if err == nil {
			return out, nil
		}
		if errors.Is(err, ErrConflict) {
			lastErr = err
			continue
		}
		return nil, err
	}
	return nil, lastErr
}

// loadMessages: read all messages stored as a Redis list. Each element is a JSON-encoded Message.
func (s *Store) loadMessages(ctx context.Context, key string) ([]Message, error) {
	return s.loadMessagesCtx(ctx, s.rdb, key)
}

func (s *Store) loadMessagesCtx(ctx context.Context, cmd redis.Cmdable, key string) ([]Message, error) {
	vals, err := cmd.LRange(ctx, key, 0, -1).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	msgs := make([]Message, 0, len(vals))
	for _, v := range vals {
		var m Message
		if err := json.Unmarshal([]byte(v), &m); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

func (s *Store) saveMessages(ctx context.Context, key string, msgs []Message) error {
	_, err := s.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		return s.writeMessages(ctx, p, key, msgs)
	})
	return err
}

func (s *Store) writeMessages(ctx context.Context, p redis.Pipeliner, key string, msgs []Message) error {
	p.Del(ctx, key)
	if len(msgs) > 0 {
		elems := make([]interface{}, 0, len(msgs))
		for _, m := range msgs {
			b, err := json.Marshal(m)
			if err != nil {
				return err
			}
			elems = append(elems, b)
		}
		p.RPush(ctx, key, elems...)
	}
	p.Expire(ctx, key, s.ttl)
	return nil
}

// applyPrune trims the list to match prune rules using list ops, and refreshes TTL.
func (s *Store) applyPrune(ctx context.Context, key string, history []Message) error {
	if len(history) == 0 {
		return s.rdb.Del(ctx, key).Err()
	}

	keepSystem, tailStart := prunePlan(history)
	if tailStart < 0 {
		tailStart = 0
	}

	pipe := s.rdb.Pipeline()
	if tailStart > 0 {
		pipe.LTrim(ctx, key, int64(tailStart), -1)
	}
	if keepSystem && tailStart > 0 {
		b, err := json.Marshal(history[0])
		if err != nil {
			return err
		}
		pipe.LPush(ctx, key, b)
	}
	pipe.Expire(ctx, key, s.ttl)
	_, err := pipe.Exec(ctx)
	return err
}
