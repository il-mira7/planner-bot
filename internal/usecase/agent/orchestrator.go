package agent

import (
	"context"
	"fmt"
	"log"
	"time"

	"planner-bot/internal/adapter/llm"
	"planner-bot/internal/domain"
)

type Orchestrator struct {
	llm      domain.LLMClient
	taskRepo domain.TaskRepository
	memRepo  domain.MemoryRepository
	userRepo domain.UserRepository
	executor *ToolExecutor
	tools    []domain.LLMTool
}

func NewOrchestrator(
	llmClient domain.LLMClient,
	taskRepo domain.TaskRepository,
	memRepo domain.MemoryRepository,
	userRepo domain.UserRepository,
) *Orchestrator {
	return &Orchestrator{
		llm:      llmClient,
		taskRepo: taskRepo,
		memRepo:  memRepo,
		userRepo: userRepo,
		executor: NewToolExecutor(taskRepo, memRepo),
		tools:    GetAgentTools(),
	}
}

// HandleMessage обрабатывает входящее сообщение пользователя через агентный цикл ReAct.
func (o *Orchestrator) HandleMessage(ctx context.Context, user *domain.User, userText string) (string, error) {
	now := time.Now()

	// 1. Загружаем долговременную память пользователя
	facts, err := o.memRepo.ListFactsByUser(ctx, user.ID, nil)
	if err != nil {
		log.Printf("failed to load user facts: %v", err)
	}

	// 2. Строим системный контекст
	systemPrompt := llm.BuildAgentSystemPrompt(user, facts, now)

	messages := []domain.LLMMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userText},
	}

	// 3. Агентный цикл (до 4 итераций вызова инструментов)
	const maxIterations = 4
	for i := range maxIterations {
		_ = i
		resp, err := o.llm.ChatWithTools(ctx, messages, o.tools)
		if err != nil {
			return "", fmt.Errorf("agent llm error: %w", err)
		}

		// Если модель не вызывала инструменты, возвращаем её финальный ответ
		if len(resp.ToolCalls) == 0 {
			if resp.Content != "" {
				return resp.Content, nil
			}
			return "Принято!", nil
		}

		// Добавляем сообщение ассистента с вызовами инструментов в историю диалога
		assistantMsg := domain.LLMMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
			Raw:       resp.RawMessage,
		}
		messages = append(messages, assistantMsg)

		// Исполняем каждый инструмент и возвращаем результат в историю
		for _, call := range resp.ToolCalls {
			toolResult, execErr := o.executor.Execute(ctx, user, call)
			if execErr != nil {
				toolResult = fmt.Sprintf(`{"status": "error", "error": "%s"}`, execErr.Error())
			}

			toolMsg := domain.LLMMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    toolResult,
			}
			messages = append(messages, toolMsg)
		}
	}

	return "Задачи и информация обработаны.", nil
}
