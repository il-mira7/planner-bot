package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"planner-bot/internal/domain"
)

type ReminderRepo struct {
	db *sql.DB
}

func NewReminderRepo(db *sql.DB) *ReminderRepo {
	return &ReminderRepo{db: db}
}

func (r *ReminderRepo) Create(ctx context.Context, rem *domain.Reminder) error {
	query := `
	INSERT INTO reminders (user_id, task_id, type, trigger_at, sent_at, message, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	now := time.Now().UTC()
	rem.CreatedAt = now

	res, err := r.db.ExecContext(ctx, query,
		rem.UserID, rem.TaskID, string(rem.Type), rem.TriggerAt, rem.SentAt, rem.Message, rem.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create reminder: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	rem.ID = id
	return nil
}

func (r *ReminderRepo) MarkSent(ctx context.Context, id int64, sentAt time.Time) error {
	query := `UPDATE reminders SET sent_at = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, sentAt.UTC(), id)
	return err
}

func (r *ReminderRepo) ListPending(ctx context.Context, before time.Time) ([]domain.Reminder, error) {
	query := `
	SELECT id, user_id, task_id, type, trigger_at, sent_at, message, created_at
	FROM reminders
	WHERE sent_at IS NULL AND trigger_at <= ?
	ORDER BY trigger_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, before.UTC())
	if err != nil {
		return nil, fmt.Errorf("list pending reminders: %w", err)
	}
	defer rows.Close()

	var list []domain.Reminder
	for rows.Next() {
		var rem domain.Reminder
		var remType string
		if err := rows.Scan(
			&rem.ID, &rem.UserID, &rem.TaskID, &remType, &rem.TriggerAt,
			&rem.SentAt, &rem.Message, &rem.CreatedAt,
		); err != nil {
			return nil, err
		}
		rem.Type = domain.ReminderType(remType)
		list = append(list, rem)
	}
	return list, rows.Err()
}
