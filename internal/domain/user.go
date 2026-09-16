package domain

import "time"

// User представляет пользователя бота с персональными настройками.
type User struct {
	ID                  int64     `json:"id"`
	TelegramID          int64     `json:"telegram_id"`
	Username            string    `json:"username,omitempty"`
	FirstName           string    `json:"first_name,omitempty"`
	Timezone            string    `json:"timezone"`              // по умолчанию "Europe/Moscow"
	QuietHoursStart     int       `json:"quiet_hours_start"`     // час начала тишины (например 23)
	QuietHoursEnd       int       `json:"quiet_hours_end"`       // час окончания тишины (например 8)
	OnboardingCompleted bool      `json:"onboarding_completed"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (u *User) Location() *time.Location {
	if u.Timezone == "" {
		return time.Local
	}
	loc, err := time.LoadLocation(u.Timezone)
	if err != nil {
		return time.Local
	}
	return loc
}

// InQuietHours проверяет, попадает ли заданное время в тихие часы пользователя.
func (u *User) InQuietHours(t time.Time) bool {
	userTime := t.In(u.Location())
	hour := userTime.Hour()
	if u.QuietHoursStart > u.QuietHoursEnd {
		// Например, с 23:00 до 08:00
		return hour >= u.QuietHoursStart || hour < u.QuietHoursEnd
	}
	// Например, с 01:00 до 06:00
	return hour >= u.QuietHoursStart && hour < u.QuietHoursEnd
}
