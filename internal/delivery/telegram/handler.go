package telegram

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	adapterTg "planner-bot/internal/adapter/telegram"
	"planner-bot/internal/domain"
	"planner-bot/internal/usecase"
	"planner-bot/internal/usecase/agent"
)

type Handler struct {
	tgClient     *adapterTg.Client
	userRepo     domain.UserRepository
	taskRepo     domain.TaskRepository
	agent        *agent.Orchestrator
	whisper      domain.Transcriber
	proactive    *usecase.ProactiveService
	allowedUsers map[int64]bool
}

func NewHandler(
	tgClient *adapterTg.Client,
	userRepo domain.UserRepository,
	taskRepo domain.TaskRepository,
	agent *agent.Orchestrator,
	whisper domain.Transcriber,
	proactive *usecase.ProactiveService,
	allowedUsersStr string,
) *Handler {
	allowed := make(map[int64]bool)
	if allowedUsersStr != "" {
		for _, s := range strings.Split(allowedUsersStr, ",") {
			s = strings.TrimSpace(s)
			if id, err := strconv.ParseInt(s, 10, 64); err == nil && id > 0 {
				allowed[id] = true
			}
		}
	}

	return &Handler{
		tgClient:     tgClient,
		userRepo:     userRepo,
		taskRepo:     taskRepo,
		agent:        agent,
		whisper:      whisper,
		proactive:    proactive,
		allowedUsers: allowed,
	}
}

func (h *Handler) HandleUpdate(ctx context.Context, u adapterTg.Update) {
	// Обработка нажатий на инлайн-кнопки
	if u.CallbackQuery != nil {
		h.handleCallback(ctx, u.CallbackQuery)
		return
	}

	msg := u.Message
	if msg == nil || msg.From == nil {
		return
	}

	fromID := msg.From.ID
	if len(h.allowedUsers) > 0 && !h.allowedUsers[fromID] {
		_ = h.tgClient.SendMessage(ctx, msg.Chat.ID, "Извините, бот пока находится в приватном режиме тестирования.", nil)
		return
	}

	// Поиск или создание пользователя
	user, err := h.userRepo.GetByTelegramID(ctx, fromID)
	if err != nil {
		log.Printf("get user error: %v", err)
		return
	}
	if user == nil {
		user = &domain.User{
			TelegramID: fromID,
			Username:   msg.From.Username,
			FirstName:  msg.From.FirstName,
			Timezone:   "Europe/Moscow",
		}
		if err := h.userRepo.Create(ctx, user); err != nil {
			log.Printf("create user error: %v", err)
			return
		}
	}

	// Обработка голосовых сообщений
	if msg.Voice != nil {
		h.handleVoice(ctx, user, msg.Chat.ID, msg.Voice.FileID)
		return
	}

	// Обработка текстовых сообщений
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	switch {
	case strings.HasPrefix(text, "/start"):
		h.handleStart(ctx, user, msg.Chat.ID)
	case strings.HasPrefix(text, "/tasks") || strings.HasPrefix(text, "/list"):
		h.handleListTasks(ctx, user, msg.Chat.ID)
	case strings.HasPrefix(text, "/brief") || strings.HasPrefix(text, "/day"):
		h.handleDayBrief(ctx, user, msg.Chat.ID)
	case strings.HasPrefix(text, "/help"):
		h.handleHelp(ctx, msg.Chat.ID)
	default:
		// Обычный текст передаем AI-агенту
		h.handleAgentMessage(ctx, user, msg.Chat.ID, text)
	}
}

func (h *Handler) handleStart(ctx context.Context, u *domain.User, chatID int64) {
	welcome := fmt.Sprintf(
		"Привет, %s! 🚀 Я твой проактивный персональный ИИ-планировщик.\n\n"+
			"Что ты можешь делать:\n"+
			"• Писать задачи обычным языком (или слать голосовые!)\n"+
			"• Делиться привычками и графиком («магазины работают до 23:00», «я сова»)\n"+
			"• Я сам оцениваю приоритеты, напоминаю заранее и слежу за ограничениями дня.\n\n"+
			"Команды:\n"+
			"/tasks — список открытых задач\n"+
			"/brief — сводка планов на сегодня\n"+
			"/help — помощь\n\n"+
			"Просто пришли первую задачу текстом или голосом!",
		u.FirstName,
	)
	_ = h.tgClient.SendMessage(ctx, chatID, welcome, nil)
}

func (h *Handler) handleHelp(ctx context.Context, chatID int64) {
	text := "💡 Примеры того, что можно мне написать:\n\n" +
		"• «Напомни завтра в 14:00 созвониться с тимлидом»\n" +
		"• «Купить корм коту сегодня до вечера»\n" +
		"• «Магазины около дома работают до 23:00» (я сохраню в память!)\n" +
		"• «Перенеси созвон с врачом на пятницу на утро»\n" +
		"• Голосовое сообщение: просто надиктуй задачу на бегу!"
	_ = h.tgClient.SendMessage(ctx, chatID, text, nil)
}

func (h *Handler) handleListTasks(ctx context.Context, u *domain.User, chatID int64) {
	pending := domain.TaskStatusPending
	tasks, err := h.taskRepo.ListByUser(ctx, u.ID, domain.TaskFilter{Status: &pending})
	if err != nil {
		_ = h.tgClient.SendMessage(ctx, chatID, "Не удалось загрузить задачи 😔", nil)
		return
	}

	if len(tasks) == 0 {
		_ = h.tgClient.SendMessage(ctx, chatID, "Активных задач нет! 🎉 Можно отдыхать или добавить новую.", nil)
		return
	}

	var sb strings.Builder
	sb.WriteString("📋 Твои активные задачи:\n\n")
	for i, t := range tasks {
		dueStr := "без дедлайна"
		if t.DueAt != nil {
			dueStr = t.DueAt.In(u.Location()).Format("02.01 15:04")
		}
		fmt.Fprintf(&sb, "%d. [%s] %s\n   ⏰ %s (приоритет: %s)\n",
			i+1, t.Category, t.Title, dueStr, t.Priority)
	}

	_ = h.tgClient.SendMessage(ctx, chatID, sb.String(), nil)
}

func (h *Handler) handleDayBrief(ctx context.Context, u *domain.User, chatID int64) {
	brief, err := h.proactive.GenerateMorningBrief(ctx, u)
	if err != nil {
		_ = h.tgClient.SendMessage(ctx, chatID, "Не удалось составить сводку дня.", nil)
		return
	}
	_ = h.tgClient.SendMessage(ctx, chatID, brief, nil)
}

func (h *Handler) handleVoice(ctx context.Context, u *domain.User, chatID int64, fileID string) {
	_ = h.tgClient.SendMessage(ctx, chatID, "🎙 Слушаю голосовое сообщение...", nil)

	data, err := h.tgClient.DownloadFile(ctx, fileID)
	if err != nil {
		log.Printf("download voice error: %v", err)
		_ = h.tgClient.SendMessage(ctx, chatID, "Не удалось скачать аудиозапись 😔", nil)
		return
	}

	text, err := h.whisper.TranscribeAudio(ctx, data, "voice.oga")
	if err != nil {
		log.Printf("whisper error: %v", err)
		_ = h.tgClient.SendMessage(ctx, chatID, "Не удалось расшифровать аудио. Проверь PLANNER_API_KEY.", nil)
		return
	}

	if text == "" {
		_ = h.tgClient.SendMessage(ctx, chatID, "Не удалось разобрать речь в аудиозаписи.", nil)
		return
	}

	// Отправляем расшифрованный текст пользователю и передаем в агент
	_ = h.tgClient.SendMessage(ctx, chatID, fmt.Sprintf("🗣 «%s»", text), nil)
	h.handleAgentMessage(ctx, u, chatID, text)
}

func (h *Handler) handleAgentMessage(ctx context.Context, u *domain.User, chatID int64, text string) {
	reply, err := h.agent.HandleMessage(ctx, u, text)
	if err != nil {
		log.Printf("agent error: %v", err)
		_ = h.tgClient.SendMessage(ctx, chatID, "Произошла ошибка при обработке сообщения нейросетью.", nil)
		return
	}
	_ = h.tgClient.SendMessage(ctx, chatID, reply, nil)
}

func (h *Handler) handleCallback(ctx context.Context, cb *struct {
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
}) {
	parts := strings.Split(cb.Data, ":")
	action := parts[0]

	switch action {
	case "done":
		if len(parts) < 2 {
			return
		}
		taskID, _ := strconv.ParseInt(parts[1], 10, 64)
		task, err := h.taskRepo.GetByID(ctx, taskID)
		if err == nil && task != nil {
			task.Status = domain.TaskStatusDone
			_ = h.taskRepo.Update(ctx, task)
			_ = h.tgClient.AnswerCallback(ctx, cb.ID, "Задача выполнена! 🎉")
			if cb.Message != nil {
				newText := fmt.Sprintf("✅ Выполнено: «%s»", task.Title)
				_ = h.tgClient.EditMessageText(ctx, cb.Message.Chat.ID, cb.Message.MessageID, newText, nil)
			}
		}

	case "postpone":
		if len(parts) < 3 {
			return
		}
		taskID, _ := strconv.ParseInt(parts[1], 10, 64)
		minutes, _ := strconv.Atoi(parts[2])
		task, err := h.taskRepo.GetByID(ctx, taskID)
		if err == nil && task != nil {
			newDue := time.Now().UTC().Add(time.Duration(minutes) * time.Minute)
			task.DueAt = &newDue
			_ = h.taskRepo.Update(ctx, task)
			_ = h.tgClient.AnswerCallback(ctx, cb.ID, fmt.Sprintf("Отложено на %d минут", minutes))
			if cb.Message != nil {
				newText := fmt.Sprintf("⏳ Задача «%s» отложена на %d мин", task.Title, minutes)
				_ = h.tgClient.EditMessageText(ctx, cb.Message.Chat.ID, cb.Message.MessageID, newText, nil)
			}
		}

	case "tomorrow":
		if len(parts) < 2 {
			return
		}
		taskID, _ := strconv.ParseInt(parts[1], 10, 64)
		task, err := h.taskRepo.GetByID(ctx, taskID)
		if err == nil && task != nil {
			loc := time.UTC
			now := time.Now().In(loc)
			tomorrow10 := time.Date(now.Year(), now.Month(), now.Day()+1, 10, 0, 0, 0, loc)
			task.DueAt = &tomorrow10
			_ = h.taskRepo.Update(ctx, task)
			_ = h.tgClient.AnswerCallback(ctx, cb.ID, "Перенесено на завтра на 10:00")
			if cb.Message != nil {
				newText := fmt.Sprintf("🌙 Задача «%s» перенесена на завтра 10:00", task.Title)
				_ = h.tgClient.EditMessageText(ctx, cb.Message.Chat.ID, cb.Message.MessageID, newText, nil)
			}
		}
	}
}
