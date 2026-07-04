package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// TenantRepository implements domain.TenantRepository.
type TenantRepository struct {
	*BaseRepository
}

func NewTenantRepository(pool *pgxpool.Pool) *TenantRepository {
	return &TenantRepository{BaseRepository: NewBaseRepository(pool)}
}

func (r *TenantRepository) Create(ctx context.Context, input domain.CreateTenantInput, createdBy string) (*domain.Tenant, error) {
	q := r.getQuerier(ctx)
	t := &domain.Tenant{}
	status := domain.TenantStatusActive
	if input.Status != nil {
		status = *input.Status
	}
	settings := input.Settings
	if settings == nil {
		settings = []byte("{}")
	}
	err := q.QueryRow(ctx,
		`INSERT INTO tenants (name, slug, status, settings, created_by, updated_by)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, name, slug, status, settings, balance, created_by, updated_by, created_at, updated_at, version`,
		input.Name, input.Slug, status, settings, createdBy, createdBy,
	).Scan(&t.ID, &t.Name, &t.Slug, &t.Status, &t.Settings, &t.Balance, &t.CreatedBy, &t.UpdatedBy, &t.CreatedAt, &t.UpdatedAt, &t.Version)
	if err != nil {
		return nil, r.wrapError(err)
	}
	return t, nil
}

func (r *TenantRepository) GetByID(ctx context.Context, id string) (*domain.Tenant, error) {
	q := r.getQuerier(ctx)
	t := &domain.Tenant{}
	err := q.QueryRow(ctx,
		`SELECT id, name, slug, status, settings, balance, created_by, updated_by, created_at, updated_at, deleted_at, version
		 FROM tenants WHERE id = $1 AND `+r.softDeleteClause(),
		id,
	).Scan(&t.ID, &t.Name, &t.Slug, &t.Status, &t.Settings, &t.Balance, &t.CreatedBy, &t.UpdatedBy, &t.CreatedAt, &t.UpdatedAt, &t.DeletedAt, &t.Version)
	if err == pgx.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get tenant by id: %w", err)
	}
	return t, nil
}

func (r *TenantRepository) GetBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	q := r.getQuerier(ctx)
	t := &domain.Tenant{}
	err := q.QueryRow(ctx,
		`SELECT id, name, slug, status, settings, balance, created_by, updated_by, created_at, updated_at, deleted_at, version
		 FROM tenants WHERE slug = $1 AND `+r.softDeleteClause(),
		slug,
	).Scan(&t.ID, &t.Name, &t.Slug, &t.Status, &t.Settings, &t.Balance, &t.CreatedBy, &t.UpdatedBy, &t.CreatedAt, &t.UpdatedAt, &t.DeletedAt, &t.Version)
	if err == pgx.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get tenant by slug: %w", err)
	}
	return t, nil
}

func (r *TenantRepository) Update(ctx context.Context, id string, input domain.UpdateTenantInput, updatedBy string, version int) (*domain.Tenant, error) {
	q := r.getQuerier(ctx)
	t := &domain.Tenant{}
	err := q.QueryRow(ctx,
		`UPDATE tenants SET
			name = COALESCE($1, name),
			slug = COALESCE($2, slug),
			status = COALESCE($3, status),
			settings = CASE WHEN $4::jsonb IS NOT NULL THEN $4::jsonb ELSE settings END,
			updated_by = $5,
			updated_at = $6,
			version = version + 1
		 WHERE id = $7 AND version = $8 AND `+r.softDeleteClause()+`
		 RETURNING id, name, slug, status, settings, balance, created_by, updated_by, created_at, updated_at, version`,
		nullableString(input.Name), nullableString(input.Slug), (*string)(input.Status),
		input.Settings, updatedBy, time.Now().UTC(),
		id, version,
	).Scan(&t.ID, &t.Name, &t.Slug, &t.Status, &t.Settings, &t.Balance, &t.CreatedBy, &t.UpdatedBy, &t.CreatedAt, &t.UpdatedAt, &t.Version)
	if err == pgx.ErrNoRows {
		if _, err2 := r.GetByID(ctx, id); err2 != nil {
			return nil, domain.ErrNotFound
		}
		return nil, domain.ErrConflict
	}
	if err != nil {
		return nil, fmt.Errorf("update tenant: %w", err)
	}
	return t, nil
}

func (r *TenantRepository) Delete(ctx context.Context, id string) error {
	q := r.getQuerier(ctx)
	tag, err := q.Exec(ctx,
		`UPDATE tenants SET deleted_at = $1, updated_at = $1 WHERE id = $2 AND `+r.softDeleteClause(),
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

func (r *TenantRepository) List(ctx context.Context, page domain.Page) (domain.PageResult[domain.Tenant], error) {
	q := r.getQuerier(ctx)
	rows, err := q.Query(ctx,
		`SELECT id, name, slug, status, settings, balance, created_by, updated_by, created_at, updated_at, deleted_at, version
		 FROM tenants WHERE `+r.softDeleteClause()+`
		 ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		page.Limit, page.Offset,
	)
	if err != nil {
		return domain.PageResult[domain.Tenant]{}, fmt.Errorf("list tenants: %w", err)
	}
	defer rows.Close()

	var tenants []domain.Tenant
	for rows.Next() {
		var t domain.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.Status, &t.Settings, &t.Balance, &t.CreatedBy, &t.UpdatedBy, &t.CreatedAt, &t.UpdatedAt, &t.DeletedAt, &t.Version); err != nil {
			return domain.PageResult[domain.Tenant]{}, err
		}
		tenants = append(tenants, t)
	}

	total, err := r.Count(ctx)
	if err != nil {
		return domain.PageResult[domain.Tenant]{}, err
	}
	return domain.PageResult[domain.Tenant]{Items: tenants, Total: total, Page: page}, nil
}

func (r *TenantRepository) Count(ctx context.Context) (int64, error) {
	q := r.getQuerier(ctx)
	var count int64
	err := q.QueryRow(ctx,
		`SELECT COUNT(*) FROM tenants WHERE `+r.softDeleteClause(),
	).Scan(&count)
	return count, err
}


