package domain

import (
	"context"
	"encoding/json"
	"time"
)

// TaskRepository описывает хранилище задач.
type TaskRepository interface {
	Create(ctx context.Context, task *Task) error
	GetByID(ctx context.Context, id int64) (*Task, error)
	Update(ctx context.Context, task *Task) error
	Delete(ctx context.Context, id int64) error
	ListByUser(ctx context.Context, userID int64, filter TaskFilter) ([]Task, error)
	ListPendingDue(ctx context.Context, before time.Time) ([]Task, error)
}

// UserRepository описывает хранилище профилей пользователей.
type UserRepository interface {
	GetByTelegramID(ctx context.Context, telegramID int64) (*User, error)
	GetByID(ctx context.Context, id int64) (*User, error)
	Create(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User) error
	ListAll(ctx context.Context) ([]User, error)
}

// MemoryRepository описывает хранилище фактов профиля (долговременной памяти).
type MemoryRepository interface {
	SaveFact(ctx context.Context, fact *UserFact) error
	ListFactsByUser(ctx context.Context, userID int64, category *FactCategory) ([]UserFact, error)
	DeleteFact(ctx context.Context, userID int64, key string) error
}

// ReminderRepository описывает хранилище запланированных напоминаний.
type ReminderRepository interface {
	Create(ctx context.Context, reminder *Reminder) error
	MarkSent(ctx context.Context, id int64, sentAt time.Time) error
	ListPending(ctx context.Context, before time.Time) ([]Reminder, error)
}

// LLMMessage описывает сообщение в диалоге с моделью.
type LLMMessage struct {
	Role       string          `json:"role"` // system, user, assistant, tool
	Content    string          `json:"content,omitempty"`
	ToolCalls  []LLMToolCall   `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Raw        json.RawMessage `json:"-"`
}

func (m LLMMessage) MarshalJSON() ([]byte, error) {
	if len(m.Raw) > 0 {
		return m.Raw, nil
	}
	type Alias LLMMessage
	return json.Marshal((Alias)(m))
}

type LLMToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type LLMTool struct {
	Type     string         `json:"type"` // "function"
	Function LLMFunctionDef `json:"function"`
}

type LLMFunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type LLMResponse struct {
	Content    string
	ToolCalls  []LLMToolCall
	RawMessage json.RawMessage
}

// LLMClient описывает интерфейс к нейросети с поддержкой вызова инструментов.
type LLMClient interface {
	ChatWithTools(ctx context.Context, messages []LLMMessage, tools []LLMTool) (*LLMResponse, error)
}

// Transcriber описывает сервис транскрибации речи в текст.
type Transcriber interface {
	TranscribeAudio(ctx context.Context, audioData []byte, filename string) (string, error)
}

// InlineButton представляет кнопку в Telegram.
type InlineButton struct {
	Text string
	Data string
}

// TelegramSender описывает интерфейс отправки сообщений в Telegram.
type TelegramSender interface {
	SendMessage(ctx context.Context, chatID int64, text string, keyboard [][]InlineButton) error
	EditMessageText(ctx context.Context, chatID int64, messageID int64, text string, keyboard [][]InlineButton) error
	AnswerCallback(ctx context.Context, callbackID string, text string) error
	DownloadFile(ctx context.Context, fileID string) ([]byte, error)
}
