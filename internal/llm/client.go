package llm

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/JekYUlll/eino-mini/internal/session"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"
)

func float32Ptr(v float32) *float32 { return &v }

type Client struct {
	model *openai.ChatModel
}

func New(ctx context.Context) (*Client, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	baseURL := os.Getenv("OPENAI_BASE_URL")
	model := os.Getenv("OPENAI_MODEL")
	if apiKey == "" || baseURL == "" || model == "" {
		return nil, fmt.Errorf("missing env: OPENAI_API_KEY / OPENAI_BASE_URL / OPENAI_MODEL")
	}

	cm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:      apiKey,
		BaseURL:     baseURL,
		Model:       model,
		Temperature: float32Ptr(0.2),
	})
	if err != nil {
		return nil, err
	}

	return &Client{model: cm}, nil
}

func (c *Client) Ask(ctx context.Context, question string) (string, error) {
	msgs := []*schema.Message{
		{Role: schema.System, Content: "You are a helpful backend assistant. Answer concisely."},
		{Role: schema.User, Content: question},
	}
	resp, err := c.model.Generate(ctx, msgs)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func (c *Client) AskWithHistory(ctx context.Context, history []session.Message) (string, error) {
	msgs := make([]*schema.Message, 0, len(history))
	for _, m := range history {
		role := strings.ToLower(m.Role)
		if role != string(schema.System) && role != string(schema.User) &&
			role != string(schema.Assistant) && role != string(schema.Tool) {
			role = string(schema.User)
		}
		msgs = append(msgs, &schema.Message{
			Role:    schema.RoleType(role),
			Content: m.Content,
		})
	}

	resp, err := c.model.Generate(ctx, msgs)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}
