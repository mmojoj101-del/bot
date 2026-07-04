package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// UserRepository implements domain.UserRepository.
type UserRepository struct {
	*BaseRepository
}

// NewUserRepository creates a new user repository.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{
		BaseRepository: NewBaseRepository(pool),
	}
}

func (r *UserRepository) Create(ctx context.Context, input domain.CreateUserInput, passwordHash string) (*domain.User, error) {
	q := r.getQuerier(ctx)
	user := &domain.User{}
	err := q.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, name, status, is_super_admin, password_changed_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, email, password_hash, name, status, is_super_admin, last_login_at, password_changed_at, created_at, updated_at, version`,
		input.Email, passwordHash, input.Name, domain.UserStatusActive, false, time.Now().UTC(),
	).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name,
		&user.Status, &user.IsSuperAdmin, &user.LastLoginAt,
		&user.PasswordChangedAt, &user.CreatedAt, &user.UpdatedAt, &user.Version,
	)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", r.wrapError(err))
	}
	return user, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	q := r.getQuerier(ctx)
	user := &domain.User{}
	err := q.QueryRow(ctx,
		`SELECT id, email, password_hash, name, status, is_super_admin, last_login_at, password_changed_at, created_at, updated_at, deleted_at, version
		 FROM users WHERE id = $1 AND `+r.softDeleteClause(),
		id,
	).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name,
		&user.Status, &user.IsSuperAdmin, &user.LastLoginAt,
		&user.PasswordChangedAt, &user.CreatedAt, &user.UpdatedAt, &user.DeletedAt, &user.Version,
	)
	if err == pgx.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return user, nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	q := r.getQuerier(ctx)
	user := &domain.User{}
	err := q.QueryRow(ctx,
		`SELECT id, email, password_hash, name, status, is_super_admin, last_login_at, password_changed_at, created_at, updated_at, deleted_at, version
		 FROM users WHERE email = $1 AND `+r.softDeleteClause(),
		email,
	).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name,
		&user.Status, &user.IsSuperAdmin, &user.LastLoginAt,
		&user.PasswordChangedAt, &user.CreatedAt, &user.UpdatedAt, &user.DeletedAt, &user.Version,
	)
	if err == pgx.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return user, nil
}

func (r *UserRepository) Update(ctx context.Context, id string, input domain.UpdateUserInput, version int) (*domain.User, error) {
	q := r.getQuerier(ctx)
	user := &domain.User{}
	err := q.QueryRow(ctx,
		`UPDATE users SET
			name = COALESCE($1, name),
			status = COALESCE($2, status),
			updated_at = $3,
			version = version + 1
		 WHERE id = $4 AND version = $5 AND `+r.softDeleteClause()+`
		 RETURNING id, email, password_hash, name, status, is_super_admin, last_login_at, password_changed_at, created_at, updated_at, version`,
		nullableString(input.Name), nullableStatus(input.Status), time.Now().UTC(),
		id, version,
	).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name,
		&user.Status, &user.IsSuperAdmin, &user.LastLoginAt,
		&user.PasswordChangedAt, &user.CreatedAt, &user.UpdatedAt, &user.Version,
	)
	if err == pgx.ErrNoRows {
		// Check if the record exists to differentiate not found vs version conflict
		if _, err2 := r.GetByID(ctx, id); err2 != nil {
			return nil, domain.ErrNotFound
		}
		return nil, domain.ErrConflict
	}
	if err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}
	return user, nil
}

func (r *UserRepository) Delete(ctx context.Context, id string) error {
	q := r.getQuerier(ctx)
	tag, err := q.Exec(ctx,
		`UPDATE users SET deleted_at = $1, updated_at = $1 WHERE id = $2 AND `+r.softDeleteClause(),
		time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("soft delete user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *UserRepository) List(ctx context.Context, page domain.Page) (domain.PageResult[domain.User], error) {
	q := r.getQuerier(ctx)
	rows, err := q.Query(ctx,
		`SELECT id, email, password_hash, name, status, is_super_admin, last_login_at, password_changed_at, created_at, updated_at, deleted_at, version
		 FROM users WHERE `+r.softDeleteClause()+`
		 ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		page.Limit, page.Offset,
	)
	if err != nil {
		return domain.PageResult[domain.User]{}, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Status, &u.IsSuperAdmin, &u.LastLoginAt, &u.PasswordChangedAt, &u.CreatedAt, &u.UpdatedAt, &u.DeletedAt, &u.Version); err != nil {
			return domain.PageResult[domain.User]{}, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}

	total, err := r.Count(ctx)
	if err != nil {
		return domain.PageResult[domain.User]{}, err
	}

	return domain.PageResult[domain.User]{Items: users, Total: total, Page: page}, nil
}

func (r *UserRepository) UpdateLastLogin(ctx context.Context, id string) error {
	q := r.getQuerier(ctx)
	_, err := q.Exec(ctx,
		`UPDATE users SET last_login_at = $1 WHERE id = $2 AND `+r.softDeleteClause(),
		time.Now().UTC(), id,
	)
	return err
}

func (r *UserRepository) UpdatePassword(ctx context.Context, id string, passwordHash string, version int) error {
	q := r.getQuerier(ctx)
	tag, err := q.Exec(ctx,
		`UPDATE users SET password_hash = $1, password_changed_at = $2, version = version + 1 WHERE id = $3 AND version = $4 AND `+r.softDeleteClause(),
		passwordHash, time.Now().UTC(), id, version,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return nil
}

func (r *UserRepository) Count(ctx context.Context) (int64, error) {
	q := r.getQuerier(ctx)
	var count int64
	err := q.QueryRow(ctx,
		`SELECT COUNT(*) FROM users WHERE `+r.softDeleteClause(),
	).Scan(&count)
	return count, err
}

// Helper functions
func nullableString(s *string) *string {
	if s == nil {
		return nil
	}
	return s
}

func nullableStatus(s *domain.UserStatus) *domain.UserStatus {
	if s == nil {
		return nil
	}
	return s
}
