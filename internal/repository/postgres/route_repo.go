package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// RouteRepository implements domain.RouteRepository.
type RouteRepository struct {
	*BaseRepository
}

func NewRouteRepository(pool *pgxpool.Pool) *RouteRepository {
	return &RouteRepository{BaseRepository: NewBaseRepository(pool)}
}

func (r *RouteRepository) Create(ctx context.Context, input domain.CreateRouteInput, createdBy string) (*domain.Route, error) {
	q := r.getQuerier(ctx)
	route := &domain.Route{}
	err := q.QueryRow(ctx,
		`INSERT INTO routes (tenant_id, type, priority, prefix, connector_id, created_by, updated_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, tenant_id, type, priority, prefix, connector_id, enabled, created_by, updated_by, created_at, updated_at, version`,
		input.TenantID, input.Type, input.Priority, input.Prefix, input.ConnectorID, createdBy, createdBy,
	).Scan(&route.ID, &route.TenantID, &route.Type, &route.Priority, &route.Prefix, &route.ConnectorID,
		&route.Enabled, &route.CreatedBy, &route.UpdatedBy, &route.CreatedAt, &route.UpdatedAt, &route.Version)
	if err != nil {
		return nil, r.wrapError(err)
	}
	return route, nil
}

func (r *RouteRepository) GetByID(ctx context.Context, id string) (*domain.Route, error) {
	q := r.getQuerier(ctx)
	route := &domain.Route{}
	err := q.QueryRow(ctx,
		`SELECT id, tenant_id, type, priority, prefix, connector_id, enabled, created_by, updated_by, created_at, updated_at, deleted_at, version
		 FROM routes WHERE id = $1 AND `+r.softDeleteClause(),
		id,
	).Scan(&route.ID, &route.TenantID, &route.Type, &route.Priority, &route.Prefix, &route.ConnectorID,
		&route.Enabled, &route.CreatedBy, &route.UpdatedBy, &route.CreatedAt, &route.UpdatedAt, &route.DeletedAt, &route.Version)
	if err == pgx.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get route by id: %w", err)
	}
	return route, nil
}

func (r *RouteRepository) Update(ctx context.Context, id string, input domain.UpdateRouteInput, updatedBy string, version int) (*domain.Route, error) {
	q := r.getQuerier(ctx)
	route := &domain.Route{}
	err := q.QueryRow(ctx,
		`UPDATE routes SET
			type = COALESCE($1, type),
			priority = COALESCE($2, priority),
			prefix = COALESCE($3, prefix),
			connector_id = COALESCE($4, connector_id),
			enabled = COALESCE($5, enabled),
			updated_by = $6,
			updated_at = $7,
			version = version + 1
		 WHERE id = $8 AND version = $9 AND `+r.softDeleteClause()+`
		 RETURNING id, tenant_id, type, priority, prefix, connector_id, enabled, created_by, updated_by, created_at, updated_at, version`,
		(*string)(input.Type), input.Priority, nullableString(input.Prefix), nullableString(input.ConnectorID),
		input.Enabled, updatedBy, time.Now().UTC(), id, version,
	).Scan(&route.ID, &route.TenantID, &route.Type, &route.Priority, &route.Prefix, &route.ConnectorID,
		&route.Enabled, &route.CreatedBy, &route.UpdatedBy, &route.CreatedAt, &route.UpdatedAt, &route.Version)
	if err == pgx.ErrNoRows {
		if _, err2 := r.GetByID(ctx, id); err2 != nil {
			return nil, domain.ErrNotFound
		}
		return nil, domain.ErrConflict
	}
	if err != nil {
		return nil, fmt.Errorf("update route: %w", err)
	}
	return route, nil
}

func (r *RouteRepository) Delete(ctx context.Context, id string) error {
	q := r.getQuerier(ctx)
	tag, err := q.Exec(ctx,
		`UPDATE routes SET deleted_at = $1, updated_at = $1 WHERE id = $2 AND `+r.softDeleteClause(),
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

func (r *RouteRepository) ListByTenant(ctx context.Context, tenantID string, page domain.Page) (domain.PageResult[domain.Route], error) {
	q := r.getQuerier(ctx)
	rows, err := q.Query(ctx,
		`SELECT id, tenant_id, type, priority, prefix, connector_id, enabled, created_by, updated_by, created_at, updated_at, deleted_at, version
		 FROM routes WHERE tenant_id = $1 AND `+r.softDeleteClause()+`
		 ORDER BY priority DESC, created_at DESC LIMIT $2 OFFSET $3`,
		tenantID, page.Limit, page.Offset,
	)
	if err != nil {
		return domain.PageResult[domain.Route]{}, err
	}
	defer rows.Close()

	var routes []domain.Route
	for rows.Next() {
		var rt domain.Route
		if err := rows.Scan(&rt.ID, &rt.TenantID, &rt.Type, &rt.Priority, &rt.Prefix, &rt.ConnectorID,
			&rt.Enabled, &rt.CreatedBy, &rt.UpdatedBy, &rt.CreatedAt, &rt.UpdatedAt, &rt.DeletedAt, &rt.Version); err != nil {
			return domain.PageResult[domain.Route]{}, err
		}
		routes = append(routes, rt)
	}

	total, err := r.CountByTenant(ctx, tenantID)
	if err != nil {
		return domain.PageResult[domain.Route]{}, err
	}
	return domain.PageResult[domain.Route]{Items: routes, Total: total, Page: page}, nil
}

func (r *RouteRepository) ListByTenantAndType(ctx context.Context, tenantID string, routeType domain.RouteType) ([]domain.Route, error) {
	q := r.getQuerier(ctx)
	rows, err := q.Query(ctx,
		`SELECT id, tenant_id, type, priority, prefix, connector_id, enabled, created_by, updated_by, created_at, updated_at, deleted_at, version
		 FROM routes WHERE tenant_id = $1 AND type = $2 AND `+r.softDeleteClause()+`
		 ORDER BY priority DESC`,
		tenantID, routeType,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var routes []domain.Route
	for rows.Next() {
		var rt domain.Route
		if err := rows.Scan(&rt.ID, &rt.TenantID, &rt.Type, &rt.Priority, &rt.Prefix, &rt.ConnectorID,
			&rt.Enabled, &rt.CreatedBy, &rt.UpdatedBy, &rt.CreatedAt, &rt.UpdatedAt, &rt.DeletedAt, &rt.Version); err != nil {
			return nil, err
		}
		routes = append(routes, rt)
	}
	return routes, nil
}

func (r *RouteRepository) CountByTenant(ctx context.Context, tenantID string) (int64, error) {
	q := r.getQuerier(ctx)
	var count int64
	err := q.QueryRow(ctx,
		`SELECT COUNT(*) FROM routes WHERE tenant_id = $1 AND `+r.softDeleteClause(),
		tenantID,
	).Scan(&count)
	return count, err
}
