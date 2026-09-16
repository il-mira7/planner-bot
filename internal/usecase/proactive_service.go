package usecase

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"planner-bot/internal/domain"
)

type ProactiveService struct {
	userRepo domain.UserRepository
	taskRepo domain.TaskRepository
	memRepo  domain.MemoryRepository
	remRepo  domain.ReminderRepository
	tgSender domain.TelegramSender
	llm      domain.LLMClient
}

func NewProactiveService(
	userRepo domain.UserRepository,
	taskRepo domain.TaskRepository,
	memRepo domain.MemoryRepository,
	remRepo domain.ReminderRepository,
	tgSender domain.TelegramSender,
	llm domain.LLMClient,
) *ProactiveService {
	return &ProactiveService{
		userRepo: userRepo,
		taskRepo: taskRepo,
		memRepo:  memRepo,
		remRepo:  remRepo,
		tgSender: tgSender,
		llm:      llm,
	}
}

// CheckAndNotify выполняет один цикл проверки для всех пользователей.
func (s *ProactiveService) CheckAndNotify(ctx context.Context) {
	users, err := s.userRepo.ListAll(ctx)
	if err != nil {
		log.Printf("proactive: list users error: %v", err)
		return
	}

	now := time.Now().UTC()
	for _, u := range users {
		s.checkUser(ctx, &u, now)
	}
}

func (s *ProactiveService) checkUser(ctx context.Context, u *domain.User, now time.Time) {
	userNow := now.In(u.Location())

	// Если у пользователя тихий час — не беспокоим
	if u.InQuietHours(now) {
		return
	}

	pendingStatus := domain.TaskStatusPending
	tasks, err := s.taskRepo.ListByUser(ctx, u.ID, domain.TaskFilter{Status: &pendingStatus})
	if err != nil {
		log.Printf("proactive: list tasks for user %d: %v", u.ID, err)
		return
	}

	// 1. Проверяем задачи с наступающим дедлайном (в течение 30 минут) или только что наступившим
	for _, t := range tasks {
		if t.DueAt == nil {
			continue
		}

		due := *t.DueAt
		diff := due.Sub(now)

		// Напоминание за ~30 минут до дедлайна
		if diff > 0 && diff <= 30*time.Minute {
			s.sendTaskAlert(ctx, u, &t, domain.ReminderTypePreEvent,
				fmt.Sprintf("⏳ До дедлайна 30 минут: «%s»", t.Title))
		} else if diff <= 0 && diff >= -15*time.Minute {
			// Напоминание в момент дедлайна
			s.sendTaskAlert(ctx, u, &t, domain.ReminderTypeDueAlert,
				fmt.Sprintf("⏰ Наступил дедлайн по задаче: «%s»", t.Title))
		}
	}

	// 2. Проверка вечерних ограничений (например, магазины закрываются, а задача на покупки висит)
	if userNow.Hour() >= 20 && userNow.Hour() < 22 {
		for _, t := range tasks {
			if t.Category == domain.TaskCategoryShopping && t.DueAt != nil && t.DueAt.In(u.Location()).Day() == userNow.Day() {
				s.sendTaskAlert(ctx, u, &t, domain.ReminderTypeProactiveWarning,
					fmt.Sprintf("🛒 Задача «%s»: вечер уже в разгаре, магазины могут скоро закрыться!", t.Title))
			}
		}
	}
}

func (s *ProactiveService) sendTaskAlert(ctx context.Context, u *domain.User, t *domain.Task, remType domain.ReminderType, text string) {
	// Проверяем, не отправляли ли уже такое напоминание сегодня
	reminders, err := s.remRepo.ListPending(ctx, time.Now().UTC().Add(time.Hour))
	if err == nil {
		for _, r := range reminders {
			if r.TaskID != nil && *r.TaskID == t.ID && r.Type == remType {
				return // уже в очереди или отправлено
			}
		}
	}

	// Формируем интерактивные кнопки
	keyboard := [][]domain.InlineButton{
		{
			{Text: "✅ Сделано", Data: fmt.Sprintf("done:%d", t.ID)},
			{Text: "⏳ +1 час", Data: fmt.Sprintf("postpone:%d:60", t.ID)},
			{Text: "🌙 На завтра", Data: fmt.Sprintf("tomorrow:%d", t.ID)},
		},
	}

	if err := s.tgSender.SendMessage(ctx, u.TelegramID, text, keyboard); err != nil {
		log.Printf("failed to send proactive alert to %d: %v", u.TelegramID, err)
		return
	}

	// Записываем факт отправки
	now := time.Now().UTC()
	rem := &domain.Reminder{
		UserID:    u.ID,
		TaskID:    &t.ID,
		Type:      remType,
		TriggerAt: now,
		SentAt:    &now,
		Message:   text,
	}
	_ = s.remRepo.Create(ctx, rem)
}

// GenerateMorningBrief создает утренний обзор дня с приоритетами и советом ИИ.
func (s *ProactiveService) GenerateMorningBrief(ctx context.Context, u *domain.User) (string, error) {
	pendingStatus := domain.TaskStatusPending
	tasks, err := s.taskRepo.ListByUser(ctx, u.ID, domain.TaskFilter{Status: &pendingStatus})
	if err != nil {
		return "", err
	}

	userNow := time.Now().In(u.Location())
	todayDay := userNow.Day()

	var todayTasks []domain.Task
	var overdueTasks []domain.Task

	for _, t := range tasks {
		if t.DueAt == nil {
			continue
		}
		dueUser := t.DueAt.In(u.Location())
		if dueUser.Before(userNow) {
			overdueTasks = append(overdueTasks, t)
		} else if dueUser.Day() == todayDay && dueUser.Month() == userNow.Month() {
			todayTasks = append(todayTasks, t)
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "☀️ Доброе утро, %s! План на сегодня (%s):\n\n", u.FirstName, userNow.Format("02.01"))

	if len(todayTasks) == 0 && len(overdueTasks) == 0 {
		sb.WriteString("На сегодня жестких дедлайнов нет! Отличный день для фокусной работы или отдыха 🎉")
		return sb.String(), nil
	}

	if len(overdueTasks) > 0 {
		fmt.Fprintf(&sb, "🔴 Просрочено (%d):\n", len(overdueTasks))
		for _, t := range overdueTasks {
			fmt.Fprintf(&sb, "• %s (было %s)\n", t.Title, t.DueAt.In(u.Location()).Format("02.01 15:04"))
		}
		sb.WriteString("\n")
	}

	if len(todayTasks) > 0 {
		fmt.Fprintf(&sb, "🟡 Задачи на сегодня (%d):\n", len(todayTasks))
		for _, t := range todayTasks {
			fmt.Fprintf(&sb, "• [%s] %s — %s\n", t.Priority, t.Title, t.DueAt.In(u.Location()).Format("15:04"))
		}
	}

	return sb.String(), nil
}
