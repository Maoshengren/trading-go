package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	openaiModel "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"trading-go/config"
)

var (
	mu        sync.RWMutex
	chatModel model.BaseChatModel
)

func Init(ctx context.Context, cfg config.LLMConfig) error {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return errors.New("llm api_key is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return errors.New("llm model is required")
	}

	temperature := cfg.Temperature
	c := &openaiModel.ChatModelConfig{
		APIKey:      cfg.APIKey,
		Model:       cfg.Model,
		Temperature: &temperature,
	}
	if strings.TrimSpace(cfg.BaseURL) != "" {
		c.BaseURL = strings.TrimSpace(cfg.BaseURL)
	}

	m, err := openaiModel.NewChatModel(ctx, c)
	if err != nil {
		return fmt.Errorf("init chat model: %w", err)
	}

	mu.Lock()
	chatModel = m
	mu.Unlock()
	return nil
}

func Model() (model.BaseChatModel, error) {
	mu.RLock()
	defer mu.RUnlock()
	if chatModel == nil {
		return nil, errors.New("llm is not initialized")
	}
	return chatModel, nil
}

func Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	m, err := Model()
	if err != nil {
		return "", err
	}

	msg, err := m.Generate(ctx, []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(userPrompt),
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(msg.Content), nil
}

func GenerateJSON(ctx context.Context, systemPrompt, userPrompt string, out any) error {
	raw, err := Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return err
	}
	payload := extractJSON(raw)
	if err := json.Unmarshal([]byte(payload), out); err != nil {
		return fmt.Errorf("unmarshal llm json: %w; raw=%s", err, raw)
	}
	return nil
}

func extractJSON(raw string) string {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}
