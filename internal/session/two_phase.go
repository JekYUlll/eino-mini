package session

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var ErrUserPruned = errors.New("user message pruned before assistant insertion")

// Phase 1: 原子追加 user（很快）
// 返回：追加后快照 + 本次 user 的 msgID
func (s *Store) AppendUser(ctx context.Context, convID string, userContent string) ([]Message, string, error) {
	key := s.key(convID)
	userID := uuid.NewString()

	var out []Message

	err := s.rdb.Watch(ctx, func(tx *redis.Tx) error {
		// load
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

		// init system if empty
		if len(cur) == 0 {
			cur = append(cur, Message{
				Role:    "system",
				Content: "你是一个后端助手，回答简洁、工程化。",
			})
		}

		// append user with id
		cur = append(cur, Message{
			ID:      userID,
			Role:    "user",
			Content: userContent,
		})

		// prune BEFORE saving
		cur = Prune(cur)

		b, err := json.Marshal(cur)
		if err != nil {
			return err
		}

		_, err = tx.TxPipelined(ctx, func(p redis.Pipeliner) error {
			p.Set(ctx, key, b, s.ttl)
			return nil
		})
		if err != nil {
			return err
		}

		out = cur
		return nil
	}, key)

	if err != nil {
		return nil, "", err
	}
	return out, userID, nil
}

// Phase 2: 把 assistant 插回 “对应 user 后面”
// 并发下即使有其他 user 已经追加，也能找到 userID 并插入到它后面。
func (s *Store) InsertAssistant(ctx context.Context, convID, userID, assistantContent string) error {
	key := s.key(convID)

	return s.rdb.Watch(ctx, func(tx *redis.Tx) error {
		// load
		val, err := tx.Get(ctx, key).Result()
		if err == redis.Nil {
			return ErrUserPruned
		}
		if err != nil {
			return err
		}
		var cur []Message
		if err := json.Unmarshal([]byte(val), &cur); err != nil {
			return err
		}

		// find user index
		userIdx := -1
		for i := range cur {
			if cur[i].Role == "user" && cur[i].ID == userID {
				userIdx = i
				break
			}
		}
		if userIdx == -1 {
			return ErrUserPruned
		}

		// idempotency: 如果已经插过（同一个 parent_id），直接成功返回
		for i := range cur {
			if cur[i].Role == "assistant" && cur[i].ParentID == userID {
				return nil
			}
		}

		// insert assistant right after userIdx
		assist := Message{
			ID:       uuid.NewString(),
			ParentID: userID,
			Role:     "assistant",
			Content:  assistantContent,
		}

		next := make([]Message, 0, len(cur)+1)
		next = append(next, cur[:userIdx+1]...)
		next = append(next, assist)
		next = append(next, cur[userIdx+1:]...)

		// prune AFTER insertion
		next = Prune(next)

		b, err := json.Marshal(next)
		if err != nil {
			return err
		}

		_, err = tx.TxPipelined(ctx, func(p redis.Pipeliner) error {
			p.Set(ctx, key, b, s.ttl)
			return nil
		})
		return err
	}, key)
}
