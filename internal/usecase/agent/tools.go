package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"planner-bot/internal/domain"
)

func GetAgentTools() []domain.LLMTool {
	return []domain.LLMTool{
		{
			Type: "function",
			Function: domain.LLMFunctionDef{
				Name:        "create_task",
				Description: "Создает новую задачу или событие для пользователя с распознанным дедлайном, категорией и ограничениями.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"title": map[string]any{
							"type":        "string",
							"description": "Краткое название задачи или действия (например: 'Сдать отчет', 'Купить продукты')",
						},
						"category": map[string]any{
							"type":        "string",
							"enum":        []string{"work", "personal", "shopping", "health", "study", "other"},
							"description": "Категория задачи",
						},
						"priority": map[string]any{
							"type":        "string",
							"enum":        []string{"low", "medium", "high", "urgent"},
							"description": "Приоритет задачи",
						},
						"due_at": map[string]any{
							"type":        "string",
							"description": "Дедлайн в формате ISO 8601 (YYYY-MM-DDTHH:MM:SS), если указан",
						},
						"scheduled_at": map[string]any{
							"type":        "string",
							"description": "Запланированное время начала (например, для созвона или визита к врачу) в ISO 8601",
						},
						"duration_minutes": map[string]any{
							"type":        "integer",
							"description": "Оценочная продолжительность в минутах (например 30, 60)",
						},
						"constraints": map[string]any{
							"type":        "array",
							"items":       map[string]any{"type": "string"},
							"description": "Внешние условия или ограничения (например: ['магазины до 22:00', 'нужен ноутбук'])",
						},
					},
					"required": []string{"title", "category", "priority"},
				},
			},
		},
		{
			Type: "function",
			Function: domain.LLMFunctionDef{
				Name:        "reschedule_task",
				Description: "Переносит дедлайн или запланированное время задачи по её номеру или названию.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"task_id": map[string]any{
							"type":        "integer",
							"description": "ID задачи (если известен пользователю или найден в списке)",
						},
						"search_title": map[string]any{
							"type":        "string",
							"description": "Название задачи, если ID не указан",
						},
						"new_due_at": map[string]any{
							"type":        "string",
							"description": "Новый дедлайн в ISO 8601 (YYYY-MM-DDTHH:MM:SS)",
						},
						"reason": map[string]any{
							"type":        "string",
							"description": "Причина переноса, если пользователь её озвучил",
						},
					},
					"required": []string{"new_due_at"},
				},
			},
		},
		{
			Type: "function",
			Function: domain.LLMFunctionDef{
				Name:        "complete_task",
				Description: "Отмечает задачу выполненной.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"task_id": map[string]any{
							"type":        "integer",
							"description": "ID задачи",
						},
						"search_title": map[string]any{
							"type":        "string",
							"description": "Название задачи, если ID не назван",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: domain.LLMFunctionDef{
				Name:        "list_tasks",
				Description: "Возвращает список активных задач пользователя для проверки расписания.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"status": map[string]any{
							"type": "string",
							"enum": []string{"pending", "done"},
						},
						"category": map[string]any{
							"type": "string",
							"enum": []string{"work", "personal", "shopping", "health", "study", "other"},
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: domain.LLMFunctionDef{
				Name:        "save_user_fact",
				Description: "Сохраняет важный факт о пользователе в долговременную память (привычки, часы работы мест, правила, цели).",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"category": map[string]any{
							"type":        "string",
							"enum":        []string{"habit", "place", "goal", "preference", "rule"},
							"description": "Категория факта",
						},
						"key": map[string]any{
							"type":        "string",
							"description": "Краткий идентификатор факта (например: 'supermarket_hours', 'sleep_schedule', 'career_goal')",
						},
						"value": map[string]any{
							"type":        "string",
							"description": "Содержание факта (например: 'магазины у дома работают до 23:00', 'сова, просыпается в 10:00')",
						},
					},
					"required": []string{"category", "key", "value"},
				},
			},
		},
	}
}

// ToolExecutor исполняет вызовы инструментов агента.
type ToolExecutor struct {
	taskRepo domain.TaskRepository
	memRepo  domain.MemoryRepository
}

func NewToolExecutor(taskRepo domain.TaskRepository, memRepo domain.MemoryRepository) *ToolExecutor {
	return &ToolExecutor{taskRepo: taskRepo, memRepo: memRepo}
}

func (e *ToolExecutor) Execute(ctx context.Context, user *domain.User, call domain.LLMToolCall) (string, error) {
	switch call.Function.Name {
	case "create_task":
		var args struct {
			Title           string   `json:"title"`
			Category        string   `json:"category"`
			Priority        string   `json:"priority"`
			DueAt           *string  `json:"due_at"`
			ScheduledAt     *string  `json:"scheduled_at"`
			DurationMinutes int      `json:"duration_minutes"`
			Constraints     []string `json:"constraints"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse create_task args: %w", err)
		}

		t := &domain.Task{
			UserID:          user.ID,
			Title:           args.Title,
			Category:        domain.TaskCategory(args.Category),
			Priority:        domain.TaskPriority(args.Priority),
			DurationMinutes: args.DurationMinutes,
			Constraints:     args.Constraints,
			Status:          domain.TaskStatusPending,
		}

		loc := user.Location()
		if args.DueAt != nil && *args.DueAt != "" {
			if parsed, err := parseFlexTime(*args.DueAt, loc); err == nil {
				t.DueAt = &parsed
			}
		}
		if args.ScheduledAt != nil && *args.ScheduledAt != "" {
			if parsed, err := parseFlexTime(*args.ScheduledAt, loc); err == nil {
				t.ScheduledAt = &parsed
			}
		}

		if err := e.taskRepo.Create(ctx, t); err != nil {
			return "", fmt.Errorf("db create task: %w", err)
		}

		res := map[string]any{
			"status":  "created",
			"task_id": t.ID,
			"title":   t.Title,
			"due_at":  t.DueAt,
		}
		raw, _ := json.Marshal(res)
		return string(raw), nil

	case "reschedule_task":
		var args struct {
			TaskID      int64   `json:"task_id"`
			SearchTitle string  `json:"search_title"`
			NewDueAt    string  `json:"new_due_at"`
			Reason      string  `json:"reason"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse reschedule args: %w", err)
		}

		var targetTask *domain.Task
		if args.TaskID > 0 {
			targetTask, _ = e.taskRepo.GetByID(ctx, args.TaskID)
		} else if args.SearchTitle != "" {
			pending := domain.TaskStatusPending
			tasks, _ := e.taskRepo.ListByUser(ctx, user.ID, domain.TaskFilter{Status: &pending})
			for _, t := range tasks {
				if strings.Contains(strings.ToLower(t.Title), strings.ToLower(args.SearchTitle)) {
					taskCopy := t
					targetTask = &taskCopy
					break
				}
			}
		}

		if targetTask == nil {
			return `{"status": "error", "message": "задача для переноса не найдена"}`, nil
		}

		loc := user.Location()
		parsed, err := parseFlexTime(args.NewDueAt, loc)
		if err != nil {
			return `{"status": "error", "message": "некорректный формат новой даты"}`, nil
		}
		targetTask.DueAt = &parsed
		if err := e.taskRepo.Update(ctx, targetTask); err != nil {
			return "", fmt.Errorf("update task: %w", err)
		}

		return fmt.Sprintf(`{"status": "rescheduled", "task_id": %d, "new_due_at": "%s"}`, targetTask.ID, parsed.Format(time.RFC3339)), nil

	case "complete_task":
		var args struct {
			TaskID      int64  `json:"task_id"`
			SearchTitle string `json:"search_title"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse complete args: %w", err)
		}

		var targetTask *domain.Task
		if args.TaskID > 0 {
			targetTask, _ = e.taskRepo.GetByID(ctx, args.TaskID)
		} else if args.SearchTitle != "" {
			pending := domain.TaskStatusPending
			tasks, _ := e.taskRepo.ListByUser(ctx, user.ID, domain.TaskFilter{Status: &pending})
			for _, t := range tasks {
				if strings.Contains(strings.ToLower(t.Title), strings.ToLower(args.SearchTitle)) {
					taskCopy := t
					targetTask = &taskCopy
					break
				}
			}
		}

		if targetTask == nil {
			return `{"status": "error", "message": "задача не найдена"}`, nil
		}

		targetTask.Status = domain.TaskStatusDone
		if err := e.taskRepo.Update(ctx, targetTask); err != nil {
			return "", fmt.Errorf("complete task: %w", err)
		}
		return fmt.Sprintf(`{"status": "completed", "task_id": %d, "title": "%s"}`, targetTask.ID, targetTask.Title), nil

	case "list_tasks":
		pending := domain.TaskStatusPending
		tasks, err := e.taskRepo.ListByUser(ctx, user.ID, domain.TaskFilter{Status: &pending})
		if err != nil {
			return "", err
		}
		raw, _ := json.Marshal(tasks)
		return string(raw), nil

	case "save_user_fact":
		var args struct {
			Category string `json:"category"`
			Key      string `json:"key"`
			Value    string `json:"value"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse fact args: %w", err)
		}

		fact := &domain.UserFact{
			UserID:     user.ID,
			Category:   domain.FactCategory(args.Category),
			Key:        args.Key,
			Value:      args.Value,
			Source:     "conversation",
			Confidence: 1.0,
		}
		if err := e.memRepo.SaveFact(ctx, fact); err != nil {
			return "", fmt.Errorf("save fact: %w", err)
		}
		return fmt.Sprintf(`{"status": "saved_to_memory", "key": "%s", "value": "%s"}`, fact.Key, fact.Value), nil

	default:
		return "", fmt.Errorf("unknown function: %s", call.Function.Name)
	}
}

func parseFlexTime(s string, loc *time.Location) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, s, loc); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unknown time format: %s", s)
}
