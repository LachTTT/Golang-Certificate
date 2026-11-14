package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Connect opens the pool and waits (up to ~30s) until PostgreSQL is reachable.
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

var schema = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id            UUID PRIMARY KEY,
		name          TEXT NOT NULL,
		email         TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE TABLE IF NOT EXISTS wallets (
		user_id    UUID PRIMARY KEY REFERENCES users(id),
		balance    NUMERIC(18,2) NOT NULL DEFAULT 0 CHECK (balance >= 0),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE TABLE IF NOT EXISTS transactions (
		id           UUID PRIMARY KEY,
		type         TEXT NOT NULL CHECK (type IN ('topup','transfer','payment')),
		from_user    UUID REFERENCES users(id),
		to_user      UUID REFERENCES users(id),
		merchant_id  TEXT,
		description  TEXT,
		amount       NUMERIC(18,2) NOT NULL CHECK (amount > 0),
		status       TEXT NOT NULL CHECK (status IN ('pending','completed','failed','expired')),
		expires_at   TIMESTAMPTZ,
		created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
		completed_at TIMESTAMPTZ
	)`,
	`CREATE INDEX IF NOT EXISTS idx_tx_from ON transactions (from_user, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_tx_to ON transactions (to_user, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_tx_pending_payment ON transactions (expires_at)
		WHERE type = 'payment' AND status = 'pending'`,
	`CREATE INDEX IF NOT EXISTS idx_tx_pending_transfer ON transactions (created_at)
		WHERE type = 'transfer' AND status = 'pending'`,
}

func Migrate(db *sql.DB) error {
	for _, stmt := range schema {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}
