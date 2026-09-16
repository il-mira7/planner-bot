package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Update — минимум полей Telegram Bot API, нужных боту.
type Update struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		From *struct {
			ID       int64  `json:"id"`
			UserName string `json:"username"`
		} `json:"from"`
		Text string `json:"text"`
	} `json:"message"`
}

type telegram struct {
	token string
	http  *http.Client
}

func newTelegram(token string) *telegram {
	return &telegram{token: token, http: &http.Client{Timeout: 35 * time.Second}}
}

func (t *telegram) call(ctx context.Context, method string, payload any, out any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.telegram.org/bot"+t.token+"/"+method, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.http.Do(req)
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

func (t *telegram) getUpdates(ctx context.Context, offset int64) ([]Update, error) {
	var updates []Update
	err := t.call(ctx, "getUpdates", map[string]any{
		"offset":          offset,
		"timeout":         25,
		"allowed_updates": []string{"message"},
	}, &updates)
	return updates, err
}

func (t *telegram) sendMessage(ctx context.Context, chatID int64, text string) error {
	return t.call(ctx, "sendMessage", map[string]any{
		"chat_id": chatID,
		"text":    truncateRunes(text, 3800),
	}, nil)
}

// truncateRunes режет по границе рун, чтобы не ломать кириллицу.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
