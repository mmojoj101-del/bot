package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// RefreshTokenRepository implements domain.RefreshTokenRepository.
type RefreshTokenRepository struct {
	*BaseRepository
}

func NewRefreshTokenRepository(pool *pgxpool.Pool) *RefreshTokenRepository {
	return &RefreshTokenRepository{BaseRepository: NewBaseRepository(pool)}
}

func (r *RefreshTokenRepository) Create(ctx context.Context, token *domain.RefreshToken) error {
	q := r.getQuerier(ctx)
	_, err := q.Exec(ctx,
		`INSERT INTO refresh_tokens (id, user_id, tenant_id, token_hash, jti, device_name, ip_address, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		token.ID, token.UserID, token.TenantID, token.TokenHash, token.JTI,
		token.DeviceName, token.IPAddress, token.ExpiresAt,
	)
	return err
}

func (r *RefreshTokenRepository) GetByJTI(ctx context.Context, jti string) (*domain.RefreshToken, error) {
	q := r.getQuerier(ctx)
	t := &domain.RefreshToken{}
	err := q.QueryRow(ctx,
		`SELECT id, user_id, tenant_id, token_hash, jti, device_name, ip_address, expires_at, last_used_at, created_at, revoked_at
		 FROM refresh_tokens WHERE jti = $1`,
		jti,
	).Scan(&t.ID, &t.UserID, &t.TenantID, &t.TokenHash, &t.JTI, &t.DeviceName, &t.IPAddress,
		&t.ExpiresAt, &t.LastUsedAt, &t.CreatedAt, &t.RevokedAt)
	if err == pgx.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get refresh token by jti: %w", err)
	}
	return t, nil
}

func (r *RefreshTokenRepository) Revoke(ctx context.Context, jti string) error {
	q := r.getQuerier(ctx)
	now := time.Now().UTC()
	_, err := q.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = $1 WHERE jti = $2 AND revoked_at IS NULL`,
		now, jti,
	)
	return err
}

func (r *RefreshTokenRepository) RevokeAllByUser(ctx context.Context, userID string) error {
	q := r.getQuerier(ctx)
	now := time.Now().UTC()
	_, err := q.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = $1 WHERE user_id = $2 AND revoked_at IS NULL`,
		now, userID,
	)
	return err
}

func (r *RefreshTokenRepository) ListByUser(ctx context.Context, userID string) ([]domain.RefreshToken, error) {
	q := r.getQuerier(ctx)
	rows, err := q.Query(ctx,
		`SELECT id, user_id, tenant_id, token_hash, jti, device_name, ip_address, expires_at, last_used_at, created_at, revoked_at
		 FROM refresh_tokens WHERE user_id = $1 AND revoked_at IS NULL ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []domain.RefreshToken
	for rows.Next() {
		var t domain.RefreshToken
		if err := rows.Scan(&t.ID, &t.UserID, &t.TenantID, &t.TokenHash, &t.JTI, &t.DeviceName, &t.IPAddress,
			&t.ExpiresAt, &t.LastUsedAt, &t.CreatedAt, &t.RevokedAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, nil
}

func (r *RefreshTokenRepository) DeleteExpired(ctx context.Context) (int64, error) {
	q := r.getQuerier(ctx)
	tag, err := q.Exec(ctx,
		`DELETE FROM refresh_tokens WHERE expires_at < $1`,
		time.Now().UTC(),
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
