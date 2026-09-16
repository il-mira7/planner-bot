package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"planner-bot/internal/domain"
)

type UserRepo struct {
	db *sql.DB
}

func NewUserRepo(db *sql.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) Create(ctx context.Context, u *domain.User) error {
	query := `
	INSERT INTO users (telegram_id, username, first_name, timezone, quiet_hours_start, quiet_hours_end, onboarding_completed, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	now := time.Now().UTC()
	if u.Timezone == "" {
		u.Timezone = "Europe/Moscow"
	}
	if u.QuietHoursStart == 0 && u.QuietHoursEnd == 0 {
		u.QuietHoursStart = 23
		u.QuietHoursEnd = 8
	}
	u.CreatedAt = now
	u.UpdatedAt = now

	res, err := r.db.ExecContext(ctx, query,
		u.TelegramID, u.Username, u.FirstName, u.Timezone,
		u.QuietHoursStart, u.QuietHoursEnd, u.OnboardingCompleted,
		u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	u.ID = id
	return nil
}

func (r *UserRepo) GetByTelegramID(ctx context.Context, telegramID int64) (*domain.User, error) {
	query := `
	SELECT id, telegram_id, username, first_name, timezone, quiet_hours_start, quiet_hours_end, onboarding_completed, created_at, updated_at
	FROM users
	WHERE telegram_id = ?
	`
	var u domain.User
	err := r.db.QueryRowContext(ctx, query, telegramID).Scan(
		&u.ID, &u.TelegramID, &u.Username, &u.FirstName, &u.Timezone,
		&u.QuietHoursStart, &u.QuietHoursEnd, &u.OnboardingCompleted,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // Пользователь не найден
	}
	if err != nil {
		return nil, fmt.Errorf("get user by tg id: %w", err)
	}
	return &u, nil
}

func (r *UserRepo) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	query := `
	SELECT id, telegram_id, username, first_name, timezone, quiet_hours_start, quiet_hours_end, onboarding_completed, created_at, updated_at
	FROM users
	WHERE id = ?
	`
	var u domain.User
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&u.ID, &u.TelegramID, &u.Username, &u.FirstName, &u.Timezone,
		&u.QuietHoursStart, &u.QuietHoursEnd, &u.OnboardingCompleted,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &u, nil
}

func (r *UserRepo) Update(ctx context.Context, u *domain.User) error {
	query := `
	UPDATE users
	SET username = ?, first_name = ?, timezone = ?, quiet_hours_start = ?, quiet_hours_end = ?, onboarding_completed = ?, updated_at = ?
	WHERE id = ?
	`
	u.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, query,
		u.Username, u.FirstName, u.Timezone,
		u.QuietHoursStart, u.QuietHoursEnd, u.OnboardingCompleted,
		u.UpdatedAt, u.ID,
	)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

func (r *UserRepo) ListAll(ctx context.Context) ([]domain.User, error) {
	query := `
	SELECT id, telegram_id, username, first_name, timezone, quiet_hours_start, quiet_hours_end, onboarding_completed, created_at, updated_at
	FROM users
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list all users: %w", err)
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(
			&u.ID, &u.TelegramID, &u.Username, &u.FirstName, &u.Timezone,
			&u.QuietHoursStart, &u.QuietHoursEnd, &u.OnboardingCompleted,
			&u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
