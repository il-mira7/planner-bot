package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	adapterLLM "planner-bot/internal/adapter/llm"
	adapterSQLite "planner-bot/internal/adapter/sqlite"
	adapterTG "planner-bot/internal/adapter/telegram"
	adapterWhisper "planner-bot/internal/adapter/whisper"
	deliveryCron "planner-bot/internal/delivery/cron"
	deliveryTG "planner-bot/internal/delivery/telegram"
	"planner-bot/internal/usecase"
	"planner-bot/internal/usecase/agent"
)

func main() {
	// Настройка красивого структурированного логирования slog (Go 1.21+)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Автоматическая загрузка .env файла (чистый Go, без внешних библиотек)
	loadEnv(".env", ".env.local")

	token := os.Getenv("PLANNER_TOKEN")
	if token == "" {
		slog.Error("PLANNER_TOKEN не задан — укажи токен бота в файле .env")
		os.Exit(1)
	}

	dbPath := envOr("PLANNER_DB_PATH", "planner.db")
	db, err := adapterSQLite.Open(dbPath)
	if err != nil {
		slog.Error("ошибка инициализации базы sqlite", "path", dbPath, "err", err)
		os.Exit(1)
	}
	defer db.Close()

	// Репозитории
	userRepo := adapterSQLite.NewUserRepo(db)
	taskRepo := adapterSQLite.NewTaskRepo(db)
	memRepo := adapterSQLite.NewMemoryRepo(db)
	remRepo := adapterSQLite.NewReminderRepo(db)

	// Адаптеры API
	llmClient := adapterLLM.New(adapterLLM.FromEnv())
	whisperClient := adapterWhisper.New(adapterWhisper.FromEnv())
	tgClient := adapterTG.NewClient(token)

	// Usecase слои
	agentOrchestrator := agent.NewOrchestrator(llmClient, taskRepo, memRepo, userRepo)
	proactiveService := usecase.NewProactiveService(userRepo, taskRepo, memRepo, remRepo, tgClient, llmClient)

	// Telegram контроллер
	allowedUsers := os.Getenv("PLANNER_ALLOWED_USERS")
	if allowedUsers == "" {
		allowedUsers = os.Getenv("PLANNER_OWNER") // обратная совместимость
	}
	handler := deliveryTG.NewHandler(tgClient, userRepo, taskRepo, agentOrchestrator, whisperClient, proactiveService, allowedUsers)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Запуск фонового проактивного планировщика
	go deliveryCron.StartProactiveWorker(ctx, proactiveService, 1*time.Minute)

	slog.Info("🤖 Proactive AI Planner Bot запущен",
		"go_version", "1.27",
		"ai_enabled", llmClient.Enabled(),
		"db_path", dbPath,
	)

	var offset int64
	for {
		updates, err := tgClient.GetUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				slog.Info("Остановка сервиса по сигналу ОС...")
				return
			}
			slog.Error("ошибка получения getUpdates", "err", err)
			time.Sleep(3 * time.Second)
			continue
		}

		for _, u := range updates {
			offset = u.UpdateID + 1
			handler.HandleUpdate(ctx, u)
		}
	}
}

func loadEnv(paths ...string) {
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])
				val = strings.Trim(val, `"'`)
				if os.Getenv(key) == "" && val != "" {
					_ = os.Setenv(key, val)
				}
			}
		}
	}
}

func envOr(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
