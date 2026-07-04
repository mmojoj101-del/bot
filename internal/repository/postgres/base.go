package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// BaseRepository provides common database operations.
type BaseRepository struct {
	pool    *pgxpool.Pool
	clock   func() time.Time
}

// NewBaseRepository creates a new base repository.
func NewBaseRepository(pool *pgxpool.Pool) *BaseRepository {
	return &BaseRepository{
		pool:  pool,
		clock: time.Now,
	}
}

// getQuerier returns a pool or transaction from context.
func (b *BaseRepository) getQuerier(ctx context.Context) interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, arguments ...interface{}) pgx.Row
	Query(ctx context.Context, sql string, arguments ...interface{}) (pgx.Rows, error)
} {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return b.pool
}

// softDeleteClause returns the SQL clause for soft delete.
func (b *BaseRepository) softDeleteClause() string {
	return "deleted_at IS NULL"
}

// now returns the current UTC time.
func (b *BaseRepository) now() time.Time {
	return b.clock().UTC()
}

// wrapError wraps common PostgreSQL errors into domain errors.
func (b *BaseRepository) wrapError(err error) error {
	if err == nil {
		return nil
	}
	if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
		return domain.ErrDuplicate
	}
	return err
}
