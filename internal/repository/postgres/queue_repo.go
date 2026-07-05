package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// QueueRepository implements domain.QueueRepository.
type QueueRepository struct {
	*BaseRepository
	txManager *TxManager
}

func NewQueueRepository(pool *pgxpool.Pool, txManager *TxManager) *QueueRepository {
	return &QueueRepository{
		BaseRepository: NewBaseRepository(pool),
		txManager:      txManager,
	}
}

// ClaimQueued pulls pending messages using FOR UPDATE SKIP LOCKED.
// Multiple workers can safely pull without duplicates.
func (r *QueueRepository) ClaimQueued(ctx context.Context, limit int) ([]domain.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var messages []domain.Message
	err := r.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		// Step 1: Claim messages with FOR UPDATE SKIP LOCKED
		rows, err := r.getQuerier(txCtx).Query(ctx,
			`SELECT id, tenant_id, connector_id, route_id, client_id, direction, status,
			        source, destination, text, encoding, priority, parts, dlr_status, dlr_url, dlr_id,
			        external_id, client_ref, retry_count, max_retries, price, cost,
			        sent_at, delivered_at, failed_at, error_code, error_message,
			        created_at, updated_at, deleted_at
			 FROM messages
			 WHERE status = 'queued' AND `+r.softDeleteClause()+`
			 ORDER BY created_at ASC
			 LIMIT $1
			 FOR UPDATE SKIP LOCKED`,
			limit)
		if err != nil {
			return fmt.Errorf("claim queued: %w", err)
		}
		defer rows.Close()

		var ids []string
		for rows.Next() {
			var m domain.Message
			if err := rows.Scan(&m.ID, &m.TenantID, &m.ConnectorID, &m.RouteID, &m.ClientID, &m.Direction, &m.Status,
				&m.Source, &m.Destination, &m.Text, &m.Encoding, &m.Priority, &m.Parts, &m.DLRStatus, &m.DLRURL, &m.DLRID,
				&m.ExternalID, &m.ClientRef, &m.RetryCount, &m.MaxRetries, &m.Price, &m.Cost,
				&m.SentAt, &m.DeliveredAt, &m.FailedAt, &m.ErrorCode, &m.ErrorMessage,
				&m.CreatedAt, &m.UpdatedAt, &m.DeletedAt); err != nil {
				return fmt.Errorf("scan claimed message: %w", err)
			}
			ids = append(ids, m.ID)
			messages = append(messages, m)
		}

		if len(ids) == 0 {
			return nil // nothing to claim
		}

		// Step 2: Update claimed messages to 'sending' within the same transaction
		tag, err := r.getQuerier(txCtx).Exec(ctx,
			`UPDATE messages SET
				status = 'sending',
				version = version + 1,
				updated_at = $1
			 WHERE id = ANY($2) AND status = 'queued' AND `+r.softDeleteClause(),
			time.Now().UTC(), ids)
		if err != nil {
			return fmt.Errorf("claim update to sending: %w", err)
		}
		if tag.RowsAffected() != int64(len(ids)) {
			return fmt.Errorf("mismatch: claimed %d but updated %d", len(ids), tag.RowsAffected())
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return messages, nil
}

// AckSent marks a message as sent after successful transmission.
func (r *QueueRepository) AckSent(ctx context.Context, id string, version int, externalID string, parts int, price, cost int64) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	now := time.Now().UTC()
	tag, err := q.Exec(ctx,
		`UPDATE messages SET
			status = 'sent',
			external_id = $1,
			parts = $2,
			price = $3,
			cost = $4,
			sent_at = $5,
			version = version + 1,
			updated_at = $5
		 WHERE id = $6 AND version = $7 AND `+r.softDeleteClause(),
		externalID, parts, price, cost, now, id, version)
	if err != nil {
		return fmt.Errorf("ack sent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return nil
}

// AckFailed marks a message as failed after a send error.
func (r *QueueRepository) AckFailed(ctx context.Context, id string, version int, errorCode, errorMessage string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	now := time.Now().UTC()
	tag, err := q.Exec(ctx,
		`UPDATE messages SET
			status = 'failed',
			error_code = $1,
			error_message = $2,
			failed_at = $3,
			version = version + 1,
			updated_at = $3
		 WHERE id = $4 AND version = $5 AND `+r.softDeleteClause(),
		errorCode, errorMessage, now, id, version)
	if err != nil {
		return fmt.Errorf("ack failed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return nil
}

// ScheduleRetry moves a message to 'retrying' status for a future retry.
// The retry engine will pick it up after the backoff delay has elapsed.
func (r *QueueRepository) ScheduleRetry(ctx context.Context, id string, version int, errorCode, errorMessage string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	now := time.Now().UTC()
	tag, err := q.Exec(ctx,
		`UPDATE messages SET
			status = 'retrying',
			retry_count = retry_count + 1,
			error_code = $1,
			error_message = $2,
			version = version + 1,
			updated_at = $3
		 WHERE id = $4 AND version = $5 AND `+r.softDeleteClause(),
		errorCode, errorMessage, now, id, version)
	if err != nil {
		return fmt.Errorf("schedule retry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return nil
}

// GetRetryable fetches messages whose backoff delay has elapsed.
// The delay is calculated as: retry_backoff_base * 2^(attempt-1) seconds.
func (r *QueueRepository) GetRetryable(ctx context.Context, now time.Time, minDelay time.Duration, limit int) ([]domain.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	cutoff := now.Add(-minDelay)

	rows, err := q.Query(ctx,
		`SELECT id, tenant_id, connector_id, route_id, client_id, direction, status,
		        source, destination, text, encoding, priority, parts, dlr_status, dlr_url, dlr_id,
		        external_id, client_ref, retry_count, max_retries, price, cost,
		        sent_at, delivered_at, failed_at, error_code, error_message,
		        created_at, updated_at, deleted_at
		 FROM messages
		 WHERE status = 'retrying' AND updated_at <= $1 AND `+r.softDeleteClause()+`
		 ORDER BY updated_at ASC
		 LIMIT $2`,
		cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("get retryable: %w", err)
	}
	defer rows.Close()

	var messages []domain.Message
	for rows.Next() {
		var m domain.Message
		if err := rows.Scan(&m.ID, &m.TenantID, &m.ConnectorID, &m.RouteID, &m.ClientID, &m.Direction, &m.Status,
			&m.Source, &m.Destination, &m.Text, &m.Encoding, &m.Priority, &m.Parts, &m.DLRStatus, &m.DLRURL, &m.DLRID,
			&m.ExternalID, &m.ClientRef, &m.RetryCount, &m.MaxRetries, &m.Price, &m.Cost,
			&m.SentAt, &m.DeliveredAt, &m.FailedAt, &m.ErrorCode, &m.ErrorMessage,
			&m.CreatedAt, &m.UpdatedAt, &m.DeletedAt); err != nil {
			return nil, fmt.Errorf("scan retryable: %w", err)
		}
		messages = append(messages, m)
	}

	return messages, nil
}
