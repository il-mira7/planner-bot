// Package ai — опциональный ИИ-слой планера через OpenAI-совместимый API
// (GLM, Gemini, OpenRouter, Ollama). Без ключа бот работает только на правилах.
package ai

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
)

const (
	defaultBaseURL = "https://open.bigmodel.cn/api/paas/v4"
	defaultModel   = "glm-4-flash"
)

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

// FromEnv читает PLANNER_API_KEY, PLANNER_BASE_URL, PLANNER_MODEL.
func FromEnv() Config {
	return Config{
		BaseURL: envOr("PLANNER_BASE_URL", defaultBaseURL),
		APIKey:  os.Getenv("PLANNER_API_KEY"),
		Model:   envOr("PLANNER_MODEL", defaultModel),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: 90 * time.Second}}
}

// Enabled — задан ли ключ (можно ли звать модель).
func (c *Client) Enabled() bool { return c.cfg.APIKey != "" }

// Breakdown раскладывает большую задачу на 3–6 конкретных шагов.
func (c *Client) Breakdown(ctx context.Context, task string) ([]string, error) {
	system := "Ты помощник-планировщик. Пользователь даст большую задачу. " +
		"Верни СТРОГО JSON — массив из 3–6 коротких конкретных шагов на русском языке. " +
		"Каждый шаг — законченное действие, начинается с глагола. " +
		"Без markdown-разметки и пояснений, только JSON-массив строк."
	raw, err := c.chat(ctx, system, task)
	if err != nil {
		return nil, err
	}
	var steps []string
	if err := json.Unmarshal([]byte(stripFences(raw)), &steps); err != nil {
		return nil, fmt.Errorf("модель вернула не JSON: %w", err)
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("модель вернула пустой список")
	}
	return steps, nil
}

// DayAdvice советует, с чего начать день (2–3 предложения).
func (c *Client) DayAdvice(ctx context.Context, summary string) (string, error) {
	system := "Ты личный ассистент. Пользователь пришлёт сводку задач на день. " +
		"Ответь 2–3 предложениями на русском: с чего начать и что важно учесть. " +
		"По делу, без приветствий и без воды."
	return c.chat(ctx, system, summary)
}

func (c *Client) chat(ctx context.Context, system, user string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model": c.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"temperature": 0.3,
	})
	if err != nil {
		return "", err
	}
	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("провайдер вернул %s: %.200s", resp.Status, raw)
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("пустой ответ модели")
	}
	return parsed.Choices[0].Message.Content, nil
}

// stripFences убирает ```-обёртки и лишний текст вокруг JSON-массива.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "```"); i >= 0 {
		s = s[i:]
		if j := strings.Index(s, "\n"); j >= 0 {
			s = s[j+1:]
		}
		s = strings.ReplaceAll(s, "```", "")
	}
	if i := strings.Index(s, "["); i >= 0 {
		if j := strings.LastIndex(s, "]"); j > i {
			return s[i : j+1]
		}
	}
	return s
}
