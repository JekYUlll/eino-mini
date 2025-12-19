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

	const maxRetries = 3

	var answer string

	for attempt := 1; attempt <= maxRetries; attempt++ {
		updated, err := s.Store.Update(r.Context(), convID, func(cur []session.Message) ([]session.Message, error) {
			history := cur
			if len(history) == 0 {
				history = append(history, session.Message{
					Role:    "system",
					Content: "你是一个后端助手，回答简洁、工程化。",
				})
			}

			// 追加本轮 user
			history = append(history, session.Message{
				Role:    "user",
				Content: req.Question,
			})

			// 先裁剪，避免把超长上下文送给模型
			history = session.Prune(history)

			// 调模型（注意：这里在 updater 里调用 LLM，会拉长 WATCH 时间）
			// 这是最小改动版本，能正确，但吞吐一般。下一步我们会优化成“两阶段提交”。
			a, llmErr := s.LLM.AskWithHistory(r.Context(), history)
			if llmErr != nil {
				return nil, llmErr
			}
			answer = a

			// 追加 assistant
			history = append(history, session.Message{
				Role:    "assistant",
				Content: answer,
			})

			// 再裁剪一次（防止 assistant 太长）
			history = session.Prune(history)

			return history, nil
		})

		if err == nil {
			_ = updated // 你如果想 debug，可以把 updated 返回给客户端
			break
		}

		if err == session.ErrConflict && attempt < maxRetries {
			// 冲突重试
			continue
		}

		// 其他错误 / 重试耗尽
		http.Error(w, "session update error: "+err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("content-type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(askResp{
		ConversationID: convID,
		Answer:         answer,
	})
}
