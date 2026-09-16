package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"planner-bot/internal/domain"
)

type Client struct {
	token    string
	endpoint string
	http     *http.Client
}

func NewClient(token string) *Client {
	endpoint := os.Getenv("PLANNER_TELEGRAM_ENDPOINT")
	if endpoint == "" {
		endpoint = "https://api.telegram.org"
	}
	endpoint = strings.TrimRight(endpoint, "/")

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}

	if proxyStr := os.Getenv("PLANNER_TELEGRAM_PROXY"); proxyStr != "" {
		if parsedURL, err := url.Parse(proxyStr); err == nil {
			transport.Proxy = http.ProxyURL(parsedURL)
		}
	}

	return &Client{
		token:    token,
		endpoint: endpoint,
		http: &http.Client{
			Timeout:   45 * time.Second,
			Transport: transport,
		},
	}
}

type Update struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		MessageID int64 `json:"message_id"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		From *struct {
			ID        int64  `json:"id"`
			Username  string `json:"username"`
			FirstName string `json:"first_name"`
		} `json:"from"`
		Text  string `json:"text"`
		Voice *struct {
			FileID   string `json:"file_id"`
			Duration int    `json:"duration"`
			MimeType string `json:"mime_type"`
		} `json:"voice"`
	} `json:"message"`
	CallbackQuery *struct {
		ID      string `json:"id"`
		From    struct {
			ID int64 `json:"id"`
		} `json:"from"`
		Message *struct {
			MessageID int64 `json:"message_id"`
			Chat      struct {
				ID int64 `json:"id"`
			} `json:"chat"`
			Text string `json:"text"`
		} `json:"message"`
		Data string `json:"data"`
	} `json:"callback_query"`
}

func (c *Client) GetUpdates(ctx context.Context, offset int64) ([]Update, error) {
	var updates []Update
	payload := map[string]any{
		"offset":          offset,
		"timeout":         25,
		"allowed_updates": []string{"message", "callback_query"},
	}
	err := c.call(ctx, "getUpdates", payload, &updates)
	return updates, err
}

func (c *Client) SendMessage(ctx context.Context, chatID int64, text string, keyboard [][]domain.InlineButton) error {
	payload := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	if len(keyboard) > 0 {
		payload["reply_markup"] = buildReplyMarkup(keyboard)
	}
	return c.call(ctx, "sendMessage", payload, nil)
}

func (c *Client) EditMessageText(ctx context.Context, chatID int64, messageID int64, text string, keyboard [][]domain.InlineButton) error {
	payload := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
	}
	if len(keyboard) > 0 {
		payload["reply_markup"] = buildReplyMarkup(keyboard)
	}
	return c.call(ctx, "editMessageText", payload, nil)
}

func (c *Client) AnswerCallback(ctx context.Context, callbackID string, text string) error {
	payload := map[string]any{
		"callback_query_id": callbackID,
		"text":              text,
	}
	return c.call(ctx, "answerCallbackQuery", payload, nil)
}

func (c *Client) DownloadFile(ctx context.Context, fileID string) ([]byte, error) {
	var fileInfo struct {
		FilePath string `json:"file_path"`
	}
	if err := c.call(ctx, "getFile", map[string]string{"file_id": fileID}, &fileInfo); err != nil {
		return nil, fmt.Errorf("get file info: %w", err)
	}

	downloadURL := fmt.Sprintf("%s/file/bot%s/%s", c.endpoint, c.token, fileInfo.FilePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download file request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download status: %s", resp.Status)
	}

	return io.ReadAll(resp.Body)
}

func (c *Client) call(ctx context.Context, method string, payload any, out any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/bot%s/%s", c.endpoint, c.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var envelope struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return err
	}
	if !envelope.OK {
		return fmt.Errorf("telegram %s: %s", method, envelope.Description)
	}
	if out != nil {
		return json.Unmarshal(envelope.Result, out)
	}
	return nil
}

func buildReplyMarkup(keyboard [][]domain.InlineButton) map[string]any {
	var tgKeyboard [][]map[string]string
	for _, row := range keyboard {
		var tgRow []map[string]string
		for _, btn := range row {
			tgRow = append(tgRow, map[string]string{
				"text":          btn.Text,
				"callback_data": btn.Data,
			})
		}
		tgKeyboard = append(tgKeyboard, tgRow)
	}
	return map[string]any{"inline_keyboard": tgKeyboard}
}
