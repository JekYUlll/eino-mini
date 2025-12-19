package session

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

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
