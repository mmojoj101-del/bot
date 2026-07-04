package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// ConnectorRepository implements domain.ConnectorRepository.
type ConnectorRepository struct {
	*BaseRepository
}

func NewConnectorRepository(pool *pgxpool.Pool) *ConnectorRepository {
	return &ConnectorRepository{BaseRepository: NewBaseRepository(pool)}
}

func (r *ConnectorRepository) Create(ctx context.Context, input domain.CreateConnectorInput, createdBy string) (*domain.Connector, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	c := &domain.Connector{}
	config := input.Config
	if config == nil {
		config = []byte("{}")
	}
	status := domain.ConnectorStatusActive
	if input.Status != nil {
		status = *input.Status
	}

	err := q.QueryRow(ctx,
		`INSERT INTO connectors (tenant_id, type, name, status, config, created_by, updated_by)
		 VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)
		 RETURNING id, tenant_id, type, name, status, config, created_by, updated_by, created_at, updated_at, version`,
		input.TenantID, input.Type, input.Name, status, config, createdBy, createdBy,
	).Scan(&c.ID, &c.TenantID, &c.Type, &c.Name, &c.Status, &c.Config, &c.CreatedBy, &c.UpdatedBy, &c.CreatedAt, &c.UpdatedAt, &c.Version)
	if err != nil {
		return nil, r.wrapError(err)
	}
	return c, nil
}

func (r *ConnectorRepository) GetByID(ctx context.Context, id string) (*domain.Connector, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	c := &domain.Connector{}
	err := q.QueryRow(ctx,
		`SELECT id, tenant_id, type, name, status, config, created_by, updated_by, created_at, updated_at, deleted_at, version
		 FROM connectors WHERE id = $1 AND `+r.softDeleteClause(),
		id,
	).Scan(&c.ID, &c.TenantID, &c.Type, &c.Name, &c.Status, &c.Config, &c.CreatedBy, &c.UpdatedBy, &c.CreatedAt, &c.UpdatedAt, &c.DeletedAt, &c.Version)
	if err == pgx.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get connector by id: %w", err)
	}
	return c, nil
}

func (r *ConnectorRepository) Update(ctx context.Context, id string, input domain.UpdateConnectorInput, updatedBy string, version int) (*domain.Connector, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	c := &domain.Connector{}
	err := q.QueryRow(ctx,
		`UPDATE connectors SET
			name = COALESCE($1, name),
			type = COALESCE($2, type),
			status = COALESCE($3, status),
			config = CASE WHEN $4::jsonb IS NOT NULL THEN $4::jsonb ELSE config END,
			updated_by = $5,
			updated_at = $6,
			version = version + 1
		 WHERE id = $7 AND version = $8 AND `+r.softDeleteClause()+`
		 RETURNING id, tenant_id, type, name, status, config, created_by, updated_by, created_at, updated_at, version`,
		nullableString(input.Name), (*string)(input.Type), (*string)(input.Status),
		input.Config, updatedBy, time.Now().UTC(), id, version,
	).Scan(&c.ID, &c.TenantID, &c.Type, &c.Name, &c.Status, &c.Config, &c.CreatedBy, &c.UpdatedBy, &c.CreatedAt, &c.UpdatedAt, &c.Version)
	if err == pgx.ErrNoRows {
		if _, err2 := r.GetByID(ctx, id); err2 != nil {
			return nil, domain.ErrNotFound
		}
		return nil, domain.ErrConflict
	}
	if err != nil {
		return nil, fmt.Errorf("update connector: %w", err)
	}
	return c, nil
}

func (r *ConnectorRepository) Delete(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	tag, err := q.Exec(ctx,
		`UPDATE connectors SET deleted_at = $1, updated_at = $1 WHERE id = $2 AND `+r.softDeleteClause(),
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

func (r *ConnectorRepository) ListByTenant(ctx context.Context, filter domain.ConnectorFilter) (domain.PageResult[domain.Connector], error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)

	query := `SELECT id, tenant_id, type, name, status, config, created_by, updated_by, created_at, updated_at, deleted_at, version
		 FROM connectors WHERE tenant_id = $1 AND ` + r.softDeleteClause()
	countQuery := `SELECT COUNT(*) FROM connectors WHERE tenant_id = $1 AND ` + r.softDeleteClause()

	args := []interface{}{filter.TenantID}
	argIdx := 2

	if filter.Type != nil {
		query += fmt.Sprintf(" AND type = $%d", argIdx)
		countQuery += fmt.Sprintf(" AND type = $%d", argIdx)
		args = append(args, *filter.Type)
		argIdx++
	}
	if filter.Status != nil {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		countQuery += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, *filter.Status)
		argIdx++
	}
	if filter.Search != "" {
		searchPattern := "%" + filter.Search + "%"
		query += fmt.Sprintf(" AND (name ILIKE $%d)", argIdx)
		countQuery += fmt.Sprintf(" AND (name ILIKE $%d)", argIdx)
		args = append(args, searchPattern)
		argIdx++
	}

	// Count
	var total int64
	err := q.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return domain.PageResult[domain.Connector]{}, err
	}

	// Paginated query
	query += ` ORDER BY created_at DESC LIMIT $` + fmt.Sprintf("%d", argIdx) + ` OFFSET $` + fmt.Sprintf("%d", argIdx+1)
	args = append(args, filter.Page.Limit, filter.Page.Offset)

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return domain.PageResult[domain.Connector]{}, err
	}
	defer rows.Close()

	var connectors []domain.Connector
	for rows.Next() {
		var c domain.Connector
		if err := rows.Scan(&c.ID, &c.TenantID, &c.Type, &c.Name, &c.Status, &c.Config, &c.CreatedBy, &c.UpdatedBy, &c.CreatedAt, &c.UpdatedAt, &c.DeletedAt, &c.Version); err != nil {
			return domain.PageResult[domain.Connector]{}, err
		}
		connectors = append(connectors, c)
	}

	return domain.PageResult[domain.Connector]{Items: connectors, Total: total, Page: filter.Page}, nil
}

func (r *ConnectorRepository) CountByTenant(ctx context.Context, tenantID string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	var count int64
	err := q.QueryRow(ctx,
		`SELECT COUNT(*) FROM connectors WHERE tenant_id = $1 AND `+r.softDeleteClause(),
		tenantID,
	).Scan(&count)
	return count, err
}
