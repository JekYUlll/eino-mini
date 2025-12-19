package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/JekYUlll/eino-mini/internal/httpapi"
	"github.com/JekYUlll/eino-mini/internal/llm"
	"github.com/JekYUlll/eino-mini/internal/session"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	llmClient, err := llm.New(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	store, err := session.NewStore()
	if err != nil {
		log.Fatal(err)
	}

	s := &httpapi.Server{
		LLM:   llmClient,
		Store: store,
	}
	mux := http.NewServeMux()
	s.Register(mux)

	log.Println("listening on : " + port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
