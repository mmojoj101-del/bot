package postgres

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// AuditLogRepository implements domain.AuditLogRepository.
type AuditLogRepository struct {
	*BaseRepository
}

func NewAuditLogRepository(pool *pgxpool.Pool) *AuditLogRepository {
	return &AuditLogRepository{BaseRepository: NewBaseRepository(pool)}
}

func (r *AuditLogRepository) Create(ctx context.Context, log *domain.AuditLog) error {
	q := r.getQuerier(ctx)
	err := q.QueryRow(ctx,
		`INSERT INTO audit_logs (id, tenant_id, user_id, request_id, action, resource, metadata, ip_address, user_agent)
		 VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9)
		 RETURNING created_at`,
		log.ID, log.TenantID, log.UserID, log.RequestID, log.Action, log.Resource,
		log.Metadata, log.IPAddress, log.UserAgent,
	).Scan(&log.CreatedAt)
	return err
}

func (r *AuditLogRepository) GetByID(ctx context.Context, id string) (*domain.AuditLog, error) {
	q := r.getQuerier(ctx)
	l := &domain.AuditLog{}
	err := q.QueryRow(ctx,
		`SELECT id, tenant_id, user_id, request_id, action, resource, metadata, ip_address, user_agent, created_at
		 FROM audit_logs WHERE id = $1`,
		id,
	).Scan(&l.ID, &l.TenantID, &l.UserID, &l.RequestID, &l.Action, &l.Resource,
		&l.Metadata, &l.IPAddress, &l.UserAgent, &l.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get audit log by id: %w", err)
	}
	return l, nil
}

func (r *AuditLogRepository) ListByTenant(ctx context.Context, tenantID string, page domain.CursorPage) (domain.CursorPageResult[domain.AuditLog], error) {
	q := r.getQuerier(ctx)

	var rows pgx.Rows
	var err error

	if page.Cursor == "" {
		rows, err = q.Query(ctx,
			`SELECT id, tenant_id, user_id, request_id, action, resource, metadata, ip_address, user_agent, created_at
			 FROM audit_logs WHERE tenant_id = $1
			 ORDER BY created_at DESC LIMIT $2`,
			tenantID, page.Limit+1,
		)
	} else {
		cursorTime, err := decodeCursor(string(page.Cursor))
		if err != nil {
			return domain.CursorPageResult[domain.AuditLog]{}, fmt.Errorf("invalid cursor: %w", err)
		}
		rows, err = q.Query(ctx,
			`SELECT id, tenant_id, user_id, request_id, action, resource, metadata, ip_address, user_agent, created_at
			 FROM audit_logs WHERE tenant_id = $1 AND created_at < $2
			 ORDER BY created_at DESC LIMIT $3`,
			tenantID, cursorTime, page.Limit+1,
		)
	}
	if err != nil {
		return domain.CursorPageResult[domain.AuditLog]{}, err
	}
	defer rows.Close()

	var logs []domain.AuditLog
	var hasMore bool
	count := 0
	for rows.Next() {
		if count == page.Limit {
			hasMore = true
			break
		}
		var l domain.AuditLog
		if err := rows.Scan(&l.ID, &l.TenantID, &l.UserID, &l.RequestID, &l.Action, &l.Resource,
			&l.Metadata, &l.IPAddress, &l.UserAgent, &l.CreatedAt); err != nil {
			return domain.CursorPageResult[domain.AuditLog]{}, err
		}
		logs = append(logs, l)
		count++
	}

	result := domain.CursorPageResult[domain.AuditLog]{
		Items:   logs,
		HasMore: hasMore,
	}

	if hasMore && len(logs) > 0 {
		lastLog := logs[len(logs)-1]
		result.NextCursor = domain.Cursor(encodeCursor(lastLog.CreatedAt))
	}

	return result, nil
}

func (r *AuditLogRepository) ListByUser(ctx context.Context, userID string, page domain.CursorPage) (domain.CursorPageResult[domain.AuditLog], error) {
	q := r.getQuerier(ctx)

	var rows pgx.Rows
	var err error

	if page.Cursor == "" {
		rows, err = q.Query(ctx,
			`SELECT id, tenant_id, user_id, request_id, action, resource, metadata, ip_address, user_agent, created_at
			 FROM audit_logs WHERE user_id = $1
			 ORDER BY created_at DESC LIMIT $2`,
			userID, page.Limit+1,
		)
	} else {
		cursorTime, err := decodeCursor(string(page.Cursor))
		if err != nil {
			return domain.CursorPageResult[domain.AuditLog]{}, fmt.Errorf("invalid cursor: %w", err)
		}
		rows, err = q.Query(ctx,
			`SELECT id, tenant_id, user_id, request_id, action, resource, metadata, ip_address, user_agent, created_at
			 FROM audit_logs WHERE user_id = $1 AND created_at < $2
			 ORDER BY created_at DESC LIMIT $3`,
			userID, cursorTime, page.Limit+1,
		)
	}
	if err != nil {
		return domain.CursorPageResult[domain.AuditLog]{}, err
	}
	defer rows.Close()

	var logs []domain.AuditLog
	var hasMore bool
	count := 0
	for rows.Next() {
		if count == page.Limit {
			hasMore = true
			break
		}
		var l domain.AuditLog
		if err := rows.Scan(&l.ID, &l.TenantID, &l.UserID, &l.RequestID, &l.Action, &l.Resource,
			&l.Metadata, &l.IPAddress, &l.UserAgent, &l.CreatedAt); err != nil {
			return domain.CursorPageResult[domain.AuditLog]{}, err
		}
		logs = append(logs, l)
		count++
	}

	result := domain.CursorPageResult[domain.AuditLog]{
		Items:   logs,
		HasMore: hasMore,
	}

	if hasMore && len(logs) > 0 {
		lastLog := logs[len(logs)-1]
		result.NextCursor = domain.Cursor(encodeCursor(lastLog.CreatedAt))
	}

	return result, nil
}

func encodeCursor(t time.Time) string {
	return base64.URLEncoding.EncodeToString([]byte(t.Format(time.RFC3339Nano)))
}

func decodeCursor(s string) (time.Time, error) {
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339Nano, string(b))
}
