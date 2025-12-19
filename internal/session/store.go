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
	Role    string `json:"role"`
	Content string `json:"content"`
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
	val, err := s.rdb.Get(ctx, s.key(id)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var msgs []Message
	err = json.Unmarshal(val, &msgs)
	if err != nil {
		return nil, err
	}
	return msgs, nil
}

func (s *Store) Save(ctx context.Context, id string, msgs []Message) error {
	b, err := json.Marshal(msgs)
	if err != nil {
		return err
	}

	return s.rdb.Set(ctx, s.key(id), b, s.ttl).Err()
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
		// 1) 读当前值
		var cur []Message
		val, err := tx.Get(ctx, key).Result()
		if err == redis.Nil {
			cur = nil
		} else if err != nil {
			return err
		} else {
			if err := json.Unmarshal([]byte(val), &cur); err != nil {
				return err
			}
		}

		// 2) 让调用方基于 cur 生成新值
		next, err := updater(cur)
		if err != nil {
			return err
		}

		// 3) 序列化
		b, err := json.Marshal(next)
		if err != nil {
			return err
		}

		// 4) MULTI/EXEC 提交（带 TTL）
		_, err = tx.TxPipelined(ctx, func(p redis.Pipeliner) error {
			p.Set(ctx, key, b, s.ttl)
			return nil
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
