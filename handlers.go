package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"planner-bot/internal/ai"
	"planner-bot/internal/parse"
	"planner-bot/internal/sched"
	"planner-bot/internal/store"
)

type app struct {
	st    *store.Store
	ai    *ai.Client
	tg    *telegram
	owner int64 // telegram user id из PLANNER_OWNER; 0 — не ограничивать
}

func (a *app) handleUpdate(ctx context.Context, u Update) {
	msg := u.Message
	if msg == nil {
		return
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	var fromID int64
	if msg.From != nil {
		fromID = msg.From.ID
	}
	if a.owner != 0 && fromID != a.owner {
		a.reply(ctx, msg.Chat.ID, "Это личный бот планирования 🙂")
		return
	}
	a.st.SetChat(msg.Chat.ID)

	cmd, rest := splitCommand(text)
	switch cmd {
	case "/start", "/help":
		a.reply(ctx, msg.Chat.ID, helpText)
	case "/add":
		a.cmdAdd(ctx, msg.Chat.ID, rest)
	case "/list", "/tasks":
		a.reply(ctx, msg.Chat.ID, listText(a.st))
	case "/done":
		a.cmdDone(ctx, msg.Chat.ID, rest)
	case "/del", "/delete":
		a.cmdDel(ctx, msg.Chat.ID, rest)
	case "/plan":
		a.cmdPlan(ctx, msg.Chat.ID, rest)
	case "/day", "/today":
		a.cmdDay(ctx, msg.Chat.ID)
	case "":
		a.cmdAdd(ctx, msg.Chat.ID, text) // любое сообщение — быстрое добавление задачи
	default:
		a.reply(ctx, msg.Chat.ID, "Не знаю такую команду. /help — что я умею")
	}
}

func (a *app) reply(ctx context.Context, chatID int64, text string) {
	if err := a.tg.sendMessage(ctx, chatID, text); err != nil {
		log.Printf("sendMessage: %v", err)
	}
}

func (a *app) cmdAdd(ctx context.Context, chatID int64, text string) {
	if text == "" {
		a.reply(ctx, chatID, "Что добавить? Пример: /add сдать отчёт завтра в 18")
		return
	}
	due, cleaned, ok := parse.ExtractDue(text, time.Now())
	var dp *time.Time
	if ok {
		dp = &due
	}
	t := a.st.Add(cleaned, dp)
	if dp != nil {
		a.reply(ctx, chatID, fmt.Sprintf("📌 Добавлено: «%s»\n⏰ Дедлайн: %s", t.Text, due.Format("02.01 15:04")))
	} else {
		a.reply(ctx, chatID, fmt.Sprintf("📌 Добавлено: «%s» (без дедлайна)", t.Text))
	}
}

func (a *app) cmdDone(ctx context.Context, chatID int64, rest string) {
	id, ok := taskID(rest)
	if !ok {
		a.reply(ctx, chatID, "Формат: /done номер задачи, например /done 3")
		return
	}
	t, err := a.st.Done(id)
	if err != nil {
		a.reply(ctx, chatID, fmt.Sprintf("Задачи №%d не нашлось, /list покажет номера", id))
		return
	}
	a.reply(ctx, chatID, fmt.Sprintf("✅ Готово: «%s»", t.Text))
}

func (a *app) cmdDel(ctx context.Context, chatID int64, rest string) {
	id, ok := taskID(rest)
	if !ok {
		a.reply(ctx, chatID, "Формат: /del номер задачи, например /del 3")
		return
	}
	if err := a.st.Delete(id); err != nil {
		a.reply(ctx, chatID, fmt.Sprintf("Задачи №%d не нашлось, /list покажет номера", id))
		return
	}
	a.reply(ctx, chatID, fmt.Sprintf("🗑 Убрала задачу №%d", id))
}

// taskID достаёт номер задачи из аргумента команды.
func taskID(rest string) (int, bool) {
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return 0, false
	}
	id, err := strconv.Atoi(fields[0])
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func (a *app) cmdPlan(ctx context.Context, chatID int64, rest string) {
	if rest == "" {
		a.reply(ctx, chatID, "Какую задачу раскладываем? Пример: /plan добить проект ShopAPI")
		return
	}
	if !a.ai.Enabled() {
		a.cmdAdd(ctx, chatID, rest)
		a.reply(ctx, chatID, "Ключ ИИ не задан — добавила как обычную задачу (раскладка на шаги появится с PLANNER_API_KEY)")
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	steps, err := a.ai.Breakdown(ctx, rest)
	if err != nil {
		log.Printf("breakdown: %v", err)
		a.cmdAdd(ctx, chatID, rest)
		a.reply(ctx, chatID, "ИИ не разложил задачу, добавила как есть 🙃")
		return
	}
	for _, s := range steps {
		a.st.Add(fmt.Sprintf("» %s", s), nil)
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🧩 Разложила на %d шагов:\n", len(steps)))
	for i, s := range steps {
		fmt.Fprintf(&b, "%d. %s\n", i+1, s)
	}
	a.reply(ctx, chatID, b.String())
}

func (a *app) cmdDay(ctx context.Context, chatID int64) {
	summary := sched.DaySummary(a.st, time.Now())
	text := "📋 " + summary
	if a.ai.Enabled() {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if advice, err := a.ai.DayAdvice(ctx, summary); err == nil && advice != "" {
			text += "\n\n💡 " + advice
		} else if err != nil {
			log.Printf("day advice: %v", err)
		}
	}
	a.reply(ctx, chatID, text)
}

// splitCommand выделяет команду ("/done@my_bot 3" → "/done", "3").
func splitCommand(t string) (string, string) {
	if !strings.HasPrefix(t, "/") {
		return "", t
	}
	parts := strings.SplitN(t, " ", 2)
	cmd := strings.ToLower(parts[0])
	if i := strings.Index(cmd, "@"); i > 0 {
		cmd = cmd[:i]
	}
	rest := ""
	if len(parts) == 2 {
		rest = strings.TrimSpace(parts[1])
	}
	return cmd, rest
}

func listText(st *store.Store) string {
	active := st.Active()
	if len(active) == 0 {
		return "Задач нет 🎉 Просто пришли текст — я добавлю (дедлайн распознаю сама)"
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	var overdue, dueToday, later, noDue []store.Task
	for _, t := range active {
		switch {
		case t.Due == nil:
			noDue = append(noDue, t)
		case t.Due.Before(today):
			overdue = append(overdue, t)
		case t.Due.Before(today.AddDate(0, 0, 1)):
			dueToday = append(dueToday, t)
		default:
			later = append(later, t)
		}
	}

	var b strings.Builder
	section := func(title string, ts []store.Task, withDue bool) {
		if len(ts) == 0 {
			return
		}
		fmt.Fprintf(&b, "\n%s\n", title)
		for _, t := range ts {
			if withDue {
				fmt.Fprintf(&b, "%d. %s — %s\n", t.ID, t.Text, t.Due.Format("02.01 15:04"))
			} else {
				fmt.Fprintf(&b, "%d. %s\n", t.ID, t.Text)
			}
		}
	}
	section("🔴 Просрочено:", overdue, true)
	section("🟡 Сегодня:", dueToday, true)
	section("🔵 Дальше:", later, true)
	section("📌 Без дедлайна:", noDue, false)
	b.WriteString("\n/done N — закрыть, /del N — убрать")
	return strings.TrimSpace(b.String())
}

const helpText = `Я твой планировщик. Просто пришли задачу текстом — дедлайн распознаю сама.

Примеры:
сдать тестовое завтра в 18
созвон 15.10 в 14:30
полить цветы через 2 дня
позвонить в банк в 10

Команды:
/list — все задачи
/done N — закрыть задачу №N
/del N — удалить задачу №N
/plan большая задача — разложу на шаги (нужен ключ ИИ)
/day — план на день
`
