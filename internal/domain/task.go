package domain

import "time"

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusDone      TaskStatus = "done"
	TaskStatusCancelled TaskStatus = "cancelled"
)

type TaskPriority string

const (
	TaskPriorityLow    TaskPriority = "low"
	TaskPriorityMedium TaskPriority = "medium"
	TaskPriorityHigh   TaskPriority = "high"
	TaskPriorityUrgent TaskPriority = "urgent"
)

type TaskCategory string

const (
	TaskCategoryWork     TaskCategory = "work"
	TaskCategoryPersonal TaskCategory = "personal"
	TaskCategoryShopping TaskCategory = "shopping"
	TaskCategoryHealth   TaskCategory = "health"
	TaskCategoryStudy    TaskCategory = "study"
	TaskCategoryOther    TaskCategory = "other"
)

// Task представляет задачу или событие пользователя.
type Task struct {
	ID              int64        `json:"id"`
	UserID          int64        `json:"user_id"`
	Title           string       `json:"title"`
	Description     string       `json:"description,omitempty"`
	Category        TaskCategory `json:"category"`
	Priority        TaskPriority `json:"priority"`
	DueAt           *time.Time   `json:"due_at,omitempty"`           // крайний срок
	ScheduledAt     *time.Time   `json:"scheduled_at,omitempty"`     // запланированное время старта
	DurationMinutes int          `json:"duration_minutes,omitempty"` // примерная продолжительность
	Constraints     []string     `json:"constraints,omitempty"`      // внешние ограничения ("магазины до 22", etc.)
	Status          TaskStatus   `json:"status"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
	CompletedAt     *time.Time   `json:"completed_at,omitempty"`
}

type TaskFilter struct {
	Status    *TaskStatus
	Category  *TaskCategory
	DueBefore *time.Time
	DueAfter  *time.Time
}
