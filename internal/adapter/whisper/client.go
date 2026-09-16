package whisper

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultWhisperBaseURL = "https://api.groq.com/openai/v1"
	defaultWhisperModel   = "whisper-large-v3"
)

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

func FromEnv() Config {
	return Config{
		BaseURL: envOr("PLANNER_BASE_URL", defaultWhisperBaseURL),
		APIKey:  os.Getenv("PLANNER_API_KEY"),
		Model:   envOr("PLANNER_MODEL", defaultWhisperModel),
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

// TranscribeAudio расшифровывает аудио через Gemini Audio API или классический Whisper API.
func (c *Client) TranscribeAudio(ctx context.Context, audioData []byte, filename string) (string, error) {
	if !c.Enabled() {
		return "", fmt.Errorf("API ключ нейросети не задан для транскрибации")
	}

	// Если используется Google Gemini — транскрибируем мультимодально через Gemini
	if strings.Contains(c.cfg.BaseURL, "generativelanguage.googleapis.com") || strings.HasPrefix(c.cfg.APIKey, "AIzaSy") {
		return c.transcribeWithGemini(ctx, audioData)
	}

	// Иначе используем стандартный Whisper API (Groq / OpenAI)
	return c.transcribeWithWhisper(ctx, audioData, filename)
}

// transcribeWithGemini использует встроенную мультимодальность Gemini для транскрибации OGG/OGA аудио из Telegram.
func (c *Client) transcribeWithGemini(ctx context.Context, audioData []byte) (string, error) {
	encodedAudio := base64.StdEncoding.EncodeToString(audioData)

	model := c.cfg.Model
	if model == "" || strings.Contains(model, "whisper") {
		model = "gemini-2.5-flash"
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, c.cfg.APIKey)

	reqBody := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]any{
					{
						"text": "Транскрибируй эту голосовую аудиозапись на русском языке. " +
							"Верни ТОЛЬКО расшифрованный текст без кавычек, без вводных слов и без каких-либо комментариев.",
					},
					{
						"inline_data": map[string]string{
							"mime_type": "audio/ogg",
							"data":      encodedAudio,
						},
					},
				},
			},
		},
		"generationConfig": map[string]any{
			"temperature": 0.0,
		},
	}

	raw, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.cfg.APIKey)
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini audio request error: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini error (%s): %.300s", resp.Status, respBytes)
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", fmt.Errorf("unmarshal gemini audio response: %w", err)
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini не смог распознать речь в аудио")
	}

	return strings.TrimSpace(result.Candidates[0].Content.Parts[0].Text), nil
}

// transcribeWithWhisper использует OpenAI-совместимый Whisper API (например, Groq).
func (c *Client) transcribeWithWhisper(ctx context.Context, audioData []byte, filename string) (string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(audioData)); err != nil {
		return "", fmt.Errorf("copy audio data: %w", err)
	}

	whisperModel := os.Getenv("PLANNER_WHISPER_MODEL")
	if whisperModel == "" {
		whisperModel = "whisper-large-v3"
	}

	if err := writer.WriteField("model", whisperModel); err != nil {
		return "", fmt.Errorf("write model field: %w", err)
	}
	if err := writer.WriteField("language", "ru"); err != nil {
		return "", fmt.Errorf("write language field: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close multipart writer: %w", err)
	}

	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/audio/transcriptions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("http post audio: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("whisper provider error (%s): %.300s", resp.Status, respBytes)
	}

	var result struct {
		Text  string `json:"text"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", fmt.Errorf("unmarshal whisper response: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("whisper error: %s", result.Error.Message)
	}

	return strings.TrimSpace(result.Text), nil
}
