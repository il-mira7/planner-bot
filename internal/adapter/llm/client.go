package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"planner-bot/internal/domain"
)

const (
	defaultBaseURL = "https://api.groq.com/openai/v1"
	defaultModel   = "llama-3.3-70b-versatile"
)

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

func FromEnv() Config {
	return Config{
		BaseURL: envOr("PLANNER_BASE_URL", defaultBaseURL),
		APIKey:  os.Getenv("PLANNER_API_KEY"),
		Model:   envOr("PLANNER_MODEL", defaultModel),
	}
}

func envOr(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: 90 * time.Second},
	}
}

func (c *Client) Enabled() bool {
	return c.cfg.APIKey != ""
}

// chatRequest — запрос к OpenAI-совместимому /chat/completions с tools.
type chatRequest struct {
	Model       string               `json:"model"`
	Messages    []domain.LLMMessage  `json:"messages"`
	Tools       []domain.LLMTool     `json:"tools,omitempty"`
	ToolChoice  string               `json:"tool_choice,omitempty"`
	Temperature float64              `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message      json.RawMessage `json:"message"`
		FinishReason string          `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// ChatWithTools выполняет один шаг диалога с моделью с поддержкой Function Calling.
func (c *Client) ChatWithTools(ctx context.Context, messages []domain.LLMMessage, tools []domain.LLMTool) (*domain.LLMResponse, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("API-ключ нейросети (PLANNER_API_KEY) не задан")
	}

	reqBody := chatRequest{
		Model:       c.cfg.Model,
		Messages:    messages,
		Tools:       tools,
		Temperature: 0.2, // Низкая температура для точного вызова инструментов
	}
	if len(tools) > 0 {
		reqBody.ToolChoice = "auto"
	}

	raw, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal chat request: %w", err)
	}

	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	httpResp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer httpResp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(httpResp.Body, 2<<20)) // 2MB limit
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API provider error (%s): %.300s", httpResp.Status, bodyBytes)
	}

	var resp chatResponse
	if err := json.Unmarshal(bodyBytes, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal chat response: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("provider returned error: %s", resp.Error.Message)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("provider returned empty choices")
	}

	var parsedMsg struct {
		Role      string               `json:"role"`
		Content   *string              `json:"content"`
		ToolCalls []domain.LLMToolCall `json:"tool_calls,omitempty"`
	}
	if err := json.Unmarshal(resp.Choices[0].Message, &parsedMsg); err != nil {
		return nil, fmt.Errorf("unmarshal choice message: %w", err)
	}

	content := ""
	if parsedMsg.Content != nil {
		content = *parsedMsg.Content
	}

	return &domain.LLMResponse{
		Content:    content,
		ToolCalls:  parsedMsg.ToolCalls,
		RawMessage: resp.Choices[0].Message,
	}, nil
}
