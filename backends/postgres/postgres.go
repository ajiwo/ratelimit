package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresConfig struct {
	ConnString string
	MaxConns   int32
	MinConns   int32
}

type PostgresStorage struct {
	pool *pgxpool.Pool
}

func NewPostgresStorage(config PostgresConfig) (*PostgresStorage, error) {
	if config.MaxConns == 0 {
		config.MaxConns = 10
	}
	if config.MinConns == 0 {
		config.MinConns = 2
	}

	poolConfig, err := pgxpool.ParseConfig(config.ConnString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	poolConfig.MaxConns = config.MaxConns
	poolConfig.MinConns = config.MinConns

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if err := createTable(context.Background(), pool); err != nil {
		return nil, fmt.Errorf("failed to create table: %w", err)
	}

	return &PostgresStorage{pool: pool}, nil
}

func createTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS ratelimit_kv (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			expires_at TIMESTAMP WITH TIME ZONE
		)
	`)
	return err
}

func (p *PostgresStorage) GetPool() *pgxpool.Pool {
	return p.pool
}

func (p *PostgresStorage) Get(ctx context.Context, key string) (string, error) {
	var value string
	var expiresAt *time.Time

	err := p.pool.QueryRow(ctx, `
		SELECT value, expires_at
		FROM ratelimit_kv
		WHERE key = $1
	`, key).Scan(&value, &expiresAt)

	if err != nil {
		return "", nil
	}

	if expiresAt != nil && time.Now().After(*expiresAt) {
		return "", nil
	}

	return value, nil
}

func (p *PostgresStorage) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	var expiresAt *time.Time
	if expiration > 0 {
		t := time.Now().Add(expiration)
		expiresAt = &t
	}

	valueStr := fmt.Sprintf("%v", value)

	_, err := p.pool.Exec(ctx, `
		INSERT INTO ratelimit_kv (key, value, expires_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE SET
			value = EXCLUDED.value,
			expires_at = EXCLUDED.expires_at
	`, key, valueStr, expiresAt)

	return err
}

func (p *PostgresStorage) Delete(ctx context.Context, key string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM ratelimit_kv WHERE key = $1`, key)
	return err
}

func (p *PostgresStorage) Close() error {
	if p.pool != nil {
		p.pool.Close()
	}
	return nil
}
