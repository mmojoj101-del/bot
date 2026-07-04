package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// TenantMemberRepository implements domain.TenantMemberRepository.
type TenantMemberRepository struct {
	*BaseRepository
}

func NewTenantMemberRepository(pool *pgxpool.Pool) *TenantMemberRepository {
	return &TenantMemberRepository{BaseRepository: NewBaseRepository(pool)}
}

func (r *TenantMemberRepository) Add(ctx context.Context, input domain.AddMemberInput) (*domain.TenantMember, error) {
	q := r.getQuerier(ctx)
	m := &domain.TenantMember{}
	err := q.QueryRow(ctx,
		`INSERT INTO tenant_members (tenant_id, user_id, role)
		 VALUES ($1, $2, $3)
		 RETURNING id, tenant_id, user_id, role, joined_at, created_at, updated_at, version`,
		input.TenantID, input.UserID, input.Role,
	).Scan(&m.ID, &m.TenantID, &m.UserID, &m.Role, &m.JoinedAt, &m.CreatedAt, &m.UpdatedAt, &m.Version)
	if err != nil {
		return nil, fmt.Errorf("add member: %w", r.wrapError(err))
	}
	return m, nil
}

func (r *TenantMemberRepository) Get(ctx context.Context, tenantID, userID string) (*domain.TenantMember, error) {
	q := r.getQuerier(ctx)
	m := &domain.TenantMember{}
	err := q.QueryRow(ctx,
		`SELECT id, tenant_id, user_id, role, joined_at, created_at, updated_at, deleted_at, version
		 FROM tenant_members WHERE tenant_id = $1 AND user_id = $2 AND `+r.softDeleteClause(),
		tenantID, userID,
	).Scan(&m.ID, &m.TenantID, &m.UserID, &m.Role, &m.JoinedAt, &m.CreatedAt, &m.UpdatedAt, &m.DeletedAt, &m.Version)
	if err == pgx.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get member: %w", err)
	}
	return m, nil
}

func (r *TenantMemberRepository) UpdateRole(ctx context.Context, tenantID, userID string, role domain.MemberRole) error {
	q := r.getQuerier(ctx)
	tag, err := q.Exec(ctx,
		`UPDATE tenant_members SET role = $1, updated_at = $2 WHERE tenant_id = $3 AND user_id = $4 AND `+r.softDeleteClause(),
		role, time.Now().UTC(), tenantID, userID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *TenantMemberRepository) Remove(ctx context.Context, tenantID, userID string) error {
	q := r.getQuerier(ctx)
	tag, err := q.Exec(ctx,
		`UPDATE tenant_members SET deleted_at = $1, updated_at = $1 WHERE tenant_id = $2 AND user_id = $3 AND `+r.softDeleteClause(),
		time.Now().UTC(), tenantID, userID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *TenantMemberRepository) ListByTenant(ctx context.Context, tenantID string, page domain.Page) (domain.PageResult[domain.TenantMember], error) {
	q := r.getQuerier(ctx)
	rows, err := q.Query(ctx,
		`SELECT id, tenant_id, user_id, role, joined_at, created_at, updated_at, deleted_at, version
		 FROM tenant_members WHERE tenant_id = $1 AND `+r.softDeleteClause()+`
		 ORDER BY joined_at DESC LIMIT $2 OFFSET $3`,
		tenantID, page.Limit, page.Offset,
	)
	if err != nil {
		return domain.PageResult[domain.TenantMember]{}, err
	}
	defer rows.Close()

	var members []domain.TenantMember
	for rows.Next() {
		var m domain.TenantMember
		if err := rows.Scan(&m.ID, &m.TenantID, &m.UserID, &m.Role, &m.JoinedAt, &m.CreatedAt, &m.UpdatedAt, &m.DeletedAt, &m.Version); err != nil {
			return domain.PageResult[domain.TenantMember]{}, err
		}
		members = append(members, m)
	}

	total, err := r.CountByTenant(ctx, tenantID)
	if err != nil {
		return domain.PageResult[domain.TenantMember]{}, err
	}
	return domain.PageResult[domain.TenantMember]{Items: members, Total: total, Page: page}, nil
}

func (r *TenantMemberRepository) ListByUser(ctx context.Context, userID string) ([]domain.TenantMember, error) {
	q := r.getQuerier(ctx)
	rows, err := q.Query(ctx,
		`SELECT id, tenant_id, user_id, role, joined_at, created_at, updated_at, deleted_at, version
		 FROM tenant_members WHERE user_id = $1 AND `+r.softDeleteClause(),
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []domain.TenantMember
	for rows.Next() {
		var m domain.TenantMember
		if err := rows.Scan(&m.ID, &m.TenantID, &m.UserID, &m.Role, &m.JoinedAt, &m.CreatedAt, &m.UpdatedAt, &m.DeletedAt, &m.Version); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, nil
}

func (r *TenantMemberRepository) CountByTenant(ctx context.Context, tenantID string) (int64, error) {
	q := r.getQuerier(ctx)
	var count int64
	err := q.QueryRow(ctx,
		`SELECT COUNT(*) FROM tenant_members WHERE tenant_id = $1 AND `+r.softDeleteClause(),
		tenantID,
	).Scan(&count)
	return count, err
}
