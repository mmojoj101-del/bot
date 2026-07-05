package domain

import (
	"context"
	"time"
)

// TxManager manages database transactions.
type TxManager interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// UserRepository defines the interface for user persistence.
type UserRepository interface {
	Create(ctx context.Context, input CreateUserInput, passwordHash string) (*User, error)
	GetByID(ctx context.Context, id string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	Update(ctx context.Context, id string, input UpdateUserInput, version int) (*User, error)
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, page Page) (PageResult[User], error)
	UpdateLastLogin(ctx context.Context, id string) error
	UpdatePassword(ctx context.Context, id string, passwordHash string, version int) error
	Count(ctx context.Context) (int64, error)
}

// TenantRepository defines the interface for tenant persistence.
type TenantRepository interface {
	Create(ctx context.Context, input CreateTenantInput, createdBy string) (*Tenant, error)
	GetByID(ctx context.Context, id string) (*Tenant, error)
	GetBySlug(ctx context.Context, slug string) (*Tenant, error)
	Update(ctx context.Context, id string, input UpdateTenantInput, updatedBy string, version int) (*Tenant, error)
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, page Page) (PageResult[Tenant], error)
	Count(ctx context.Context) (int64, error)
}

// TenantMemberRepository defines the interface for tenant membership persistence.
type TenantMemberRepository interface {
	Add(ctx context.Context, input AddMemberInput) (*TenantMember, error)
	Get(ctx context.Context, tenantID, userID string) (*TenantMember, error)
	UpdateRole(ctx context.Context, tenantID, userID string, role MemberRole) error
	Remove(ctx context.Context, tenantID, userID string) error
	ListByTenant(ctx context.Context, tenantID string, page Page) (PageResult[TenantMember], error)
	ListByUser(ctx context.Context, userID string) ([]TenantMember, error)
	CountByTenant(ctx context.Context, tenantID string) (int64, error)
}

// APIKeyRepository defines the interface for API key persistence.
type APIKeyRepository interface {
	Create(ctx context.Context, input CreateAPIKeyInput, keyPrefix, keyHash string, createdBy string) (*APIKey, error)
	GetByID(ctx context.Context, id string) (*APIKey, error)
	GetByPrefix(ctx context.Context, prefix string) (*APIKey, error)
	Update(ctx context.Context, id string, input UpdateAPIKeyInput, updatedBy string, version int) (*APIKey, error)
	Delete(ctx context.Context, id string) error
	ListByTenant(ctx context.Context, tenantID string, page Page) (PageResult[APIKey], error)
	UpdateLastUsed(ctx context.Context, id string) error
	CountByTenant(ctx context.Context, tenantID string) (int64, error)
}

// ConnectorRepository defines the interface for connector persistence.
type ConnectorRepository interface {
	Create(ctx context.Context, input CreateConnectorInput, createdBy string) (*Connector, error)
	GetByID(ctx context.Context, id string) (*Connector, error)
	Update(ctx context.Context, id string, input UpdateConnectorInput, updatedBy string, version int) (*Connector, error)
	Delete(ctx context.Context, id string) error
	ListByTenant(ctx context.Context, filter ConnectorFilter) (PageResult[Connector], error)
	CountByTenant(ctx context.Context, tenantID string) (int64, error)
}

// RouteRepository defines the interface for route persistence.
type RouteRepository interface {
	Create(ctx context.Context, input CreateRouteInput, createdBy string) (*Route, error)
	GetByID(ctx context.Context, id string) (*Route, error)
	Update(ctx context.Context, id string, input UpdateRouteInput, updatedBy string, version int) (*Route, error)
	Delete(ctx context.Context, id string) error
	ListByTenant(ctx context.Context, filter RouteFilter) (PageResult[Route], error)
	ListByTenantAndType(ctx context.Context, tenantID string, routeType RouteType) ([]Route, error)
	CountByTenant(ctx context.Context, tenantID string) (int64, error)
}

// MessageRepository defines the interface for message persistence.
type MessageRepository interface {
	Create(ctx context.Context, input CreateMessageInput, createdBy string) (*Message, error)
	GetByID(ctx context.Context, id string) (*Message, error)
	GetByClientRef(ctx context.Context, tenantID, clientRef string) (*Message, error)
	GetByExternalID(ctx context.Context, externalID string) (*Message, error)
	UpdateStatus(ctx context.Context, id string, input UpdateMessageInput, version int) (*Message, error)
	AppendDLR(ctx context.Context, dlr *DLRRecord) error
	List(ctx context.Context, filter MessageFilter) (PageResult[Message], error)
	Count(ctx context.Context, filter MessageFilter) (int64, error)
	Delete(ctx context.Context, id string) error
}

// OutboxRepository defines the interface for outbox event persistence.
type OutboxRepository interface {
	Create(ctx context.Context, event *OutboxEvent) error
	GetPending(ctx context.Context, limit int) ([]OutboxEvent, error)
	MarkPublished(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string, errMsg string) error
	DeletePublished(ctx context.Context, before time.Time) error
}

// AuditLogRepository defines the interface for audit log persistence.
type AuditLogRepository interface {
	Create(ctx context.Context, log *AuditLog) error
	GetByID(ctx context.Context, id string) (*AuditLog, error)
	ListByTenant(ctx context.Context, tenantID string, page CursorPage) (CursorPageResult[AuditLog], error)
	ListByUser(ctx context.Context, userID string, page CursorPage) (CursorPageResult[AuditLog], error)
}

// RefreshTokenRepository defines the interface for refresh token persistence.
type RefreshTokenRepository interface {
	Create(ctx context.Context, token *RefreshToken) error
	GetByJTI(ctx context.Context, jti string) (*RefreshToken, error)
	Revoke(ctx context.Context, jti string) error
	RevokeAllByUser(ctx context.Context, userID string) error
	ListByUser(ctx context.Context, userID string) ([]RefreshToken, error)
	DeleteExpired(ctx context.Context) (int64, error)
}
