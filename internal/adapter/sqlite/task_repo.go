package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"planner-bot/internal/domain"
)

type TaskRepo struct {
	db *sql.DB
}

func NewTaskRepo(db *sql.DB) *TaskRepo {
	return &TaskRepo{db: db}
}

func (r *TaskRepo) Create(ctx context.Context, t *domain.Task) error {
	query := `
	INSERT INTO tasks (
		user_id, title, description, category, priority,
		due_at, scheduled_at, duration_minutes, constraints,
		status, created_at, updated_at, completed_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now
	if t.Status == "" {
		t.Status = domain.TaskStatusPending
	}
	if t.Priority == "" {
		t.Priority = domain.TaskPriorityMedium
	}
	if t.Category == "" {
		t.Category = domain.TaskCategoryPersonal
	}

	constraintsJSON, err := json.Marshal(t.Constraints)
	if err != nil {
		constraintsJSON = []byte("[]")
	}

	res, err := r.db.ExecContext(ctx, query,
		t.UserID, t.Title, t.Description, string(t.Category), string(t.Priority),
		t.DueAt, t.ScheduledAt, t.DurationMinutes, string(constraintsJSON),
		string(t.Status), t.CreatedAt, t.UpdatedAt, t.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("create task: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	t.ID = id
	return nil
}

func (r *TaskRepo) GetByID(ctx context.Context, id int64) (*domain.Task, error) {
	query := `
	SELECT id, user_id, title, description, category, priority,
	       due_at, scheduled_at, duration_minutes, constraints,
	       status, created_at, updated_at, completed_at
	FROM tasks
	WHERE id = ?
	`
	var t domain.Task
	var constraintsRaw string
	var cat, prio, status string
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&t.ID, &t.UserID, &t.Title, &t.Description, &cat, &prio,
		&t.DueAt, &t.ScheduledAt, &t.DurationMinutes, &constraintsRaw,
		&status, &t.CreatedAt, &t.UpdatedAt, &t.CompletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get task by id: %w", err)
	}
	t.Category = domain.TaskCategory(cat)
	t.Priority = domain.TaskPriority(prio)
	t.Status = domain.TaskStatus(status)
	_ = json.Unmarshal([]byte(constraintsRaw), &t.Constraints)
	return &t, nil
}

func (r *TaskRepo) Update(ctx context.Context, t *domain.Task) error {
	query := `
	UPDATE tasks
	SET title = ?, description = ?, category = ?, priority = ?,
	    due_at = ?, scheduled_at = ?, duration_minutes = ?, constraints = ?,
	    status = ?, updated_at = ?, completed_at = ?
	WHERE id = ? AND user_id = ?
	`
	t.UpdatedAt = time.Now().UTC()
	if t.Status == domain.TaskStatusDone && t.CompletedAt == nil {
		now := time.Now().UTC()
		t.CompletedAt = &now
	}

	constraintsJSON, err := json.Marshal(t.Constraints)
	if err != nil {
		constraintsJSON = []byte("[]")
	}

	_, err = r.db.ExecContext(ctx, query,
		t.Title, t.Description, string(t.Category), string(t.Priority),
		t.DueAt, t.ScheduledAt, t.DurationMinutes, string(constraintsJSON),
		string(t.Status), t.UpdatedAt, t.CompletedAt,
		t.ID, t.UserID,
	)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	return nil
}

func (r *TaskRepo) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM tasks WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *TaskRepo) ListByUser(ctx context.Context, userID int64, filter domain.TaskFilter) ([]domain.Task, error) {
	var sb strings.Builder
	sb.WriteString(`
	SELECT id, user_id, title, description, category, priority,
	       due_at, scheduled_at, duration_minutes, constraints,
	       status, created_at, updated_at, completed_at
	FROM tasks
	WHERE user_id = ?
	`)
	args := []any{userID}

	if filter.Status != nil {
		sb.WriteString(" AND status = ?")
		args = append(args, string(*filter.Status))
	}
	if filter.Category != nil {
		sb.WriteString(" AND category = ?")
		args = append(args, string(*filter.Category))
	}
	if filter.DueBefore != nil {
		sb.WriteString(" AND due_at IS NOT NULL AND due_at <= ?")
		args = append(args, *filter.DueBefore)
	}
	if filter.DueAfter != nil {
		sb.WriteString(" AND due_at IS NOT NULL AND due_at >= ?")
		args = append(args, *filter.DueAfter)
	}

	sb.WriteString(" ORDER BY CASE WHEN due_at IS NULL THEN 1 ELSE 0 END, due_at ASC, id DESC")

	rows, err := r.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks by user: %w", err)
	}
	defer rows.Close()

	var tasks []domain.Task
	for rows.Next() {
		var t domain.Task
		var constraintsRaw string
		var cat, prio, status string
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.Title, &t.Description, &cat, &prio,
			&t.DueAt, &t.ScheduledAt, &t.DurationMinutes, &constraintsRaw,
			&status, &t.CreatedAt, &t.UpdatedAt, &t.CompletedAt,
		); err != nil {
			return nil, err
		}
		t.Category = domain.TaskCategory(cat)
		t.Priority = domain.TaskPriority(prio)
		t.Status = domain.TaskStatus(status)
		_ = json.Unmarshal([]byte(constraintsRaw), &t.Constraints)
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func (r *TaskRepo) ListPendingDue(ctx context.Context, before time.Time) ([]domain.Task, error) {
	query := `
	SELECT id, user_id, title, description, category, priority,
	       due_at, scheduled_at, duration_minutes, constraints,
	       status, created_at, updated_at, completed_at
	FROM tasks
	WHERE status = 'pending' AND due_at IS NOT NULL AND due_at <= ?
	ORDER BY due_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, before)
	if err != nil {
		return nil, fmt.Errorf("list pending due tasks: %w", err)
	}
	defer rows.Close()

	var tasks []domain.Task
	for rows.Next() {
		var t domain.Task
		var constraintsRaw string
		var cat, prio, status string
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.Title, &t.Description, &cat, &prio,
			&t.DueAt, &t.ScheduledAt, &t.DurationMinutes, &constraintsRaw,
			&status, &t.CreatedAt, &t.UpdatedAt, &t.CompletedAt,
		); err != nil {
			return nil, err
		}
		t.Category = domain.TaskCategory(cat)
		t.Priority = domain.TaskPriority(prio)
		t.Status = domain.TaskStatus(status)
		_ = json.Unmarshal([]byte(constraintsRaw), &t.Constraints)
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}
