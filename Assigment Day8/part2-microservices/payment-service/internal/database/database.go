package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func Connect(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

	var lastErr error
	for i := 0; i < 30; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		lastErr = db.PingContext(ctx)
		cancel()
		if lastErr == nil {
			return db, nil
		}
		log.Printf("waiting for database... (%v)", lastErr)
		time.Sleep(time.Second)
	}
	return nil, fmt.Errorf("database not reachable: %w", lastErr)
}

// payment-service owns its own database; it never touches core-service tables.
var schema = []string{
	`CREATE TABLE IF NOT EXISTS payment_sessions (
		id           UUID PRIMARY KEY,
		user_id      UUID NOT NULL,
		merchant_id  TEXT NOT NULL,
		description  TEXT,
		amount       NUMERIC(18,2) NOT NULL CHECK (amount > 0),
		status       TEXT NOT NULL CHECK (status IN ('pending','completed','failed','expired')),
		expires_at   TIMESTAMPTZ NOT NULL,
		created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
		completed_at TIMESTAMPTZ
	)`,
	`CREATE INDEX IF NOT EXISTS idx_ps_user ON payment_sessions (user_id, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_ps_pending ON payment_sessions (expires_at) WHERE status = 'pending'`,
}

func Migrate(db *sql.DB) error {
	for _, stmt := range schema {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}
