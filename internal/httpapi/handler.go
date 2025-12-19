package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/JekYUlll/eino-mini/internal/llm"
	"github.com/JekYUlll/eino-mini/internal/session"
)

type Server struct {
	LLM   *llm.Client
	Store *session.Store
}

type askReq struct {
	ConversationID string `json:"conversation_id"`
	Question       string `json:"question"`
}
type askResp struct {
	ConversationID string `json:"conversation_id"`
	Answer         string `json:"answer"`
}

func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/ask", s.ask)
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) ask(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req askReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Question == "" {
		http.Error(w, "bad json or empty question", http.StatusBadRequest)
		return
	}

	// 1) convID
	convID := strings.TrimSpace(req.ConversationID)
	for strings.HasPrefix(convID, "chat_session:") {
		convID = strings.TrimPrefix(convID, "chat_session:")
	}
	if convID == "" {
		convID = s.Store.NewConversationID()
	}

	wait := 8 * time.Second
	if v := os.Getenv("CHAT_LOCK_WAIT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			wait = d
		}
	}
	deadline := time.Now().Add(wait)

	var token string
	for {
		t, ok, err := s.Store.AcquireLock(r.Context(), convID)
		if err != nil {
			http.Error(w, "redis lock error: "+err.Error(), http.StatusBadGateway)
			return
		}
		if ok {
			token = t
			break
		}

		if time.Now().After(deadline) {
			http.Error(w, "conversation is busy, try again", http.StatusTooManyRequests) // 429
			return
		}

		time.Sleep(80 * time.Millisecond)
	}

	defer func() { _ = s.Store.ReleaseLock(context.Background(), convID, token) }()

	// 2) Phase 1: 先把 user 原子写入 Redis，拿到快照和 userID
	history, userID, err := s.Store.AppendUser(r.Context(), convID, req.Question)
	if err != nil {
		http.Error(w, "redis append user error: "+err.Error(), http.StatusBadGateway)
		return
	}

	// 3) 调 LLM（事务外）
	answer, err := s.LLM.AskWithHistory(r.Context(), history)
	if err != nil {
		http.Error(w, "llm error: "+err.Error(), http.StatusBadGateway)
		return
	}

	// 4) Phase 2: 把 assistant 插回对应 user 后面（带重试）
	const maxRetry = 3
	for i := 0; i < maxRetry; i++ {
		err = s.Store.InsertAssistant(r.Context(), convID, userID, answer)
		if err == nil {
			break
		}
		// 如果 user 在此期间被 prune 掉了，只能放弃落库（但仍返回答案）
		if err == session.ErrUserPruned {
			break
		}
	}
	// 不要因为落库失败就让请求失败（你也可以选择失败）
	// 这里先走“用户优先”：返回 answer

	w.Header().Set("content-type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(askResp{
		ConversationID: convID,
		Answer:         answer,
	})
}
