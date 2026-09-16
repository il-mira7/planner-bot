package cron

import (
	"context"
	"log"
	"time"

	"planner-bot/internal/usecase"
)

// StartProactiveWorker запускает фоновый тикер, который проверяет дедлайны и проактивные условия.
func StartProactiveWorker(ctx context.Context, proactive *usecase.ProactiveService, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("proactive worker started with interval %v", interval)

	for {
		select {
		case <-ctx.Done():
			log.Println("proactive worker stopped")
			return
		case <-ticker.C:
			proactive.CheckAndNotify(ctx)
		}
	}
}
