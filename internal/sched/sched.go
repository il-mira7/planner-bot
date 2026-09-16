// Package sched — фоновый цикл: напоминания о дедлайнах
// и утренняя сводка (окно 09:00–12:00, раз в день).
package sched

import (
	"context"
	"fmt"
	"log"
	"time"

	"planner-bot/internal/store"
)

// Notify отправляет сообщение владельцу, Advice спрашивает совет у ИИ
// (может вернуть пустую строку, если ключ не задан).
type Notify func(chatID int64, text string) error
type Advice func(ctx context.Context, summary string) string

func Run(ctx context.Context, st *store.Store, notify Notify, advice Advice) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			tick(ctx, st, notify, advice, now)
		}
	}
}

func tick(ctx context.Context, st *store.Store, notify Notify, advice Advice, now time.Time) {
	chatID := st.ChatID()
	if chatID == 0 {
		return // владелец ещё ни разу не писал боту
	}

	for _, t := range st.Active() {
		if t.Due != nil && !t.Notified && !t.Due.After(now) {
			if err := notify(chatID, fmt.Sprintf("⏰ Дедлайн: «%s»", t.Text)); err != nil {
				log.Printf("напоминание не ушло: %v", err)
			}
			st.MarkNotified(t.ID)
		}
	}

	if st.ShouldBrief(now) {
		if err := notify(chatID, morningBrief(ctx, st, advice, now)); err != nil {
			log.Printf("сводка не ушла: %v", err)
		}
	}
}

func morningBrief(ctx context.Context, st *store.Store, advice Advice, now time.Time) string {
	text := fmt.Sprintf("☀️ Доброе утро! %s\n\n%s", now.Format("02.01"), DaySummary(st, now))
	if a := advice(ctx, text); a != "" {
		text += "\n\n💡 " + a
	}
	return text
}

// DaySummary — сводка задач на день: просрочено, сегодня, дальше.
func DaySummary(st *store.Store, now time.Time) string {
	var overdue, today, later []store.Task
	for _, t := range st.Active() {
		switch {
		case t.Due == nil:
			// без даты — в сводку не тащим
		case t.Due.Before(truncate(now)):
			overdue = append(overdue, t)
		case t.Due.Before(truncate(now.AddDate(0, 0, 1))):
			today = append(today, t)
		default:
			later = append(later, t)
		}
	}
	text := fmt.Sprintf("Сегодня задач: %d", len(today))
	if len(overdue) > 0 {
		text += fmt.Sprintf(", просрочено: %d 🔴", len(overdue))
	}
	if len(later) > 0 {
		text += fmt.Sprintf(", дальше по плану: %d", len(later))
	}
	for _, t := range overdue {
		text += fmt.Sprintf("\n🔴 %d. %s (была %s)", t.ID, t.Text, t.Due.Format("02.01 15:04"))
	}
	for _, t := range today {
		text += fmt.Sprintf("\n🟡 %d. %s — %s", t.ID, t.Text, t.Due.Format("15:04"))
	}
	return text
}

func truncate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
