package session

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

var ErrUserPruned = errors.New("user message pruned before assistant insertion")

// Phase 1: 原子追加 user（很快）
// 返回：追加后快照 + 本次 user 的 msgID
func (s *Store) AppendUser(ctx context.Context, convID string, userContent string) ([]Message, string, error) {
	key := s.key(convID)
	userID := uuid.NewString()

	cur, err := s.loadMessages(ctx, key)
	if err != nil {
		return nil, "", err
	}

	history := cur
	if len(history) == 0 {
		history = append(history, Message{
			Role:    "system",
			Content: s.systemPrompt(),
		})
	}

	history = append(history, Message{
		ID:      userID,
		Role:    "user",
		Content: userContent,
	})

	pruned := Prune(history)
	snap := append([]Message(nil), pruned...)

	if len(cur) == 0 {
		sys := history[0]
		b, err := json.Marshal(sys)
		if err != nil {
			return nil, "", err
		}
		if err := s.rdb.RPush(ctx, key, b).Err(); err != nil {
			return nil, "", err
		}
	}

	userMsg := history[len(history)-1]
	b, err := json.Marshal(userMsg)
	if err != nil {
		return nil, "", err
	}
	if err := s.rdb.RPush(ctx, key, b).Err(); err != nil {
		return nil, "", err
	}

	if err := s.applyPrune(ctx, key, history); err != nil {
		return nil, "", err
	}

	return snap, userID, nil
}

// Phase 2: 把 assistant 插回 “对应 user 后面”
// 并发下即使有其他 user 已经追加，也能找到 userID 并插入到它后面。
func (s *Store) InsertAssistant(ctx context.Context, convID, userID, assistantContent string) error {
	key := s.key(convID)

	cur, err := s.loadMessages(ctx, key)
	if err != nil {
		return err
	}
	if len(cur) == 0 {
		return ErrUserPruned
	}

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

	for i := range cur {
		if cur[i].Role == "assistant" && cur[i].ParentID == userID {
			return nil
		}
	}

	assist := Message{
		ID:       uuid.NewString(),
		ParentID: userID,
		Role:     "assistant",
		Content:  assistantContent,
	}
	assistJSON, err := json.Marshal(assist)
	if err != nil {
		return err
	}

	userJSON, err := json.Marshal(cur[userIdx])
	if err != nil {
		return err
	}

	res, err := s.rdb.LInsert(ctx, key, "AFTER", userJSON, assistJSON).Result()
	if err != nil {
		return err
	}
	if res == -1 {
		return ErrUserPruned
	}

	next := make([]Message, 0, len(cur)+1)
	next = append(next, cur[:userIdx+1]...)
	next = append(next, assist)
	next = append(next, cur[userIdx+1:]...)
	next = Prune(next)

	return s.applyPrune(ctx, key, next)
}
