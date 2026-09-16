package sqlite

import (
	"context"
	"testing"
	"time"

	"planner-bot/internal/domain"
)

func setupTestDB(t *testing.T) (*UserRepo, *TaskRepo, *MemoryRepo, *ReminderRepo) {
	t.Helper()
	// Использование in-memory sqlite для быстрого тестирования
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("setup memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	return NewUserRepo(db), NewTaskRepo(db), NewMemoryRepo(db), NewReminderRepo(db)
}

func TestUserRepo(t *testing.T) {
	ctx := context.Background()
	userRepo, _, _, _ := setupTestDB(t)

	u := &domain.User{
		TelegramID: 12345678,
		Username:   "testuser",
		FirstName:  "Ilmira",
		Timezone:   "Europe/Moscow",
	}

	if err := userRepo.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if u.ID == 0 {
		t.Fatalf("expected non-zero ID")
	}

	found, err := userRepo.GetByTelegramID(ctx, 12345678)
	if err != nil {
		t.Fatalf("get by tg id: %v", err)
	}
	if found == nil || found.Username != "testuser" {
		t.Fatalf("expected user 'testuser', got %+v", found)
	}

	found.Username = "updateduser"
	found.OnboardingCompleted = true
	if err := userRepo.Update(ctx, found); err != nil {
		t.Fatalf("update user: %v", err)
	}

	byID, err := userRepo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if byID.Username != "updateduser" || !byID.OnboardingCompleted {
		t.Fatalf("expected updated user, got %+v", byID)
	}
}

func TestTaskRepo(t *testing.T) {
	ctx := context.Background()
	userRepo, taskRepo, _, _ := setupTestDB(t)

	u := &domain.User{TelegramID: 1001, FirstName: "Alice"}
	if err := userRepo.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}

	due := time.Now().Add(2 * time.Hour).UTC()
	task := &domain.Task{
		UserID:          u.ID,
		Title:           "Купить продукты",
		Category:        domain.TaskCategoryShopping,
		Priority:        domain.TaskPriorityHigh,
		DueAt:           &due,
		DurationMinutes: 45,
		Constraints:     []string{"магазины до 22:00"},
	}

	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.ID == 0 {
		t.Fatalf("expected non-zero task ID")
	}

	loaded, err := taskRepo.GetByID(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if loaded == nil || loaded.Title != task.Title {
		t.Fatalf("expected title %q, got %+v", task.Title, loaded)
	}
	if len(loaded.Constraints) != 1 || loaded.Constraints[0] != "магазины до 22:00" {
		t.Fatalf("expected constraints preserved, got %+v", loaded.Constraints)
	}

	// Проверка фильтрации
	status := domain.TaskStatusPending
	tasks, err := taskRepo.ListByUser(ctx, u.ID, domain.TaskFilter{Status: &status})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 pending task, got %d", len(tasks))
	}

	// Пометка задачи выполненной
	loaded.Status = domain.TaskStatusDone
	if err := taskRepo.Update(ctx, loaded); err != nil {
		t.Fatalf("update task: %v", err)
	}

	tasks, err = taskRepo.ListByUser(ctx, u.ID, domain.TaskFilter{Status: &status})
	if err != nil {
		t.Fatalf("list tasks after done: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected 0 pending tasks, got %d", len(tasks))
	}
}

func TestMemoryRepo(t *testing.T) {
	ctx := context.Background()
	userRepo, _, memRepo, _ := setupTestDB(t)

	u := &domain.User{TelegramID: 2002, FirstName: "Bob"}
	if err := userRepo.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}

	fact := &domain.UserFact{
		UserID:     u.ID,
		Category:   domain.FactCategoryPlace,
		Key:        "supermarket_closing",
		Value:      "23:00",
		Source:     "conversation",
		Confidence: 0.95,
	}

	if err := memRepo.SaveFact(ctx, fact); err != nil {
		t.Fatalf("save fact: %v", err)
	}

	// UPSERT проверка
	factUpdated := &domain.UserFact{
		UserID:     u.ID,
		Category:   domain.FactCategoryPlace,
		Key:        "supermarket_closing",
		Value:      "24:00 (круглосуточный)",
		Source:     "conversation",
		Confidence: 1.0,
	}
	if err := memRepo.SaveFact(ctx, factUpdated); err != nil {
		t.Fatalf("upsert fact: %v", err)
	}

	facts, err := memRepo.ListFactsByUser(ctx, u.ID, nil)
	if err != nil {
		t.Fatalf("list facts: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact due to upsert, got %d", len(facts))
	}
	if facts[0].Value != "24:00 (круглосуточный)" {
		t.Fatalf("expected updated value, got %s", facts[0].Value)
	}
}

func TestReminderRepo(t *testing.T) {
	ctx := context.Background()
	userRepo, _, _, remRepo := setupTestDB(t)

	u := &domain.User{TelegramID: 3003, FirstName: "Charlie"}
	if err := userRepo.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}

	trigger := time.Now().Add(-5 * time.Minute).UTC()
	rem := &domain.Reminder{
		UserID:    u.ID,
		Type:      domain.ReminderTypeProactiveWarning,
		TriggerAt: trigger,
		Message:   "Магазины скоро закроются!",
	}

	if err := remRepo.Create(ctx, rem); err != nil {
		t.Fatalf("create reminder: %v", err)
	}

	pending, err := remRepo.ListPending(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending reminder, got %d", len(pending))
	}

	if err := remRepo.MarkSent(ctx, rem.ID, time.Now().UTC()); err != nil {
		t.Fatalf("mark sent: %v", err)
	}

	pending, err = remRepo.ListPending(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("list pending after mark: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected 0 pending reminders, got %d", len(pending))
	}
}
