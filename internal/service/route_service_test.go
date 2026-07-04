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

type mockRouteRepo struct {
	mu     sync.Mutex
	routes map[string]*domain.Route
	err    error
}

func newMockRouteRepo() *mockRouteRepo {
	return &mockRouteRepo{
		routes: make(map[string]*domain.Route),
	}
}

func (r *mockRouteRepo) Create(ctx context.Context, input domain.CreateRouteInput, createdBy string) (*domain.Route, error) {
	strategy := input.Strategy
	if strategy == "" {
		strategy = domain.RouteStrategyStatic
	}
	weight := input.Weight
	if weight < 1 {
		weight = 1
	}
	rt := &domain.Route{
		BaseModel: domain.BaseModel{
			ID:        uuid.New().String(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Version:   1,
		},
		TenantID:    input.TenantID,
		Name:        input.Name,
		Type:        input.Type,
		Strategy:    strategy,
		Weight:      weight,
		Priority:    input.Priority,
		Prefix:      input.Prefix,
		ConnectorID: input.ConnectorID,
		Enabled:     true,
		CreatedBy:   createdBy,
		UpdatedBy:   createdBy,
	}
	r.mu.Lock()
	r.routes[rt.ID] = rt
	r.mu.Unlock()
	return rt, nil
}

func (r *mockRouteRepo) GetByID(ctx context.Context, id string) (*domain.Route, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rt, ok := r.routes[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return rt, nil
}

func (r *mockRouteRepo) Update(ctx context.Context, id string, input domain.UpdateRouteInput, updatedBy string, version int) (*domain.Route, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rt, ok := r.routes[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	if input.Name != nil {
		rt.Name = *input.Name
	}
	if input.Type != nil {
		rt.Type = *input.Type
	}
	if input.Strategy != nil {
		rt.Strategy = *input.Strategy
	}
	if input.Weight != nil {
		rt.Weight = *input.Weight
	}
	if input.Priority != nil {
		rt.Priority = *input.Priority
	}
	if input.Prefix != nil {
		rt.Prefix = *input.Prefix
	}
	if input.ConnectorID != nil {
		rt.ConnectorID = *input.ConnectorID
	}
	if input.Enabled != nil {
		rt.Enabled = *input.Enabled
	}
	rt.UpdatedBy = updatedBy
	rt.Version++
	return rt, nil
}

func (r *mockRouteRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.routes[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.routes, id)
	return nil
}

func (r *mockRouteRepo) ListByTenant(ctx context.Context, filter domain.RouteFilter) (domain.PageResult[domain.Route], error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var items []domain.Route
	for _, rt := range r.routes {
		if rt.TenantID != filter.TenantID {
			continue
		}
		if filter.Type != nil && rt.Type != *filter.Type {
			continue
		}
		if filter.Strategy != nil && rt.Strategy != *filter.Strategy {
			continue
		}
		if filter.Prefix != "" && len(rt.Prefix) >= len(filter.Prefix) && rt.Prefix[:len(filter.Prefix)] != filter.Prefix {
			continue
		}
		if filter.ConnectorID != "" && rt.ConnectorID != filter.ConnectorID {
			continue
		}
		items = append(items, *rt)
	}
	return domain.PageResult[domain.Route]{Items: items, Total: int64(len(items)), Page: filter.Page}, nil
}

func (r *mockRouteRepo) ListByTenantAndType(ctx context.Context, tenantID string, routeType domain.RouteType) ([]domain.Route, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var items []domain.Route
	for _, rt := range r.routes {
		if rt.TenantID == tenantID && rt.Type == routeType {
			items = append(items, *rt)
		}
	}
	return items, nil
}

func (r *mockRouteRepo) CountByTenant(ctx context.Context, tenantID string) (int64, error) {
	return 0, nil
}

func setupRouteService() (*RouteService, *mockRouteRepo, *mockConnectorRepo, *mockAuditRepo, *event.MemoryBus) {
	routeRepo := newMockRouteRepo()
	connectorRepo := newMockConnectorRepo()
	auditRepo := &mockAuditRepo{}
	bus := event.NewMemoryBus()
	clock := &mockClock{now: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)}
	txManager := &mockTxManager{}

	svc := NewRouteService(routeRepo, connectorRepo, auditRepo, txManager, bus, clock)
	return svc, routeRepo, connectorRepo, auditRepo, bus
}

func TestCreateRoute_Success(t *testing.T) {
	svc, _, connRepo, _, _ := setupRouteService()

	// Pre-create a connector
	conn, _ := connRepo.Create(context.Background(), domain.CreateConnectorInput{
		TenantID: "tenant-1",
		Type:     domain.ConnectorTypeHTTPClient,
		Name:     "HTTP Connector",
	}, "user-1")

	route, err := svc.Create(context.Background(), domain.CreateRouteInput{
		TenantID:    "tenant-1",
		Name:        "My Route",
		Type:        domain.RouteTypeSMS,
		Strategy:    domain.RouteStrategyStatic,
		Weight:      1,
		Priority:    10,
		Prefix:      "2010",
		ConnectorID: conn.ID,
	}, "user-1", uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if route.Name != "My Route" {
		t.Fatalf("name = %s, want My Route", route.Name)
	}
	if route.Prefix != "2010" {
		t.Fatalf("prefix = %s, want 2010", route.Prefix)
	}
	if route.Type != domain.RouteTypeSMS {
		t.Fatalf("type = %s, want sms", route.Type)
	}
	if route.Strategy != domain.RouteStrategyStatic {
		t.Fatalf("strategy = %s, want static", route.Strategy)
	}
	if !route.Enabled {
		t.Fatal("route should be enabled by default")
	}
}

func TestCreateRoute_AllStrategies(t *testing.T) {
	svc, _, connRepo, _, _ := setupRouteService()
	conn, _ := connRepo.Create(context.Background(), domain.CreateConnectorInput{
		TenantID: "tenant-1", Type: domain.ConnectorTypeHTTPClient, Name: "HTTP",
	}, "user-1")

	strategies := []domain.RouteStrategy{
		domain.RouteStrategyStatic,
		domain.RouteStrategyRoundRobin,
		domain.RouteStrategyFailover,
		domain.RouteStrategyWeighted,
	}

	for _, s := range strategies {
		route, err := svc.Create(context.Background(), domain.CreateRouteInput{
			TenantID: "tenant-1", Name: "Route-" + string(s),
			Type: domain.RouteTypeSMS, Strategy: s,
			Prefix: "20", ConnectorID: conn.ID,
		}, "user-1", uuid.New().String(), "127.0.0.1")
		if err != nil {
			t.Fatalf("Create(%s) failed: %v", s, err)
		}
		if route.Strategy != s {
			t.Fatalf("strategy = %s, want %s", route.Strategy, s)
		}
	}
}

func TestCreateRoute_InvalidConnector(t *testing.T) {
	svc, _, _, _, _ := setupRouteService()

	_, err := svc.Create(context.Background(), domain.CreateRouteInput{
		TenantID:    "tenant-1",
		Name:        "Bad Route",
		Type:        domain.RouteTypeSMS,
		Prefix:      "20",
		ConnectorID: "nonexistent",
	}, "user-1", uuid.New().String(), "127.0.0.1")
	if err == nil {
		t.Fatal("expected error for nonexistent connector")
	}
}

func TestGetRoute_Success(t *testing.T) {
	svc, _, connRepo, _, _ := setupRouteService()
	conn, _ := connRepo.Create(context.Background(), domain.CreateConnectorInput{
		TenantID: "tenant-1", Type: domain.ConnectorTypeHTTPClient, Name: "HTTP",
	}, "user-1")

	created, _ := svc.Create(context.Background(), domain.CreateRouteInput{
		TenantID: "tenant-1", Name: "Test Route",
		Type: domain.RouteTypeSMS, Prefix: "2010", ConnectorID: conn.ID,
	}, "user-1", uuid.New().String(), "127.0.0.1")

	fetched, err := svc.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByID() failed: %v", err)
	}
	if fetched.ID != created.ID {
		t.Fatalf("id = %s, want %s", fetched.ID, created.ID)
	}
}

func TestUpdateRoute_Success(t *testing.T) {
	svc, _, connRepo, _, _ := setupRouteService()
	conn, _ := connRepo.Create(context.Background(), domain.CreateConnectorInput{
		TenantID: "tenant-1", Type: domain.ConnectorTypeHTTPClient, Name: "HTTP",
	}, "user-1")

	created, _ := svc.Create(context.Background(), domain.CreateRouteInput{
		TenantID: "tenant-1", Name: "Old Name",
		Type: domain.RouteTypeSMS, Prefix: "20", ConnectorID: conn.ID,
	}, "user-1", uuid.New().String(), "127.0.0.1")

	newName := "New Name"
	newPriority := 99
	updated, err := svc.Update(context.Background(), created.ID, domain.UpdateRouteInput{
		Name:     &newName,
		Priority: &newPriority,
	}, "user-1", uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("Update() failed: %v", err)
	}
	if updated.Name != newName {
		t.Fatalf("name = %s, want %s", updated.Name, newName)
	}
	if updated.Priority != newPriority {
		t.Fatalf("priority = %d, want %d", updated.Priority, newPriority)
	}
}

func TestDeleteRoute_Success(t *testing.T) {
	svc, _, connRepo, _, _ := setupRouteService()
	conn, _ := connRepo.Create(context.Background(), domain.CreateConnectorInput{
		TenantID: "tenant-1", Type: domain.ConnectorTypeHTTPClient, Name: "HTTP",
	}, "user-1")

	created, _ := svc.Create(context.Background(), domain.CreateRouteInput{
		TenantID: "tenant-1", Name: "To Delete",
		Type: domain.RouteTypeSMS, Prefix: "20", ConnectorID: conn.ID,
	}, "user-1", uuid.New().String(), "127.0.0.1")

	err := svc.Delete(context.Background(), created.ID, "user-1", uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("Delete() failed: %v", err)
	}
}

func TestListRoutes_ByTenant(t *testing.T) {
	svc, _, connRepo, _, _ := setupRouteService()
	conn, _ := connRepo.Create(context.Background(), domain.CreateConnectorInput{
		TenantID: "tenant-1", Type: domain.ConnectorTypeHTTPClient, Name: "HTTP",
	}, "user-1")

	for i := 0; i < 3; i++ {
		svc.Create(context.Background(), domain.CreateRouteInput{
			TenantID: "tenant-1", Name: "Route",
			Type: domain.RouteTypeSMS, Prefix: "20", ConnectorID: conn.ID,
		}, "user-1", uuid.New().String(), "127.0.0.1")
	}

	result, err := svc.ListByTenant(context.Background(), domain.RouteFilter{
		TenantID: "tenant-1",
		Page:     domain.Page{Limit: 10, Offset: 0},
	})
	if err != nil {
		t.Fatalf("ListByTenant() failed: %v", err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("expected 3 routes, got %d", len(result.Items))
	}
}

func TestListRoutes_FilterByStrategy(t *testing.T) {
	svc, _, connRepo, _, _ := setupRouteService()
	conn, _ := connRepo.Create(context.Background(), domain.CreateConnectorInput{
		TenantID: "tenant-1", Type: domain.ConnectorTypeHTTPClient, Name: "HTTP",
	}, "user-1")

	svc.Create(context.Background(), domain.CreateRouteInput{
		TenantID: "tenant-1", Name: "Static",
		Type: domain.RouteTypeSMS, Strategy: domain.RouteStrategyStatic,
		Prefix: "20", ConnectorID: conn.ID,
	}, "user-1", uuid.New().String(), "127.0.0.1")

	svc.Create(context.Background(), domain.CreateRouteInput{
		TenantID: "tenant-1", Name: "RoundRobin",
		Type: domain.RouteTypeSMS, Strategy: domain.RouteStrategyRoundRobin,
		Prefix: "20", ConnectorID: conn.ID,
	}, "user-1", uuid.New().String(), "127.0.0.1")

	rr := domain.RouteStrategyRoundRobin
	result, err := svc.ListByTenant(context.Background(), domain.RouteFilter{
		TenantID: "tenant-1",
		Strategy: &rr,
		Page:     domain.Page{Limit: 10, Offset: 0},
	})
	if err != nil {
		t.Fatalf("ListByTenant() failed: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 round_robin route, got %d", len(result.Items))
	}
}

func TestListByTenantAndType(t *testing.T) {
	svc, _, connRepo, _, _ := setupRouteService()
	conn, _ := connRepo.Create(context.Background(), domain.CreateConnectorInput{
		TenantID: "tenant-1", Type: domain.ConnectorTypeHTTPClient, Name: "HTTP",
	}, "user-1")

	svc.Create(context.Background(), domain.CreateRouteInput{
		TenantID: "tenant-1", Name: "SMS Route",
		Type: domain.RouteTypeSMS, Prefix: "20", ConnectorID: conn.ID,
	}, "user-1", uuid.New().String(), "127.0.0.1")

	svc.Create(context.Background(), domain.CreateRouteInput{
		TenantID: "tenant-1", Name: "Call Route",
		Type: domain.RouteTypeCall, Prefix: "20", ConnectorID: conn.ID,
	}, "user-1", uuid.New().String(), "127.0.0.1")

	smsRoutes, err := svc.ListByTenantAndType(context.Background(), "tenant-1", domain.RouteTypeSMS)
	if err != nil {
		t.Fatalf("ListByTenantAndType() failed: %v", err)
	}
	if len(smsRoutes) != 1 {
		t.Fatalf("expected 1 sms route, got %d", len(smsRoutes))
	}
}
