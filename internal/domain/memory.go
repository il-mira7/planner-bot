package domain

import "time"

type FactCategory string

const (
	FactCategoryHabit      FactCategory = "habit"      // график сна, тренировки, привычки
	FactCategoryPlace      FactCategory = "place"      // режим работы любимых магазинов, адреса
	FactCategoryGoal       FactCategory = "goal"       // текущие фокусы (поиск работы, здоровье)
	FactCategoryPreference FactCategory = "preference" // формат отдыха, транспорт, доставка
	FactCategoryRule       FactCategory = "rule"       // жесткие правила ("не ставить встречи до 12")
)

// UserFact — факт о пользователе, сохраняемый в долговременную память агента.
type UserFact struct {
	ID         int64        `json:"id"`
	UserID     int64        `json:"user_id"`
	Category   FactCategory `json:"category"`
	Key        string       `json:"key"`        // например: "supermarket_close", "morning_routine"
	Value      string       `json:"value"`      // например: "23:00", "медленный подъем, кофе 30м"
	Source     string       `json:"source"`     // "conversation", "onboarding", "explicit"
	Confidence float64      `json:"confidence"` // от 0.0 до 1.0
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
}
