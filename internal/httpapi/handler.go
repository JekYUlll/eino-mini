package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/JekYUlll/eino-mini/internal/llm"
)

type Server struct {
	LLM *llm.Client
}

type askReq struct {
	Question string `json:"question"`
}
type askResp struct {
	Answer string `json:"answer"`
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

	ans, err := s.LLM.Ask(r.Context(), req.Question)
	if err != nil {
		http.Error(w, "llm error: "+err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("content-type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(askResp{Answer: ans})
}
