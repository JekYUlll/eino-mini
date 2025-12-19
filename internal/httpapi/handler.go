package httpapi

import (
	"encoding/json"
	"net/http"

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

	convID := req.ConversationID
	if convID == "" {
		convID = s.Store.NewConversationID()
	}

	history, err := s.Store.Load(r.Context(), convID)
	if err != nil {
		http.Error(w, "redis load error", http.StatusBadGateway)
		return
	}

	// 首次对话：注入 system
	if len(history) == 0 {
		history = append(history, session.Message{
			Role:    "system",
			Content: "你是一个后端助手，回答简洁、工程化。",
		})
	}

	history = append(history, session.Message{
		Role:    "user",
		Content: req.Question,
	})

	answer, err := s.LLM.AskWithHistory(r.Context(), history)
	if err != nil {
		http.Error(w, "llm error: "+err.Error(), http.StatusBadGateway)
		return
	}

	history = append(history, session.Message{
		Role:    "assistant",
		Content: answer,
	})

	// 裁剪
	history = session.Prune(history)
	// 存回 Redis（续期 TTL）
	_ = s.Store.Save(r.Context(), convID, history)

	w.Header().Set("content-type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(askResp{
		ConversationID: convID,
		Answer:         answer,
	})
}
