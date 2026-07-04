package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/event"
)

type mockConnectorRepo struct {
	mu          sync.Mutex
	connectors  map[string]*domain.Connector
	err         error
	createErr   error
	getErr      error
	updateErr   error
	deleteErr   error
}

func newMockConnectorRepo() *mockConnectorRepo {
	return &mockConnectorRepo{
		connectors: make(map[string]*domain.Connector),
	}
}

func (r *mockConnectorRepo) Create(ctx context.Context, input domain.CreateConnectorInput, createdBy string) (*domain.Connector, error) {
	if r.createErr != nil {
		return nil, r.createErr
	}
	status := domain.ConnectorStatusActive
	if input.Status != nil {
		status = *input.Status
	}
	c := &domain.Connector{
		BaseModel: domain.BaseModel{
			ID:        uuid.New().String(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Version:   1,
		},
		TenantID:  input.TenantID,
		Type:      input.Type,
		Name:      input.Name,
		Status:    status,
		Config:    input.Config,
		Enabled:   true,
		CreatedBy: createdBy,
		UpdatedBy: createdBy,
	}
	r.mu.Lock()
	r.connectors[c.ID] = c
	r.mu.Unlock()
	return c, nil
}

func (r *mockConnectorRepo) GetByID(ctx context.Context, id string) (*domain.Connector, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.connectors[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return c, nil
}

func (r *mockConnectorRepo) Update(ctx context.Context, id string, input domain.UpdateConnectorInput, updatedBy string, version int) (*domain.Connector, error) {
	if r.updateErr != nil {
		return nil, r.updateErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.connectors[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	if input.Name != nil {
		c.Name = *input.Name
	}
	if input.Type != nil {
		c.Type = *input.Type
	}
	if input.Status != nil {
		c.Status = *input.Status
	}
	if input.Enabled != nil {
		c.Enabled = *input.Enabled
	}
	if input.Config != nil {
		c.Config = input.Config
	}
	c.UpdatedBy = updatedBy
	c.Version++
	return c, nil
}

func (r *mockConnectorRepo) Delete(ctx context.Context, id string) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.connectors, id)
	return nil
}

func (r *mockConnectorRepo) ListByTenant(ctx context.Context, tenantID string, page domain.Page) (domain.PageResult[domain.Connector], error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var items []domain.Connector
	for _, c := range r.connectors {
		if c.TenantID == tenantID {
			items = append(items, *c)
		}
	}
	return domain.PageResult[domain.Connector]{Items: items, Total: int64(len(items)), Page: page}, nil
}

func (r *mockConnectorRepo) CountByTenant(ctx context.Context, tenantID string) (int64, error) {
	return 0, nil
}

func setupConnectorService() (*ConnectorService, *mockConnectorRepo, *mockAuditRepo, *event.MemoryBus) {
	repo := newMockConnectorRepo()
	auditRepo := &mockAuditRepo{}
	bus := event.NewMemoryBus()
	clock := &mockClock{now: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)}

	svc := NewConnectorService(repo, auditRepo, bus, clock)
	return svc, repo, auditRepo, bus
}

func TestCreateConnector_Success(t *testing.T) {
	svc, _, _, _ := setupConnectorService()

	connector, err := svc.Create(context.Background(), domain.CreateConnectorInput{
		TenantID: "tenant-1",
		Type:     domain.ConnectorTypeHTTPClient,
		Name:     "My HTTP Connector",
		Config:   []byte(`{"host":"example.com","port":8080}`),
	}, "user-1", uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if connector.Name != "My HTTP Connector" {
		t.Fatalf("name = %s, want My HTTP Connector", connector.Name)
	}
	if connector.Type != domain.ConnectorTypeHTTPClient {
		t.Fatalf("type = %s, want http_client", connector.Type)
	}
	if connector.Status != domain.ConnectorStatusActive {
		t.Fatalf("status = %s, want active", connector.Status)
	}
	if !connector.Enabled {
		t.Fatal("connector should be enabled by default")
	}
}

func TestCreateConnector_AllTypes(t *testing.T) {
	svc, _, _, _ := setupConnectorService()

	types := []domain.ConnectorType{
		domain.ConnectorTypeSMPPClient,
		domain.ConnectorTypeSMPPServer,
		domain.ConnectorTypeHTTPClient,
		domain.ConnectorTypeHTTPServer,
		domain.ConnectorTypeSIPClient,
		domain.ConnectorTypeSIPServer,
		domain.ConnectorTypeMock,
	}

	for _, ct := range types {
		connector, err := svc.Create(context.Background(), domain.CreateConnectorInput{
			TenantID: "tenant-1",
			Type:     ct,
			Name:     string(ct) + "-connector",
		}, "user-1", uuid.New().String(), "127.0.0.1")
		if err != nil {
			t.Fatalf("Create(%s) failed: %v", ct, err)
		}
		if connector.Type != ct {
			t.Fatalf("type = %s, want %s", connector.Type, ct)
		}
	}
}

func TestCreateConnector_WithCustomStatus(t *testing.T) {
	svc, _, _, _ := setupConnectorService()
	status := domain.ConnectorStatusDisabled

	connector, err := svc.Create(context.Background(), domain.CreateConnectorInput{
		TenantID: "tenant-1",
		Type:     domain.ConnectorTypeHTTPClient,
		Name:     "Disabled Connector",
		Status:   &status,
	}, "user-1", uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if connector.Status != domain.ConnectorStatusDisabled {
		t.Fatalf("status = %s, want disabled", connector.Status)
	}
}

func TestGetConnector_Success(t *testing.T) {
	svc, _, _, _ := setupConnectorService()

	created, err := svc.Create(context.Background(), domain.CreateConnectorInput{
		TenantID: "tenant-1",
		Type:     domain.ConnectorTypeHTTPClient,
		Name:     "Test Connector",
	}, "user-1", uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	fetched, err := svc.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByID() failed: %v", err)
	}
	if fetched.ID != created.ID {
		t.Fatalf("id = %s, want %s", fetched.ID, created.ID)
	}
}

func TestGetConnector_NotFound(t *testing.T) {
	svc, _, _, _ := setupConnectorService()

	_, err := svc.GetByID(context.Background(), "nonexistent-id")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateConnector_Success(t *testing.T) {
	svc, _, _, _ := setupConnectorService()

	created, err := svc.Create(context.Background(), domain.CreateConnectorInput{
		TenantID: "tenant-1",
		Type:     domain.ConnectorTypeHTTPClient,
		Name:     "Old Name",
	}, "user-1", uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	newName := "New Name"
	enabled := false
	updated, err := svc.Update(context.Background(), created.ID, domain.UpdateConnectorInput{
		Name:    &newName,
		Enabled: &enabled,
	}, "user-1", uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("Update() failed: %v", err)
	}
	if updated.Name != newName {
		t.Fatalf("name = %s, want %s", updated.Name, newName)
	}
	if updated.Enabled {
		t.Fatal("enabled should be false")
	}
}

func TestUpdateConnector_NotFound(t *testing.T) {
	svc, _, _, _ := setupConnectorService()

	newName := "New Name"
	_, err := svc.Update(context.Background(), "nonexistent", domain.UpdateConnectorInput{
		Name: &newName,
	}, "user-1", uuid.New().String(), "127.0.0.1")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteConnector_Success(t *testing.T) {
	svc, _, _, _ := setupConnectorService()

	created, err := svc.Create(context.Background(), domain.CreateConnectorInput{
		TenantID: "tenant-1",
		Type:     domain.ConnectorTypeHTTPClient,
		Name:     "To Delete",
	}, "user-1", uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	err = svc.Delete(context.Background(), created.ID, "user-1", uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("Delete() failed: %v", err)
	}

	// Verify it's gone
	_, err = svc.GetByID(context.Background(), created.ID)
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeleteConnector_NotFound(t *testing.T) {
	svc, _, _, _ := setupConnectorService()

	err := svc.Delete(context.Background(), "nonexistent", "user-1", uuid.New().String(), "127.0.0.1")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListConnectors_ByTenant(t *testing.T) {
	svc, _, _, _ := setupConnectorService()

	// Create 3 connectors for tenant-1
	for i := 0; i < 3; i++ {
		_, err := svc.Create(context.Background(), domain.CreateConnectorInput{
			TenantID: "tenant-1",
			Type:     domain.ConnectorTypeHTTPClient,
			Name:     "Connector",
		}, "user-1", uuid.New().String(), "127.0.0.1")
		if err != nil {
			t.Fatalf("Create() failed: %v", err)
		}
	}

	// Create 1 for tenant-2
	_, err := svc.Create(context.Background(), domain.CreateConnectorInput{
		TenantID: "tenant-2",
		Type:     domain.ConnectorTypeSMPPClient,
		Name:     "SMPP Connector",
	}, "user-1", uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	// List tenant-1
	result, err := svc.ListByTenant(context.Background(), "tenant-1", domain.Page{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("ListByTenant() failed: %v", err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("tenant-1: expected 3 connectors, got %d", len(result.Items))
	}

	// List tenant-2
	result2, err := svc.ListByTenant(context.Background(), "tenant-2", domain.Page{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("ListByTenant() failed: %v", err)
	}
	if len(result2.Items) != 1 {
		t.Fatalf("tenant-2: expected 1 connector, got %d", len(result2.Items))
	}
}

func TestTestConnector_Success(t *testing.T) {
	svc, _, _, _ := setupConnectorService()

	created, err := svc.Create(context.Background(), domain.CreateConnectorInput{
		TenantID: "tenant-1",
		Type:     domain.ConnectorTypeHTTPClient,
		Name:     "Testable Connector",
	}, "user-1", uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	err = svc.TestConnector(context.Background(), created.ID, uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("TestConnector() failed: %v", err)
	}
}

func TestTestConnector_NotFound(t *testing.T) {
	svc, _, _, _ := setupConnectorService()

	err := svc.TestConnector(context.Background(), "nonexistent", uuid.New().String(), "127.0.0.1")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
