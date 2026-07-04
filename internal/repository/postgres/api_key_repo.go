package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// APIKeyRepository implements domain.APIKeyRepository.
type APIKeyRepository struct {
	*BaseRepository
}

func NewAPIKeyRepository(pool *pgxpool.Pool) *APIKeyRepository {
	return &APIKeyRepository{BaseRepository: NewBaseRepository(pool)}
}

func (r *APIKeyRepository) Create(ctx context.Context, input domain.CreateAPIKeyInput, keyPrefix, keyHash string, createdBy string) (*domain.APIKey, error) {
	q := r.getQuerier(ctx)
	k := &domain.APIKey{}
	err := q.QueryRow(ctx,
		`INSERT INTO api_keys (tenant_id, name, key_prefix, key_hash, permissions, ip_whitelist, expires_at, created_by, updated_by)
		 VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7, $8, $9)
		 RETURNING id, tenant_id, name, key_prefix, key_hash, permissions, ip_whitelist, rate_limits, last_used_at, expires_at, enabled, created_by, updated_by, created_at, updated_at, version`,
		input.TenantID, input.Name, keyPrefix, keyHash,
		toJSONBArray(input.Permissions), toJSONBArray(input.IPWhitelist),
		input.ExpiresAt, createdBy, createdBy,
	).Scan(&k.ID, &k.TenantID, &k.Name, &k.KeyPrefix, &k.KeyHash,
		&k.Permissions, &k.IPWhitelist, &k.RateLimits,
		&k.LastUsedAt, &k.ExpiresAt, &k.Enabled,
		&k.CreatedBy, &k.UpdatedBy, &k.CreatedAt, &k.UpdatedAt, &k.Version)
	if err != nil {
		return nil, r.wrapError(err)
	}
	return k, nil
}

func (r *APIKeyRepository) GetByID(ctx context.Context, id string) (*domain.APIKey, error) {
	q := r.getQuerier(ctx)
	k := &domain.APIKey{}
	err := q.QueryRow(ctx,
		`SELECT id, tenant_id, name, key_prefix, key_hash, permissions, ip_whitelist, rate_limits, last_used_at, expires_at, enabled, created_by, updated_by, created_at, updated_at, deleted_at, version
		 FROM api_keys WHERE id = $1 AND `+r.softDeleteClause(),
		id,
	).Scan(&k.ID, &k.TenantID, &k.Name, &k.KeyPrefix, &k.KeyHash,
		&k.Permissions, &k.IPWhitelist, &k.RateLimits,
		&k.LastUsedAt, &k.ExpiresAt, &k.Enabled,
		&k.CreatedBy, &k.UpdatedBy, &k.CreatedAt, &k.UpdatedAt, &k.DeletedAt, &k.Version)
	if err == pgx.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get api key by id: %w", err)
	}
	return k, nil
}

func (r *APIKeyRepository) GetByPrefix(ctx context.Context, prefix string) (*domain.APIKey, error) {
	q := r.getQuerier(ctx)
	k := &domain.APIKey{}
	err := q.QueryRow(ctx,
		`SELECT id, tenant_id, name, key_prefix, key_hash, permissions, ip_whitelist, rate_limits, last_used_at, expires_at, enabled, created_by, updated_by, created_at, updated_at, deleted_at, version
		 FROM api_keys WHERE key_prefix = $1 AND `+r.softDeleteClause(),
		prefix,
	).Scan(&k.ID, &k.TenantID, &k.Name, &k.KeyPrefix, &k.KeyHash,
		&k.Permissions, &k.IPWhitelist, &k.RateLimits,
		&k.LastUsedAt, &k.ExpiresAt, &k.Enabled,
		&k.CreatedBy, &k.UpdatedBy, &k.CreatedAt, &k.UpdatedAt, &k.DeletedAt, &k.Version)
	if err == pgx.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get api key by prefix: %w", err)
	}
	return k, nil
}

func (r *APIKeyRepository) Update(ctx context.Context, id string, input domain.UpdateAPIKeyInput, updatedBy string, version int) (*domain.APIKey, error) {
	q := r.getQuerier(ctx)
	k := &domain.APIKey{}
	err := q.QueryRow(ctx,
		`UPDATE api_keys SET
			name = COALESCE($1, name),
			permissions = CASE WHEN $2::jsonb IS NOT NULL THEN $2::jsonb ELSE permissions END,
			ip_whitelist = CASE WHEN $3::jsonb IS NOT NULL THEN $3::jsonb ELSE ip_whitelist END,
			enabled = COALESCE($4, enabled),
			expires_at = COALESCE($5, expires_at),
			updated_by = $6,
			updated_at = $7,
			version = version + 1
		 WHERE id = $8 AND version = $9 AND `+r.softDeleteClause()+`
		 RETURNING id, tenant_id, name, key_prefix, key_hash, permissions, ip_whitelist, rate_limits, last_used_at, expires_at, enabled, created_by, updated_by, created_at, updated_at, version`,
		nullableString(input.Name), toJSONBArray(input.Permissions), toJSONBArray(input.IPWhitelist),
		input.Enabled, input.ExpiresAt, updatedBy, time.Now().UTC(),
		id, version,
	).Scan(&k.ID, &k.TenantID, &k.Name, &k.KeyPrefix, &k.KeyHash,
		&k.Permissions, &k.IPWhitelist, &k.RateLimits,
		&k.LastUsedAt, &k.ExpiresAt, &k.Enabled,
		&k.CreatedBy, &k.UpdatedBy, &k.CreatedAt, &k.UpdatedAt, &k.Version)
	if err == pgx.ErrNoRows {
		if _, err2 := r.GetByID(ctx, id); err2 != nil {
			return nil, domain.ErrNotFound
		}
		return nil, domain.ErrConflict
	}
	if err != nil {
		return nil, fmt.Errorf("update api key: %w", err)
	}
	return k, nil
}

func (r *APIKeyRepository) Delete(ctx context.Context, id string) error {
	q := r.getQuerier(ctx)
	tag, err := q.Exec(ctx,
		`UPDATE api_keys SET deleted_at = $1, updated_at = $1 WHERE id = $2 AND `+r.softDeleteClause(),
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

func (r *APIKeyRepository) ListByTenant(ctx context.Context, tenantID string, page domain.Page) (domain.PageResult[domain.APIKey], error) {
	q := r.getQuerier(ctx)
	rows, err := q.Query(ctx,
		`SELECT id, tenant_id, name, key_prefix, key_hash, permissions, ip_whitelist, rate_limits, last_used_at, expires_at, enabled, created_by, updated_by, created_at, updated_at, deleted_at, version
		 FROM api_keys WHERE tenant_id = $1 AND `+r.softDeleteClause()+`
		 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		tenantID, page.Limit, page.Offset,
	)
	if err != nil {
		return domain.PageResult[domain.APIKey]{}, err
	}
	defer rows.Close()

	var keys []domain.APIKey
	for rows.Next() {
		var k domain.APIKey
		if err := rows.Scan(&k.ID, &k.TenantID, &k.Name, &k.KeyPrefix, &k.KeyHash,
			&k.Permissions, &k.IPWhitelist, &k.RateLimits,
			&k.LastUsedAt, &k.ExpiresAt, &k.Enabled,
			&k.CreatedBy, &k.UpdatedBy, &k.CreatedAt, &k.UpdatedAt, &k.DeletedAt, &k.Version); err != nil {
			return domain.PageResult[domain.APIKey]{}, err
		}
		keys = append(keys, k)
	}

	total, err := r.CountByTenant(ctx, tenantID)
	if err != nil {
		return domain.PageResult[domain.APIKey]{}, err
	}
	return domain.PageResult[domain.APIKey]{Items: keys, Total: total, Page: page}, nil
}

func (r *APIKeyRepository) UpdateLastUsed(ctx context.Context, id string) error {
	q := r.getQuerier(ctx)
	_, err := q.Exec(ctx,
		`UPDATE api_keys SET last_used_at = $1 WHERE id = $2 AND `+r.softDeleteClause(),
		time.Now().UTC(), id,
	)
	return err
}

func (r *APIKeyRepository) CountByTenant(ctx context.Context, tenantID string) (int64, error) {
	q := r.getQuerier(ctx)
	var count int64
	err := q.QueryRow(ctx,
		`SELECT COUNT(*) FROM api_keys WHERE tenant_id = $1 AND `+r.softDeleteClause(),
		tenantID,
	).Scan(&count)
	return count, err
}

// toJSONBArray converts a string slice to a JSONB-compatible byte slice.
func toJSONBArray(items []string) []byte {
	if items == nil {
		return nil
	}
	if len(items) == 0 {
		return []byte("[]")
	}
	result := "["
	for i, item := range items {
		if i > 0 {
			result += ","
		}
		result += `"` + item + `"`
	}
	result += "]"
	return []byte(result)
}
