package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// OutboxRepository implements domain.OutboxRepository.
type OutboxRepository struct {
	*BaseRepository
}

func NewOutboxRepository(pool *pgxpool.Pool) *OutboxRepository {
	return &OutboxRepository{BaseRepository: NewBaseRepository(pool)}
}

func (r *OutboxRepository) Create(ctx context.Context, event *domain.OutboxEvent) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	_, err := q.Exec(ctx,
		`INSERT INTO outbox_events (event_type, tenant_id, payload)
		 VALUES ($1, $2, $3::jsonb)`,
		event.EventType, event.TenantID, event.Payload,
	)
	return r.wrapError(err)
}

func (r *OutboxRepository) GetPending(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	rows, err := q.Query(ctx,
		`SELECT id, event_type, tenant_id, payload, status, attempts, last_error, published_at, created_at, updated_at, deleted_at
		 FROM outbox_events
		 WHERE status = 'pending' AND `+r.softDeleteClause()+`
		 ORDER BY created_at ASC
		 LIMIT $1
		 FOR UPDATE SKIP LOCKED`,
		limit)
	if err != nil {
		return nil, fmt.Errorf("get pending outbox: %w", err)
	}
	defer rows.Close()

	var events []domain.OutboxEvent
	for rows.Next() {
		var e domain.OutboxEvent
		if err := rows.Scan(&e.ID, &e.EventType, &e.TenantID, &e.Payload, &e.Status, &e.Attempts,
			&e.LastError, &e.PublishedAt, &e.CreatedAt, &e.UpdatedAt, &e.DeletedAt); err != nil {
			return nil, fmt.Errorf("scan outbox event: %w", err)
		}
		events = append(events, e)
	}
	return events, nil
}

func (r *OutboxRepository) MarkPublished(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	now := time.Now().UTC()
	_, err := q.Exec(ctx,
		`UPDATE outbox_events SET status = 'published', published_at = $1, updated_at = $1
		 WHERE id = $2 AND `+r.softDeleteClause(),
		now, id)
	return err
}

func (r *OutboxRepository) MarkFailed(ctx context.Context, id string, errMsg string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	_, err := q.Exec(ctx,
		`UPDATE outbox_events SET status = 'failed', last_error = $1, attempts = attempts + 1, updated_at = $2
		 WHERE id = $3 AND `+r.softDeleteClause(),
		errMsg, time.Now().UTC(), id)
	return err
}

func (r *OutboxRepository) DeletePublished(ctx context.Context, before time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	_, err := q.Exec(ctx,
		`UPDATE outbox_events SET deleted_at = $1 WHERE status = 'published' AND published_at < $1`,
		before)
	return err
}
