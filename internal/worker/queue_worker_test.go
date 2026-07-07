package worker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/connector"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// ============================================================
// Mock QueueRepository
// ============================================================

type mockQueueRepo struct {
	mu        sync.Mutex
	messages  map[string]*domain.Message
	claimed   map[string]int
	callCount int32
}

func newMockQueueRepo(messages []domain.Message) *mockQueueRepo {
	m := make(map[string]*domain.Message)
	for i := range messages {
		msg := messages[i]
		msg.Status = domain.MessageStatusQueued
		msg.Version = 1
		msg.RetryCount = 0
		msg.MaxRetries = 3
		m[msg.ID] = &msg
	}
	return &mockQueueRepo{
		messages: m,
		claimed:  make(map[string]int),
	}
}

func (r *mockQueueRepo) ClaimQueued(ctx context.Context, limit int) ([]domain.Message, error) {
	atomic.AddInt32(&r.callCount, 1)
	r.mu.Lock()
	defer r.mu.Unlock()

	var result []domain.Message
	for id, msg := range r.messages {
		if msg.Status == domain.MessageStatusQueued && len(result) < limit {
			m := *msg
			m.Status = domain.MessageStatusSending
			r.messages[id] = &m
			r.claimed[id]++
			result = append(result, m)
		}
	}
	time.Sleep(10 * time.Millisecond)
	return result, nil
}

func (r *mockQueueRepo) AckSent(ctx context.Context, id string, version int, externalID string, parts int, price, cost int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	msg, ok := r.messages[id]
	if !ok {
		return fmt.Errorf("message %s not found", id)
	}
	msg.Status = domain.MessageStatusSent
	return nil
}

func (r *mockQueueRepo) AckFailed(ctx context.Context, id string, version int, errorCode, errorMessage string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	msg, ok := r.messages[id]
	if !ok {
		return fmt.Errorf("message %s not found", id)
	}
	msg.Status = domain.MessageStatusFailed
	return nil
}

func (r *mockQueueRepo) ScheduleRetry(ctx context.Context, id string, version int, errorCode, errorMessage string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	msg, ok := r.messages[id]
	if !ok {
		return fmt.Errorf("message %s not found", id)
	}
	msg.RetryCount++
	return nil
}

func (r *mockQueueRepo) GetRetryable(ctx context.Context, now time.Time, minDelay time.Duration, limit int) ([]domain.Message, error) {
	return nil, nil
}

// ============================================================
// Mock Connector (implements connector.Connector)
// ============================================================

type mockConnector struct {
	id          string
	protocol    domain.ConnectorType
	sendCount   map[string]int
	sendCountMu sync.Mutex
	shouldFail  bool
}

func newMockConnector(id string, shouldFail bool) *mockConnector {
	return &mockConnector{
		id:         id,
		protocol:   domain.ConnectorTypeMock,
		sendCount:  make(map[string]int),
		shouldFail: shouldFail,
	}
}

func (m *mockConnector) ID() string                    { return m.id }
func (m *mockConnector) Protocol() domain.ConnectorType { return m.protocol }

func (m *mockConnector) Send(_ context.Context, req *domain.SendRequest) (*domain.SendResult, error) {
	m.sendCountMu.Lock()
	m.sendCount[req.Message.ID]++
	m.sendCountMu.Unlock()

	time.Sleep(5 * time.Millisecond)

	if m.shouldFail {
		return nil, fmt.Errorf("mock send failure")
	}

	return &domain.SendResult{
		ExternalID:     "ext-" + req.Message.ID,
		Parts:          1,
		Price:          5000,
		Cost:           2000,
		ProviderStatus: "DELIVRD",
		Acceptance:     domain.AcceptanceFinal,
	}, nil
}

// ============================================================
// Mock Connector Registry
// ============================================================

type mockRegistry struct {
	mu    sync.RWMutex
	items map[string]connector.Connector
}

func newMockRegistry(connectors ...connector.Connector) *mockRegistry {
	r := &mockRegistry{items: make(map[string]connector.Connector)}
	for _, c := range connectors {
		r.items[c.ID()] = c
	}
	return r
}

func (r *mockRegistry) Get(id string) (connector.Connector, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.items[id]
	if !ok {
		return nil, fmt.Errorf("connector %q not found", id)
	}
	return c, nil
}

func (r *mockRegistry) List() []connector.Connector {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]connector.Connector, 0, len(r.items))
	for _, c := range r.items {
		result = append(result, c)
	}
	return result
}

// ============================================================
// Mock Message Repository
// ============================================================

type mockMessageRepo struct {
	store map[string]*domain.Message
	mu    sync.Mutex
}

func newMockMessageRepo() *mockMessageRepo {
	return &mockMessageRepo{store: make(map[string]*domain.Message)}
}

func (r *mockMessageRepo) Create(ctx context.Context, input domain.CreateMessageInput, createdBy string) (*domain.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *mockMessageRepo) GetByID(ctx context.Context, id string) (*domain.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *mockMessageRepo) Update(ctx context.Context, id string, input domain.UpdateMessageInput, version int) (*domain.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *mockMessageRepo) Delete(ctx context.Context, id string) error {
	return fmt.Errorf("not implemented")
}

func (r *mockMessageRepo) ListByTenant(ctx context.Context, tenantID string, filter domain.MessageFilter, page domain.Page) (domain.PageResult[domain.Message], error) {
	return domain.PageResult[domain.Message]{}, fmt.Errorf("not implemented")
}

func (r *mockMessageRepo) CountByTenant(ctx context.Context, tenantID string) (int64, error) {
	return 0, fmt.Errorf("not implemented")
}

func (r *mockMessageRepo) UpdateStatus(ctx context.Context, id string, _ domain.UpdateMessageInput, version int) (*domain.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *mockMessageRepo) UpdateDLR(ctx context.Context, id string, dlrStatus domain.DLRStatus, status domain.MessageStatus) error {
	return fmt.Errorf("not implemented")
}

func (r *mockMessageRepo) AppendDLR(ctx context.Context, dlr *domain.DLRRecord) error {
	return fmt.Errorf("not implemented")
}

func (r *mockMessageRepo) GetByExternalID(ctx context.Context, externalID string) (*domain.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *mockMessageRepo) Count(ctx context.Context, filter domain.MessageFilter) (int64, error) {
	return 0, fmt.Errorf("not implemented")
}

func (r *mockMessageRepo) GetByClientRef(ctx context.Context, tenantID string, clientRef string) (*domain.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *mockMessageRepo) List(ctx context.Context, filter domain.MessageFilter) (domain.PageResult[domain.Message], error) {
	return domain.PageResult[domain.Message]{}, nil
}

// ============================================================
// Mock RetryPolicy
// ============================================================

type mockRetryPolicy struct{}

func (m *mockRetryPolicy) NextDelay(_ domain.RetryContext) time.Duration {
	return 10 * time.Millisecond
}

// ============================================================
// Helper
// ============================================================

func strPtr(s string) *string {
	return &s
}

// ============================================================
// Tests
// ============================================================

func TestQueueWorker_SendsAllMessages(t *testing.T) {
	messages := []domain.Message{
		{BaseModel: domain.BaseModel{ID: "msg-1"}, TenantID: "tenant-1", ConnectorID: strPtr("conn-1"), Destination: "+1234"},
		{BaseModel: domain.BaseModel{ID: "msg-2"}, TenantID: "tenant-1", ConnectorID: strPtr("conn-2"), Destination: "+5678"},
		{BaseModel: domain.BaseModel{ID: "msg-3"}, TenantID: "tenant-2", ConnectorID: strPtr("conn-1"), Destination: "+9012"},
	}

	conn1 := newMockConnector("conn-1", false)
	conn2 := newMockConnector("conn-2", false)
	registry := newMockRegistry(conn1, conn2)

	qw := NewQueueWorker(
		context.Background(),
		newMockQueueRepo(messages),
		newMockMessageRepo(),
		registry,
		&mockRetryPolicy{},
		nil, // no metrics
		nil, // no event bus
		WithBatchSize(10),
		WithPollInterval(10*time.Millisecond),
	)

	qw.Start()
	time.Sleep(300 * time.Millisecond)
	qw.Stop()

	conn1.sendCountMu.Lock()
	if conn1.sendCount["msg-1"] != 1 {
		t.Errorf("conn1 expected 1 send for msg-1, got %d", conn1.sendCount["msg-1"])
	}
	if conn1.sendCount["msg-3"] != 1 {
		t.Errorf("conn1 expected 1 send for msg-3, got %d", conn1.sendCount["msg-3"])
	}
	conn1.sendCountMu.Unlock()
	conn2.sendCountMu.Lock()
	if conn2.sendCount["msg-2"] != 1 {
		t.Errorf("conn2 expected 1 send for msg-2, got %d", conn2.sendCount["msg-2"])
	}
	conn2.sendCountMu.Unlock()
}

func TestQueueWorker_NoDoubleSend_OnRetry(t *testing.T) {
	messages := []domain.Message{
		{BaseModel: domain.BaseModel{ID: "msg-1"}, TenantID: "tenant-1", ConnectorID: strPtr("conn-1"), Destination: "+1234"},
	}

	conn := newMockConnector("conn-1", false)
	registry := newMockRegistry(conn)

	qw := NewQueueWorker(
		context.Background(),
		newMockQueueRepo(messages),
		newMockMessageRepo(),
		registry,
		&mockRetryPolicy{},
		nil,
		nil,
		WithBatchSize(10),
		WithPollInterval(10*time.Millisecond),
	)

	qw.Start()
	time.Sleep(300 * time.Millisecond)
	qw.Stop()

	conn.sendCountMu.Lock()
	sent := conn.sendCount["msg-1"]
	conn.sendCountMu.Unlock()

	if sent != 1 {
		t.Errorf("expected exactly 1 send, got %d", sent)
	}
}

func TestQueueWorker_HandlesFailure(t *testing.T) {
	messages := []domain.Message{
		{BaseModel: domain.BaseModel{ID: "fail-msg"}, TenantID: "tenant-1", ConnectorID: strPtr("conn-1"), Destination: "+1234", MaxRetries: 0},
	}

	conn := newMockConnector("conn-1", true)
	registry := newMockRegistry(conn)

	qw := NewQueueWorker(
		context.Background(),
		newMockQueueRepo(messages),
		newMockMessageRepo(),
		registry,
		&mockRetryPolicy{},
		nil,
		nil,
		WithBatchSize(10),
		WithPollInterval(10*time.Millisecond),
	)

	qw.Start()
	time.Sleep(300 * time.Millisecond)
	qw.Stop()

	conn.sendCountMu.Lock()
	sent := conn.sendCount["fail-msg"]
	conn.sendCountMu.Unlock()

	if sent == 0 {
		t.Errorf("expected at least 1 send attempt for fail-msg")
	}
}

func TestConcurrentWorkers_NoDoubleSend(t *testing.T) {
	messages := []domain.Message{
		{BaseModel: domain.BaseModel{ID: "msg-1"}, TenantID: "tenant-1", ConnectorID: strPtr("conn-1"), Destination: "+1234"},
		{BaseModel: domain.BaseModel{ID: "msg-2"}, TenantID: "tenant-1", ConnectorID: strPtr("conn-1"), Destination: "+5678"},
		{BaseModel: domain.BaseModel{ID: "msg-3"}, TenantID: "tenant-2", ConnectorID: strPtr("conn-1"), Destination: "+9012"},
		{BaseModel: domain.BaseModel{ID: "msg-4"}, TenantID: "tenant-2", ConnectorID: strPtr("conn-2"), Destination: "+3456"},
		{BaseModel: domain.BaseModel{ID: "msg-5"}, TenantID: "tenant-1", ConnectorID: strPtr("conn-2"), Destination: "+7890"},
	}

	conn1 := newMockConnector("conn-1", false)
	conn2 := newMockConnector("conn-2", false)
	registry := newMockRegistry(conn1, conn2)
	queueRepo := newMockQueueRepo(messages)

	qw1 := NewQueueWorker(
		context.Background(),
		queueRepo,
		newMockMessageRepo(),
		registry,
		&mockRetryPolicy{},
		nil,
		nil,
		WithBatchSize(10),
		WithPollInterval(5*time.Millisecond),
	)
	qw2 := NewQueueWorker(
		context.Background(),
		queueRepo,
		newMockMessageRepo(),
		registry,
		&mockRetryPolicy{},
		nil,
		nil,
		WithBatchSize(10),
		WithPollInterval(5*time.Millisecond),
	)

	qw1.Start()
	qw2.Start()

	time.Sleep(300 * time.Millisecond)
	qw1.Stop()
	qw2.Stop()

	conn1.sendCountMu.Lock()
	conn2.sendCountMu.Lock()
	totalSent := 0
	for _, id := range []string{"msg-1", "msg-2", "msg-3"} {
		totalSent += conn1.sendCount[id]
	}
	for _, id := range []string{"msg-4", "msg-5"} {
		totalSent += conn2.sendCount[id]
	}
	conn1.sendCountMu.Unlock()
	conn2.sendCountMu.Unlock()

	if totalSent != 5 {
		t.Errorf("expected exactly 5 total sends, got %d", totalSent)
	}
}
