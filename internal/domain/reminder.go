package domain

import "time"

type ReminderType string

const (
	ReminderTypeDueAlert         ReminderType = "due_alert"         // наступление дедлайна
	ReminderTypePreEvent         ReminderType = "pre_event"         // за 15-30 минут до события/встречи
	ReminderTypeProactiveWarning ReminderType = "proactive_warning" // предупреждение агента ("магазины скоро закроются")
	ReminderTypeMorningBrief     ReminderType = "morning_brief"     // утренняя сводка планов дня
)

type Reminder struct {
	ID        int64        `json:"id"`
	UserID    int64        `json:"user_id"`
	TaskID    *int64       `json:"task_id,omitempty"`
	Type      ReminderType `json:"type"`
	TriggerAt time.Time    `json:"trigger_at"`
	SentAt    *time.Time   `json:"sent_at,omitempty"`
	Message   string       `json:"message"`
	CreatedAt time.Time    `json:"created_at"`
}
