package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	ConnString string
	MaxConns   int32
	MinConns   int32
}

type Backend struct {
	pool *pgxpool.Pool
}

// New initializes a new PostgresStorage with the given configuration.
func New(config Config) (*Backend, error) {
	if config.MaxConns == 0 {
		config.MaxConns = 10
	}
	if config.MinConns == 0 {
		config.MinConns = 2
	}

	poolConfig, err := pgxpool.ParseConfig(config.ConnString)
	if err != nil {
		return nil, NewInvalidConnStringError(err)
	}

	poolConfig.MaxConns = config.MaxConns
	poolConfig.MinConns = config.MinConns

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, NewPoolCreationFailedError(err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		return nil, NewPingFailedError(err)
	}

	if err := createTable(context.Background(), pool); err != nil {
		return nil, NewTableCreationFailedError(err)
	}

	return &Backend{pool: pool}, nil
}

// NewWithClient initializes a new PostgresBackend with a pre-configured connection pool.
//
// The pool is assumed to be already connected and ready for use.
func NewWithClient(pool *pgxpool.Pool) *Backend {
	return &Backend{pool: pool}
}

func createTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS ratelimit_kv (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			expires_at TIMESTAMP WITH TIME ZONE
		)
	`)
	if err != nil {
		return NewTableQueryFailedError("CREATE TABLE", err)
	}
	return nil
}

func (p *Backend) GetPool() *pgxpool.Pool {
	return p.pool
}

func (p *Backend) Get(ctx context.Context, key string) (string, error) {
	var value string
	var expiresAt *time.Time

	err := p.pool.QueryRow(ctx, `
		SELECT value, expires_at
		FROM ratelimit_kv
		WHERE key = $1
	`, key).Scan(&value, &expiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", NewGetFailedError(key, err)
	}

	if expiresAt != nil && time.Now().After(*expiresAt) {
		return "", nil
	}

	return value, nil
}

func (p *Backend) Set(ctx context.Context, key string, value string, expiration time.Duration) error {
	var expiresAt *time.Time
	if expiration > 0 {
		t := time.Now().Add(expiration)
		expiresAt = &t
	}

	_, err := p.pool.Exec(ctx, `
		INSERT INTO ratelimit_kv (key, value, expires_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE SET
			value = EXCLUDED.value,
			expires_at = EXCLUDED.expires_at
	`, key, value, expiresAt)
	if err != nil {
		return NewSetFailedError(key, err)
	}
	return nil
}

func (p *Backend) Delete(ctx context.Context, key string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM ratelimit_kv WHERE key = $1`, key)
	if err != nil {
		return NewDeleteFailedError(key, err)
	}
	return nil
}

func (p *Backend) Close() error {
	if p.pool != nil {
		p.pool.Close()
	}
	return nil
}

// CheckAndSet atomically sets key to newValue only if current value matches oldValue.
// This operation provides compare-and-swap semantics for implementing optimistic locking.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - key: The storage key to operate on
//   - oldValue: Expected current value. Use empty string "" for "set if not exists" semantics
//   - newValue: New value to set if the current value matches oldValue
//   - expiration: Time-to-live for the key. Use 0 for no expiration
//
// Returns:
//   - bool: true if the operation succeeded (value was set), false otherwise
//   - error: Any storage-related error (not including failed comparison)
//
// Behavior:
//   - If oldValue is "", the operation succeeds only if the key doesn't exist
//   - If oldValue matches the current value, the key is updated to newValue
//   - Expired keys are treated as non-existent for comparison purposes
//   - All values are stored and compared as strings
func (p *Backend) CheckAndSet(ctx context.Context, key, oldValue, newValue string, expiration time.Duration) (bool, error) {
	var expiresAt *time.Time
	if expiration > 0 {
		t := time.Now().Add(expiration)
		expiresAt = &t
	}

	if oldValue == "" {
		// First, delete any expired entries for this key
		_, err := p.pool.Exec(ctx, `
			DELETE FROM ratelimit_kv
			WHERE key = $1 AND expires_at IS NOT NULL AND expires_at <= NOW()
		`, key)
		if err != nil {
			return false, NewCheckAndSetFailedError(key, err)
		}

		// Insert new key only if it doesn't exist (atomic operation)
		result, err := p.pool.Exec(ctx, `
			INSERT INTO ratelimit_kv (key, value, expires_at)
			VALUES ($1, $2, $3)
			ON CONFLICT (key) DO NOTHING
		`, key, newValue, expiresAt)
		if err != nil {
			return false, NewCheckAndSetFailedError(key, err)
		}

		// Return true if a row was inserted, false if key already existed
		return result.RowsAffected() > 0, nil
	}

	// Update only if current value matches oldValue
	result, err := p.pool.Exec(ctx, `
		UPDATE ratelimit_kv
		SET value = $1, expires_at = $2
		WHERE key = $3
			AND value = $4
			AND (expires_at IS NULL OR expires_at > NOW())
	`, newValue, expiresAt, key, oldValue)
	if err != nil {
		return false, NewCheckAndSetFailedError(key, err)
	}

	return result.RowsAffected() == 1, nil
}
