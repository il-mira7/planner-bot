package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Open открывает базу данных SQLite и выполняет миграцию схемы.
func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", dsn, err)
	}

	db.SetMaxOpenConns(1) // Для SQLite с WAL рекомендуется 1 писатель, предотвращает блокировки
	db.SetConnMaxLifetime(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	if err := migrate(ctx, db); err != nil {
		return nil, fmt.Errorf("migrate schema: %w", err)
	}

	return db, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	pragmas := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA journal_mode = WAL;",
		"PRAGMA busy_timeout = 5000;",
	}
	for _, p := range pragmas {
		if _, err := db.ExecContext(ctx, p); err != nil {
			return fmt.Errorf("exec %s: %w", p, err)
		}
	}

	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		telegram_id INTEGER NOT NULL UNIQUE,
		username TEXT NOT NULL DEFAULT '',
		first_name TEXT NOT NULL DEFAULT '',
		timezone TEXT NOT NULL DEFAULT 'Europe/Moscow',
		quiet_hours_start INTEGER NOT NULL DEFAULT 23,
		quiet_hours_end INTEGER NOT NULL DEFAULT 8,
		onboarding_completed BOOLEAN NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		category TEXT NOT NULL DEFAULT 'personal',
		priority TEXT NOT NULL DEFAULT 'medium',
		due_at DATETIME,
		scheduled_at DATETIME,
		duration_minutes INTEGER NOT NULL DEFAULT 0,
		constraints TEXT NOT NULL DEFAULT '[]', -- JSON array of strings
		status TEXT NOT NULL DEFAULT 'pending',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		completed_at DATETIME
	);
	CREATE INDEX IF NOT EXISTS idx_tasks_user_status ON tasks(user_id, status);
	CREATE INDEX IF NOT EXISTS idx_tasks_due ON tasks(due_at);

	CREATE TABLE IF NOT EXISTS user_facts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		category TEXT NOT NULL DEFAULT 'habit',
		key TEXT NOT NULL,
		value TEXT NOT NULL,
		source TEXT NOT NULL DEFAULT 'conversation',
		confidence REAL NOT NULL DEFAULT 1.0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(user_id, key)
	);
	CREATE INDEX IF NOT EXISTS idx_facts_user ON user_facts(user_id);

	CREATE TABLE IF NOT EXISTS reminders (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		task_id INTEGER REFERENCES tasks(id) ON DELETE CASCADE,
		type TEXT NOT NULL,
		trigger_at DATETIME NOT NULL,
		sent_at DATETIME,
		message TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_reminders_trigger ON reminders(trigger_at, sent_at);
	`

	_, err := db.ExecContext(ctx, schema)
	return err
}
