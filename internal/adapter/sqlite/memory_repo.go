package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"planner-bot/internal/domain"
)

type MemoryRepo struct {
	db *sql.DB
}

func NewMemoryRepo(db *sql.DB) *MemoryRepo {
	return &MemoryRepo{db: db}
}

func (r *MemoryRepo) SaveFact(ctx context.Context, f *domain.UserFact) error {
	query := `
	INSERT INTO user_facts (user_id, category, key, value, source, confidence, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(user_id, key) DO UPDATE SET
		category = excluded.category,
		value = excluded.value,
		source = excluded.source,
		confidence = excluded.confidence,
		updated_at = excluded.updated_at
	`
	now := time.Now().UTC()
	f.CreatedAt = now
	f.UpdatedAt = now
	if f.Confidence == 0 {
		f.Confidence = 1.0
	}
	if f.Source == "" {
		f.Source = "conversation"
	}
	if f.Category == "" {
		f.Category = domain.FactCategoryHabit
	}

	res, err := r.db.ExecContext(ctx, query,
		f.UserID, string(f.Category), f.Key, f.Value, f.Source, f.Confidence,
		f.CreatedAt, f.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("save fact: %w", err)
	}

	id, err := res.LastInsertId()
	if err == nil && id > 0 {
		f.ID = id
	}
	return nil
}

func (r *MemoryRepo) ListFactsByUser(ctx context.Context, userID int64, category *domain.FactCategory) ([]domain.UserFact, error) {
	var sb strings.Builder
	sb.WriteString(`
	SELECT id, user_id, category, key, value, source, confidence, created_at, updated_at
	FROM user_facts
	WHERE user_id = ?
	`)
	args := []any{userID}

	if category != nil {
		sb.WriteString(" AND category = ?")
		args = append(args, string(*category))
	}
	sb.WriteString(" ORDER BY updated_at DESC")

	rows, err := r.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("list facts by user: %w", err)
	}
	defer rows.Close()

	var facts []domain.UserFact
	for rows.Next() {
		var f domain.UserFact
		var cat string
		if err := rows.Scan(
			&f.ID, &f.UserID, &cat, &f.Key, &f.Value, &f.Source, &f.Confidence,
			&f.CreatedAt, &f.UpdatedAt,
		); err != nil {
			return nil, err
		}
		f.Category = domain.FactCategory(cat)
		facts = append(facts, f)
	}
	return facts, rows.Err()
}

func (r *MemoryRepo) DeleteFact(ctx context.Context, userID int64, key string) error {
	query := `DELETE FROM user_facts WHERE user_id = ? AND key = ?`
	_, err := r.db.ExecContext(ctx, query, userID, key)
	return err
}
