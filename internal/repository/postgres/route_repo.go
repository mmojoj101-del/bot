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
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	route := &domain.Route{}

	strategy := input.Strategy
	if strategy == "" {
		strategy = domain.RouteStrategyStatic
	}
	weight := input.Weight
	if weight < 1 {
		weight = 1
	}

	err := q.QueryRow(ctx,
		`INSERT INTO routes (tenant_id, name, type, strategy, weight, priority, prefix, connector_id, created_by, updated_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id, tenant_id, name, type, strategy, weight, priority, prefix, connector_id, enabled, created_by, updated_by, created_at, updated_at, version`,
		input.TenantID, input.Name, input.Type, strategy, weight, input.Priority, input.Prefix, input.ConnectorID,
		createdBy, createdBy,
	).Scan(&route.ID, &route.TenantID, &route.Name, &route.Type, &route.Strategy, &route.Weight,
		&route.Priority, &route.Prefix, &route.ConnectorID, &route.Enabled,
		&route.CreatedBy, &route.UpdatedBy, &route.CreatedAt, &route.UpdatedAt, &route.Version)
	if err != nil {
		return nil, r.wrapError(err)
	}
	return route, nil
}

func (r *RouteRepository) GetByID(ctx context.Context, id string) (*domain.Route, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	route := &domain.Route{}
	err := q.QueryRow(ctx,
		`SELECT id, tenant_id, name, type, strategy, weight, priority, prefix, connector_id, enabled, created_by, updated_by, created_at, updated_at, deleted_at, version
		 FROM routes WHERE id = $1 AND `+r.softDeleteClause(),
		id,
	).Scan(&route.ID, &route.TenantID, &route.Name, &route.Type, &route.Strategy, &route.Weight,
		&route.Priority, &route.Prefix, &route.ConnectorID, &route.Enabled,
		&route.CreatedBy, &route.UpdatedBy, &route.CreatedAt, &route.UpdatedAt, &route.DeletedAt, &route.Version)
	if err == pgx.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get route by id: %w", err)
	}
	return route, nil
}

func (r *RouteRepository) Update(ctx context.Context, id string, input domain.UpdateRouteInput, updatedBy string, version int) (*domain.Route, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	route := &domain.Route{}
	err := q.QueryRow(ctx,
		`UPDATE routes SET
			name = COALESCE($1, name),
			type = COALESCE($2, type),
			strategy = COALESCE($3, strategy),
			weight = COALESCE($4, weight),
			priority = COALESCE($5, priority),
			prefix = COALESCE($6, prefix),
			connector_id = COALESCE($7, connector_id),
			enabled = COALESCE($8, enabled),
			updated_by = $9,
			updated_at = $10,
			version = version + 1
		 WHERE id = $11 AND version = $12 AND `+r.softDeleteClause()+`
		 RETURNING id, tenant_id, name, type, strategy, weight, priority, prefix, connector_id, enabled, created_by, updated_by, created_at, updated_at, version`,
		nullableString(input.Name), (*string)(input.Type), (*string)(input.Strategy), input.Weight,
		input.Priority, nullableString(input.Prefix), nullableString(input.ConnectorID),
		input.Enabled, updatedBy, time.Now().UTC(), id, version,
	).Scan(&route.ID, &route.TenantID, &route.Name, &route.Type, &route.Strategy, &route.Weight,
		&route.Priority, &route.Prefix, &route.ConnectorID, &route.Enabled,
		&route.CreatedBy, &route.UpdatedBy, &route.CreatedAt, &route.UpdatedAt, &route.Version)
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
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

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

func (r *RouteRepository) ListByTenant(ctx context.Context, filter domain.RouteFilter) (domain.PageResult[domain.Route], error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)

	query := `SELECT id, tenant_id, name, type, strategy, weight, priority, prefix, connector_id, enabled, created_by, updated_by, created_at, updated_at, deleted_at, version
		 FROM routes WHERE tenant_id = $1 AND ` + r.softDeleteClause()
	countQuery := `SELECT COUNT(*) FROM routes WHERE tenant_id = $1 AND ` + r.softDeleteClause()

	args := []interface{}{filter.TenantID}
	argIdx := 2

	if filter.Type != nil {
		query += fmt.Sprintf(" AND type = $%d", argIdx)
		countQuery += fmt.Sprintf(" AND type = $%d", argIdx)
		args = append(args, *filter.Type)
		argIdx++
	}
	if filter.Strategy != nil {
		query += fmt.Sprintf(" AND strategy = $%d", argIdx)
		countQuery += fmt.Sprintf(" AND strategy = $%d", argIdx)
		args = append(args, *filter.Strategy)
		argIdx++
	}
	if filter.Prefix != "" {
		query += fmt.Sprintf(" AND prefix LIKE $%d", argIdx)
		countQuery += fmt.Sprintf(" AND prefix LIKE $%d", argIdx)
		args = append(args, filter.Prefix+"%")
		argIdx++
	}
	if filter.ConnectorID != "" {
		query += fmt.Sprintf(" AND connector_id = $%d", argIdx)
		countQuery += fmt.Sprintf(" AND connector_id = $%d", argIdx)
		args = append(args, filter.ConnectorID)
		argIdx++
	}
	if filter.Search != "" {
		searchPattern := "%" + filter.Search + "%"
		query += fmt.Sprintf(" AND (name ILIKE $%d OR prefix ILIKE $%d)", argIdx, argIdx+1)
		countQuery += fmt.Sprintf(" AND (name ILIKE $%d OR prefix ILIKE $%d)", argIdx, argIdx+1)
		args = append(args, searchPattern, searchPattern)
		argIdx += 2
	}

	// Count
	var total int64
	if err := q.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return domain.PageResult[domain.Route]{}, err
	}

	// Paginated query — order by priority descending, then prefix length (longest match first)
	query += ` ORDER BY priority DESC, length(prefix) DESC, created_at DESC LIMIT $` +
		fmt.Sprintf("%d", argIdx) + ` OFFSET $` + fmt.Sprintf("%d", argIdx+1)
	args = append(args, filter.Page.Limit, filter.Page.Offset)

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return domain.PageResult[domain.Route]{}, err
	}
	defer rows.Close()

	var routes []domain.Route
	for rows.Next() {
		var rt domain.Route
		if err := rows.Scan(&rt.ID, &rt.TenantID, &rt.Name, &rt.Type, &rt.Strategy, &rt.Weight,
			&rt.Priority, &rt.Prefix, &rt.ConnectorID, &rt.Enabled,
			&rt.CreatedBy, &rt.UpdatedBy, &rt.CreatedAt, &rt.UpdatedAt, &rt.DeletedAt, &rt.Version); err != nil {
			return domain.PageResult[domain.Route]{}, err
		}
		routes = append(routes, rt)
	}

	return domain.PageResult[domain.Route]{Items: routes, Total: total, Page: filter.Page}, nil
}

func (r *RouteRepository) ListByTenantAndType(ctx context.Context, tenantID string, routeType domain.RouteType) ([]domain.Route, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	rows, err := q.Query(ctx,
		`SELECT id, tenant_id, name, type, strategy, weight, priority, prefix, connector_id, enabled, created_by, updated_by, created_at, updated_at, deleted_at, version
		 FROM routes WHERE tenant_id = $1 AND type = $2 AND `+r.softDeleteClause()+`
		 ORDER BY priority DESC, length(prefix) DESC`,
		tenantID, routeType,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var routes []domain.Route
	for rows.Next() {
		var rt domain.Route
		if err := rows.Scan(&rt.ID, &rt.TenantID, &rt.Name, &rt.Type, &rt.Strategy, &rt.Weight,
			&rt.Priority, &rt.Prefix, &rt.ConnectorID, &rt.Enabled,
			&rt.CreatedBy, &rt.UpdatedBy, &rt.CreatedAt, &rt.UpdatedAt, &rt.DeletedAt, &rt.Version); err != nil {
			return nil, err
		}
		routes = append(routes, rt)
	}
	return routes, nil
}

func (r *RouteRepository) CountByTenant(ctx context.Context, tenantID string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := r.getQuerier(ctx)
	var count int64
	err := q.QueryRow(ctx,
		`SELECT COUNT(*) FROM routes WHERE tenant_id = $1 AND `+r.softDeleteClause(),
		tenantID,
	).Scan(&count)
	return count, err
}
