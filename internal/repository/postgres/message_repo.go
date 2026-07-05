package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// MessageRepository implements domain.MessageRepository.
type MessageRepository struct {
	*BaseRepository
}

func NewMessageRepository(pool *pgxpool.Pool) *MessageRepository {
	return &MessageRepository{BaseRepository: NewBaseRepository(pool)}
}

func (r *MessageRepository) Create(ctx context.Context, input domain.CreateMessageInput, createdBy string) (*domain.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	m := &domain.Message{}
	encoding := input.Encoding
	if encoding == "" {
		encoding = domain.EncodingGSM7
	}
	priority := input.Priority
	if priority == "" {
		priority = domain.MessagePriorityNormal
	}
	maxRetries := input.MaxRetries
	if maxRetries < 1 {
		maxRetries = domain.MaxRetriesDefault
	}

	err := q.QueryRow(ctx,
		`INSERT INTO messages (tenant_id, client_id, direction, source, destination, text, encoding, priority, dlr_url, client_ref, max_retries)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 RETURNING id, tenant_id, status, direction, source, destination, text, encoding, priority,
		           parts, retry_count, max_retries, price, cost, client_ref,
		           created_at, updated_at`,
		input.TenantID, input.ClientID, input.Direction, input.Source, input.Destination,
		input.Text, encoding, priority, nullableString(&input.DLRURL), nullableString(&input.ClientRef), maxRetries,
	).Scan(&m.ID, &m.TenantID, &m.Status, &m.Direction, &m.Source, &m.Destination, &m.Text, &m.Encoding, &m.Priority,
		&m.Parts, &m.RetryCount, &m.MaxRetries, &m.Price, &m.Cost, &m.ClientRef,
		&m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, r.wrapError(err)
	}
	return m, nil
}

func (r *MessageRepository) GetByID(ctx context.Context, id string) (*domain.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	m := &domain.Message{}
	err := q.QueryRow(ctx,
		`SELECT id, tenant_id, connector_id, route_id, client_id, direction, status, previous_status,
		        source, destination, text, encoding, priority, parts, dlr_status, dlr_url, dlr_id,
		        external_id, client_ref, retry_count, max_retries, price, cost,
		        sent_at, delivered_at, failed_at, error_code, error_message,
		        created_at, updated_at, deleted_at
		 FROM messages WHERE id = $1 AND `+r.softDeleteClause(),
		id,
	).Scan(&m.ID, &m.TenantID, &m.ConnectorID, &m.RouteID, &m.ClientID, &m.Direction, &m.Status, &m.PreviousStatus,
		&m.Source, &m.Destination, &m.Text, &m.Encoding, &m.Priority, &m.Parts, &m.DLRStatus, &m.DLRURL, &m.DLRID,
		&m.ExternalID, &m.ClientRef, &m.RetryCount, &m.MaxRetries, &m.Price, &m.Cost,
		&m.SentAt, &m.DeliveredAt, &m.FailedAt, &m.ErrorCode, &m.ErrorMessage,
		&m.CreatedAt, &m.UpdatedAt, &m.DeletedAt)
	if err == pgx.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get message by id: %w", err)
	}
	return m, nil
}

func (r *MessageRepository) GetByClientRef(ctx context.Context, tenantID, clientRef string) (*domain.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	m := &domain.Message{}
	err := q.QueryRow(ctx,
		`SELECT id, tenant_id, connector_id, route_id, client_id, direction, status, previous_status,
		        source, destination, text, encoding, priority, parts, dlr_status, dlr_url, dlr_id,
		        external_id, client_ref, retry_count, max_retries, price, cost,
		        sent_at, delivered_at, failed_at, error_code, error_message,
		        created_at, updated_at, deleted_at
		 FROM messages WHERE tenant_id = $1 AND client_ref = $2 AND `+r.softDeleteClause(),
		tenantID, clientRef,
	).Scan(&m.ID, &m.TenantID, &m.ConnectorID, &m.RouteID, &m.ClientID, &m.Direction, &m.Status, &m.PreviousStatus,
		&m.Source, &m.Destination, &m.Text, &m.Encoding, &m.Priority, &m.Parts, &m.DLRStatus, &m.DLRURL, &m.DLRID,
		&m.ExternalID, &m.ClientRef, &m.RetryCount, &m.MaxRetries, &m.Price, &m.Cost,
		&m.SentAt, &m.DeliveredAt, &m.FailedAt, &m.ErrorCode, &m.ErrorMessage,
		&m.CreatedAt, &m.UpdatedAt, &m.DeletedAt)
	if err == pgx.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get message by client ref: %w", err)
	}
	return m, nil
}

func (r *MessageRepository) GetByExternalID(ctx context.Context, externalID string) (*domain.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	m := &domain.Message{}
	err := q.QueryRow(ctx,
		`SELECT id, tenant_id, connector_id, route_id, client_id, direction, status, previous_status,
		        source, destination, text, encoding, priority, parts, dlr_status, dlr_url, dlr_id,
		        external_id, client_ref, retry_count, max_retries, price, cost,
		        sent_at, delivered_at, failed_at, error_code, error_message,
		        created_at, updated_at, deleted_at
		 FROM messages WHERE external_id = $1 AND `+r.softDeleteClause(),
		externalID,
	).Scan(&m.ID, &m.TenantID, &m.ConnectorID, &m.RouteID, &m.ClientID, &m.Direction, &m.Status, &m.PreviousStatus,
		&m.Source, &m.Destination, &m.Text, &m.Encoding, &m.Priority, &m.Parts, &m.DLRStatus, &m.DLRURL, &m.DLRID,
		&m.ExternalID, &m.ClientRef, &m.RetryCount, &m.MaxRetries, &m.Price, &m.Cost,
		&m.SentAt, &m.DeliveredAt, &m.FailedAt, &m.ErrorCode, &m.ErrorMessage,
		&m.CreatedAt, &m.UpdatedAt, &m.DeletedAt)
	if err == pgx.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get message by external id: %w", err)
	}
	return m, nil
}

func (r *MessageRepository) UpdateStatus(ctx context.Context, id string, input domain.UpdateMessageInput, version int) (*domain.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	m := &domain.Message{}
	err := q.QueryRow(ctx,
		`UPDATE messages SET
			status = COALESCE($1, status),
			previous_status = CASE WHEN $1 IS NOT NULL AND $1 != status THEN status ELSE previous_status END,
			connector_id = COALESCE($2, connector_id),
			route_id = COALESCE($3, route_id),
			external_id = COALESCE($4, external_id),
			dlr_status = COALESCE($5, dlr_status),
			dlr_id = COALESCE($6, dlr_id),
			error_code = COALESCE($7, error_code),
			error_message = COALESCE($8, error_message),
			parts = COALESCE($9, parts),
			price = COALESCE($10, price),
			cost = COALESCE($11, cost),
			sent_at = COALESCE($12, sent_at),
			delivered_at = COALESCE($13, delivered_at),
			failed_at = COALESCE($14, failed_at),
			retry_count = CASE WHEN $1 = 'retrying' THEN retry_count + 1 ELSE retry_count END,
			updated_at = $15,
			version = version + 1
		 WHERE id = $16 AND version = $17 AND `+r.softDeleteClause()+`
		 RETURNING id, tenant_id, connector_id, route_id, client_id, direction, status, previous_status,
		           source, destination, text, encoding, priority, parts, dlr_status, dlr_url, dlr_id,
		           external_id, client_ref, retry_count, max_retries, price, cost,
		           sent_at, delivered_at, failed_at, error_code, error_message,
		           created_at, updated_at, version`,
		(*string)(input.Status), nullableString(input.ConnectorID), nullableString(input.RouteID),
		nullableString(input.ExternalID), (*string)(input.DLRStatus), nullableString(input.DLRID),
		nullableString(input.ErrorCode), nullableString(input.ErrorMessage),
		input.Parts, input.Price, input.Cost,
		input.SentAt, input.DeliveredAt, input.FailedAt,
		time.Now().UTC(), id, version,
	).Scan(&m.ID, &m.TenantID, &m.ConnectorID, &m.RouteID, &m.ClientID, &m.Direction, &m.Status, &m.PreviousStatus,
		&m.Source, &m.Destination, &m.Text, &m.Encoding, &m.Priority, &m.Parts, &m.DLRStatus, &m.DLRURL, &m.DLRID,
		&m.ExternalID, &m.ClientRef, &m.RetryCount, &m.MaxRetries, &m.Price, &m.Cost,
		&m.SentAt, &m.DeliveredAt, &m.FailedAt, &m.ErrorCode, &m.ErrorMessage,
		&m.CreatedAt, &m.UpdatedAt, &m.Version)
	if err == pgx.ErrNoRows {
		if _, err2 := r.GetByID(ctx, id); err2 != nil {
			return nil, domain.ErrNotFound
		}
		return nil, domain.ErrConflict
	}
	if err != nil {
		return nil, fmt.Errorf("update message status: %w", err)
	}
	return m, nil
}

func (r *MessageRepository) AppendDLR(ctx context.Context, dlr *domain.DLRRecord) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	_, err := q.Exec(ctx,
		`INSERT INTO dlr_logs (message_id, tenant_id, status, external_id, error_code, description, raw_response)
		 VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)`,
		dlr.MessageID, dlr.TenantID, dlr.Status, nullableString(&dlr.ExternalID),
		nullableString(&dlr.ErrorCode), nullableString(&dlr.Description), dlr.RawResponse,
	)
	return err
}

func (r *MessageRepository) List(ctx context.Context, filter domain.MessageFilter) (domain.PageResult[domain.Message], error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)

	query := `SELECT id, tenant_id, connector_id, route_id, client_id, direction, status, previous_status,
		        source, destination, text, encoding, priority, parts, dlr_status, dlr_url, dlr_id,
		        external_id, client_ref, retry_count, max_retries, price, cost,
		        sent_at, delivered_at, failed_at, error_code, error_message,
		        created_at, updated_at, deleted_at
		 FROM messages WHERE tenant_id = $1 AND ` + r.softDeleteClause()
	countQuery := `SELECT COUNT(*) FROM messages WHERE tenant_id = $1 AND ` + r.softDeleteClause()

	args := []interface{}{filter.TenantID}
	argIdx := 2

	if filter.Status != nil {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		countQuery += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, *filter.Status)
		argIdx++
	}
	if filter.Direction != nil {
		query += fmt.Sprintf(" AND direction = $%d", argIdx)
		countQuery += fmt.Sprintf(" AND direction = $%d", argIdx)
		args = append(args, *filter.Direction)
		argIdx++
	}
	if filter.ConnectorID != "" {
		query += fmt.Sprintf(" AND connector_id = $%d", argIdx)
		countQuery += fmt.Sprintf(" AND connector_id = $%d", argIdx)
		args = append(args, filter.ConnectorID)
		argIdx++
	}
	if filter.Source != "" {
		query += fmt.Sprintf(" AND source = $%d", argIdx)
		countQuery += fmt.Sprintf(" AND source = $%d", argIdx)
		args = append(args, filter.Source)
		argIdx++
	}
	if filter.Destination != "" {
		query += fmt.Sprintf(" AND destination = $%d", argIdx)
		countQuery += fmt.Sprintf(" AND destination = $%d", argIdx)
		args = append(args, filter.Destination)
		argIdx++
	}
	if filter.ClientRef != "" {
		query += fmt.Sprintf(" AND client_ref = $%d", argIdx)
		countQuery += fmt.Sprintf(" AND client_ref = $%d", argIdx)
		args = append(args, filter.ClientRef)
		argIdx++
	}
	if filter.Search != "" {
		searchPattern := "%" + filter.Search + "%"
		query += fmt.Sprintf(" AND (text ILIKE $%d OR source ILIKE $%d OR destination ILIKE $%d)", argIdx, argIdx+1, argIdx+2)
		countQuery += fmt.Sprintf(" AND (text ILIKE $%d OR source ILIKE $%d OR destination ILIKE $%d)", argIdx, argIdx+1, argIdx+2)
		args = append(args, searchPattern, searchPattern, searchPattern)
		argIdx += 3
	}
	if filter.DateFrom != nil {
		query += fmt.Sprintf(" AND created_at >= $%d", argIdx)
		countQuery += fmt.Sprintf(" AND created_at >= $%d", argIdx)
		args = append(args, *filter.DateFrom)
		argIdx++
	}
	if filter.DateTo != nil {
		query += fmt.Sprintf(" AND created_at <= $%d", argIdx)
		countQuery += fmt.Sprintf(" AND created_at <= $%d", argIdx)
		args = append(args, *filter.DateTo)
		argIdx++
	}

	// Count
	var total int64
	if err := q.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return domain.PageResult[domain.Message]{}, err
	}

	// Paginated query
	query += ` ORDER BY created_at DESC LIMIT $` + fmt.Sprintf("%d", argIdx) + ` OFFSET $` + fmt.Sprintf("%d", argIdx+1)
	args = append(args, filter.Page.Limit, filter.Page.Offset)

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return domain.PageResult[domain.Message]{}, err
	}
	defer rows.Close()

	var messages []domain.Message
	for rows.Next() {
		var m domain.Message
		if err := rows.Scan(&m.ID, &m.TenantID, &m.ConnectorID, &m.RouteID, &m.ClientID, &m.Direction, &m.Status, &m.PreviousStatus,
			&m.Source, &m.Destination, &m.Text, &m.Encoding, &m.Priority, &m.Parts, &m.DLRStatus, &m.DLRURL, &m.DLRID,
			&m.ExternalID, &m.ClientRef, &m.RetryCount, &m.MaxRetries, &m.Price, &m.Cost,
			&m.SentAt, &m.DeliveredAt, &m.FailedAt, &m.ErrorCode, &m.ErrorMessage,
			&m.CreatedAt, &m.UpdatedAt, &m.DeletedAt); err != nil {
			return domain.PageResult[domain.Message]{}, err
		}
		messages = append(messages, m)
	}

	return domain.PageResult[domain.Message]{Items: messages, Total: total, Page: filter.Page}, nil
}

func (r *MessageRepository) Count(ctx context.Context, filter domain.MessageFilter) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	var total int64

	query := `SELECT COUNT(*) FROM messages WHERE tenant_id = $1 AND ` + r.softDeleteClause()
	args := []interface{}{filter.TenantID}
	argIdx := 2

	if filter.Status != nil {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, *filter.Status)
		argIdx++
	}
	if filter.Direction != nil {
		query += fmt.Sprintf(" AND direction = $%d", argIdx)
		args = append(args, *filter.Direction)
		argIdx++
	}

	err := q.QueryRow(ctx, query, args...).Scan(&total)
	return total, err
}

func (r *MessageRepository) Delete(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	tag, err := q.Exec(ctx,
		`UPDATE messages SET deleted_at = $1, updated_at = $1 WHERE id = $2 AND `+r.softDeleteClause(),
		time.Now().UTC(), id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
