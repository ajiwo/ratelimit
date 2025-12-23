package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config holds configuration for creating a PostgreSQL backend.
type Config struct {
	// ConnString is the PostgreSQL connection string.
	//
	// Format: "postgres://username:password@hostname:port/database?sslmode=disable"
	ConnString string
	// MaxConns is the maximum number of connections in the pool.
	//
	// If 0, a sensible default is used.
	MaxConns int32
	// MinConns is the minimum number of connections in the pool.
	//
	// If 0, defaults to 2.
	MinConns int32
	// ConnErrorStrings contains string patterns to identify connectivity-related errors.
	//
	// If nil, the default patterns from connErrorStrings are used.
	// These patterns help distinguish temporary connectivity issues from operational
	// errors like constraint violations.
	ConnErrorStrings []string
}

type Backend struct {
	pool             *pgxpool.Pool
	connErrorStrings []string
}

// New initializes a new PostgresStorage with the given configuration.
func New(config Config) (*Backend, error) {
	if config.MaxConns == 0 {
		config.MaxConns = 10
	}
	if config.MinConns == 0 {
		config.MinConns = 2
	}

	// Use custom patterns if provided, otherwise fall back to defaults
	patterns := config.ConnErrorStrings
	if patterns == nil {
		patterns = connErrorStrings
	}

	poolConfig, err := pgxpool.ParseConfig(config.ConnString)
	if err != nil {
		return nil, backends.MaybeConnError("postgres:ParseConfig",
			fmt.Errorf("invalid postgres connection string: %w", err), patterns)
	}

	poolConfig.MaxConns = config.MaxConns
	poolConfig.MinConns = config.MinConns

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, backends.MaybeConnError("postgres:NewPool",
			fmt.Errorf("failed to create postgres connection pool: %w", err), patterns)
	}

	if err := pool.Ping(context.Background()); err != nil {
		return nil, backends.MaybeConnError("postgres:Ping",
			fmt.Errorf("postgres ping failed: %w", err), patterns)
	}

	if err := createTable(context.Background(), pool); err != nil {
		return nil, fmt.Errorf("failed to create ratelimit table: %w", err)
	}

	return &Backend{
		pool:             pool,
		connErrorStrings: patterns,
	}, nil
}

// NewWithClient initializes a new PostgresBackend with a pre-configured connection pool.
//
// The pool is assumed to be already connected and ready for use.
func NewWithClient(pool *pgxpool.Pool) *Backend {
	return &Backend{
		pool:             pool,
		connErrorStrings: connErrorStrings, // Use default patterns
	}
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
		return fmt.Errorf("failed to execute table query 'CREATE TABLE': %w", err)
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
		return "", p.maybeConnError("postgres:Get",
			fmt.Errorf("failed to get key '%s' from postgres: %w", key, err))
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
		return p.maybeConnError("postgres:Set",
			fmt.Errorf("failed to set key '%s' in postgres: %w", key, err))
	}
	return nil
}

func (p *Backend) Delete(ctx context.Context, key string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM ratelimit_kv WHERE key = $1`, key)
	if err != nil {
		return p.maybeConnError("postgres:Delete",
			fmt.Errorf("failed to delete key '%s' from postgres: %w", key, err))
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
// This operation provides compare-and-swap (CAS) semantics for implementing optimistic locking.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - key: The storage key to operate on
//   - oldValue: Expected current value. Use empty string "" for "set if not exists" semantics
//   - newValue: New value to set if the current value matches oldValue
//   - expiration: Time-to-live for the key. Use 0 for no expiration
//
// Returns:
//   - bool: true if the CAS succeeded (value was set), false if the compare failed and no write occurred
//   - error: Any storage-related error (not including a compare mismatch)
//
// Behavior:
//   - If oldValue is "", the operation succeeds only if the key does not exist (or is expired)
//   - If oldValue matches the current value, the key is updated to newValue
//   - Expired keys are treated as non-existent for comparison purposes
//   - All values are stored and compared as strings
//
// Caller contract:
//   - A (false, nil) return indicates the compare condition did not match (e.g., another writer won the race
//     or the key already exists when using "set if not exists"). This is not an error. Callers may safely
//     reload state and retry with backoff according to their contention policy.
//   - A non-nil error indicates a storage/backend failure and should not be retried blindly.
func (p *Backend) CheckAndSet(ctx context.Context, key, oldValue, newValue string, expiration time.Duration) (bool, error) {
	var expiresAt *time.Time
	if expiration > 0 {
		t := time.Now().Add(expiration)
		expiresAt = &t
	}

	if oldValue == "" {
		// Insert new key if it doesn't exist, or replace it if the existing key is expired.
		result, err := p.pool.Exec(ctx, `
			INSERT INTO ratelimit_kv (key, value, expires_at)
			VALUES ($1, $2, $3)
			ON CONFLICT (key) DO UPDATE SET
				value = EXCLUDED.value,
				expires_at = EXCLUDED.expires_at
				WHERE ratelimit_kv.expires_at IS NOT NULL
					AND ratelimit_kv.expires_at <= NOW()

		`, key, newValue, expiresAt)
		if err != nil {
			return false, p.maybeConnError("postgres:CheckAndSet:Insert",
				fmt.Errorf("check-and-set operation failed for key '%s': %w", key, err))
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
		return false, p.maybeConnError("postgres:CheckAndSet:Update",
			fmt.Errorf("check-and-set operation failed for key '%s': %w", key, err))
	}

	return result.RowsAffected() == 1, nil
}

// PurgeExpired deletes up to batchSize expired rows and returns the number deleted.
func (p *Backend) PurgeExpired(ctx context.Context, batchSize int) (int64, error) {
	if batchSize <= 0 {
		batchSize = 1000
	}
	cmd, err := p.pool.Exec(ctx, `
		WITH stale AS (
			SELECT key FROM ratelimit_kv 
			WHERE expires_at IS NOT NULL AND expires_at <= NOW()
			LIMIT $1
		)
		DELETE FROM ratelimit_kv t
		USING stale
		WHERE t.key = stale.key
	`, batchSize)
	if err != nil {
		return 0, fmt.Errorf("purge expired failed: %w", err)
	}
	return cmd.RowsAffected(), nil
}

// maybeConnError checks if the error is a connectivity issue and wraps it as a health error.
//
// For Postgres, we consider connection timeouts, connection refused, and pool errors as health issues.
// Operational errors like constraint violations are not considered health errors.
func (b *Backend) maybeConnError(op string, err error) error {
	return backends.MaybeConnError(op, err, b.connErrorStrings)
}
