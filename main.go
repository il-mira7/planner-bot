// planner-bot — личный Telegram-планировщик на Go: задачи добавляются
// текстом на естественном языке, дедлайны распознаются без ИИ,
// ИИ-агент опционально раскладывает большие задачи на шаги.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"planner-bot/internal/ai"
	"planner-bot/internal/sched"
	"planner-bot/internal/store"
)

func main() {
	token := os.Getenv("PLANNER_TOKEN")
	if token == "" {
		log.Fatal("задай PLANNER_TOKEN — токен бота от @BotFather")
	}
	dataPath := envOr("PLANNER_DATA", "tasks.json")
	st, err := store.Open(dataPath)
	if err != nil {
		log.Fatalf("хранилище %s: %v", dataPath, err)
	}

	var owner int64
	if s := os.Getenv("PLANNER_OWNER"); s != "" {
		if owner, err = strconv.ParseInt(s, 10, 64); err != nil {
			log.Fatal("PLANNER_OWNER — это числовой telegram id (узнать можно у @userinfobot)")
		}
	}

	client := ai.New(ai.FromEnv())
	tg := newTelegram(token)
	a := &app{st: st, ai: client, tg: tg, owner: owner}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go sched.Run(ctx, st,
		func(chatID int64, text string) error {
			c, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			return tg.sendMessage(c, chatID, text)
		},
		func(ctx context.Context, summary string) string {
			if !client.Enabled() {
				return ""
			}
			c, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			advice, err := client.DayAdvice(c, summary)
			if err != nil {
				log.Printf("совет дня не получен: %v", err)
				return ""
			}
			return advice
		})

	var offset int64
	log.Printf("planner-bot запущен (ИИ: %v)", client.Enabled())
	for {
		updates, err := tg.getUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				log.Println("остановлена")
				return
			}
			log.Printf("getUpdates: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}
		for _, u := range updates {
			offset = u.UpdateID + 1
			a.handleUpdate(ctx, u)
		}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
