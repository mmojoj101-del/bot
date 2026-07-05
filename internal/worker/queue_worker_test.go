package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// ============================================================
// Mock QueueRepository for concurrent testing
// ============================================================

type mockQueueRepo struct {
	mu        sync.Mutex
	messages  map[string]*domain.Message
	claimed   map[string]int // how many times each message was claimed
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
	// Simulate processing delay
	time.Sleep(10 * time.Millisecond)
	return result, nil
}

func (r *mockQueueRepo) AckSent(ctx context.Context, id string, version int, externalID string, parts int, price, cost int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if msg, ok := r.messages[id]; ok {
		m := *msg
		m.Status = domain.MessageStatusSent
		r.messages[id] = &m
	}
	return nil
}

func (r *mockQueueRepo) AckFailed(ctx context.Context, id string, version int, errorCode, errorMessage string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if msg, ok := r.messages[id]; ok {
		m := *msg
		m.Status = domain.MessageStatusFailed
		r.messages[id] = &m
	}
	return nil
}

func (r *mockQueueRepo) ScheduleRetry(ctx context.Context, id string, version int, errorCode, errorMessage string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if msg, ok := r.messages[id]; ok {
		m := *msg
		m.Status = domain.MessageStatusRetrying
		m.RetryCount++
		r.messages[id] = &m
	}
	return nil
}

func (r *mockQueueRepo) GetRetryable(ctx context.Context, now time.Time, minDelay time.Duration, limit int) ([]domain.Message, error) {
	return nil, nil
}

// ============================================================
// Mock Sender — tracks how many times each message was sent
// ============================================================

type mockSender struct {
	mu         sync.Mutex
	sendCount  map[string]int // messageID → send attempts
	shouldFail bool
}

func newMockSender(shouldFail bool) *mockSender {
	return &mockSender{
		sendCount:  make(map[string]int),
		shouldFail: shouldFail,
	}
}

func (s *mockSender) Type() domain.ConnectorType {
	return domain.ConnectorTypeMock
}

func (s *mockSender) Send(ctx context.Context, req domain.SendRequest) (*domain.SendResult, error) {
	s.mu.Lock()
	s.sendCount[req.Message.ID]++
	s.mu.Unlock()

	// Simulate send time
	time.Sleep(5 * time.Millisecond)

	if s.shouldFail {
		return nil, domain.ErrInvalidInput
	}

	return &domain.SendResult{
		ExternalID:     "ext-" + req.Message.ID,
		Parts:          1,
		Price:          5000,
		Cost:           2000,
		ProviderStatus: "DELIVRD",
	}, nil
}

// ============================================================
// Mock Connector Repository
// ============================================================

type mockConnRepo struct {
	connectors map[string]*domain.Connector
}

func newMockConnRepo() *mockConnRepo {
	return &mockConnRepo{
		connectors: map[string]*domain.Connector{
			"conn-1": {
				BaseModel: domain.BaseModel{ID: "conn-1"},
				Name:      "test-connector",
				Type:      domain.ConnectorTypeMock,
				Status:    domain.ConnectorStatusActive,
				Config:    []byte(`{}`),
				TenantID:  "tenant-1",
			},
		},
	}
}

func (r *mockConnRepo) GetByID(ctx context.Context, id string) (*domain.Connector, error) {
	if c, ok := r.connectors[id]; ok {
		return c, nil
	}
	return nil, domain.ErrNotFound
}

func (r *mockConnRepo) Create(ctx context.Context, input domain.CreateConnectorInput, createdBy string) (*domain.Connector, error) {
	return nil, nil
}
func (r *mockConnRepo) Update(ctx context.Context, id string, input domain.UpdateConnectorInput, updatedBy string, version int) (*domain.Connector, error) {
	return nil, nil
}
func (r *mockConnRepo) Delete(ctx context.Context, id string) error { return nil }
func (r *mockConnRepo) ListByTenant(ctx context.Context, filter domain.ConnectorFilter) (domain.PageResult[domain.Connector], error) {
	return domain.PageResult[domain.Connector]{}, nil
}
func (r *mockConnRepo) CountByTenant(ctx context.Context, tenantID string) (int64, error) { return 0, nil }

// ============================================================
// Mock Message Repository (minimal for worker dependencies)
// ============================================================

type mockMsgRepo struct{}

func newMockMsgRepo() *mockMsgRepo { return &mockMsgRepo{} }

func (r *mockMsgRepo) Create(ctx context.Context, input domain.CreateMessageInput, createdBy string) (*domain.Message, error) { return nil, nil }
func (r *mockMsgRepo) GetByID(ctx context.Context, id string) (*domain.Message, error) { return nil, nil }
func (r *mockMsgRepo) GetByClientRef(ctx context.Context, tenantID, clientRef string) (*domain.Message, error) { return nil, nil }
func (r *mockMsgRepo) GetByExternalID(ctx context.Context, externalID string) (*domain.Message, error) { return nil, nil }
func (r *mockMsgRepo) UpdateStatus(ctx context.Context, id string, input domain.UpdateMessageInput, version int) (*domain.Message, error) { return nil, nil }
func (r *mockMsgRepo) AppendDLR(ctx context.Context, dlr *domain.DLRRecord) error { return nil }
func (r *mockMsgRepo) List(ctx context.Context, filter domain.MessageFilter) (domain.PageResult[domain.Message], error) { return domain.PageResult[domain.Message]{}, nil }
func (r *mockMsgRepo) Count(ctx context.Context, filter domain.MessageFilter) (int64, error) { return 0, nil }
func (r *mockMsgRepo) Delete(ctx context.Context, id string) error { return nil }

// ============================================================
// Test: Concurrent Workers — No Double Send
// ============================================================

func TestConcurrentWorkers_NoDoubleSend(t *testing.T) {
	// Arrange: Create 20 queued messages
	var messages []domain.Message
	for i := 0; i < 20; i++ {
		id := string(rune('a' + i))
		connID := "conn-1"
		messages = append(messages, domain.Message{
			BaseModel:    domain.BaseModel{ID: id, Version: 1},
			TenantID:     "tenant-1",
			ConnectorID: &connID,
			Status:       domain.MessageStatusQueued,
			Source:       "Sender",
			Destination: "1234567890",
			Text:        "Test message " + id,
			MaxRetries:  3,
		})
	}

	queueRepo := newMockQueueRepo(messages)
	sender := newMockSender(false)
	connRepo := newMockConnRepo()
	msgRepo := newMockMsgRepo()

	senders := map[domain.ConnectorType]domain.Sender{
		domain.ConnectorTypeMock: sender,
	}

	retryPolicy := NewDefaultRetryPolicy()

	// Act: Run 3 workers concurrently
	var wg sync.WaitGroup
	numWorkers := 3
	batchSize := 10

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker := NewQueueWorker(
				queueRepo, msgRepo, connRepo, senders, retryPolicy, nil, nil,
				WithBatchSize(batchSize),
			)
			// Each worker processes one batch
			worker.processBatch()
		}()
	}
	wg.Wait()

	// Assert
	// 1. No message was claimed more than once
	for id, count := range queueRepo.claimed {
		if count > 1 {
			t.Errorf("message %s claimed %d times (expected 1)", id, count)
		}
	}

	// 2. Each claimed message was sent exactly once
	totalSent := 0
	for _, count := range sender.sendCount {
		if count > 1 {
			t.Errorf("message sent %d times (expected 1)", count)
		}
		totalSent += count
	}

	// 3. Total sent is between batchSize and total messages
	expectedMax := batchSize * numWorkers // max possible claims
	if totalSent > expectedMax {
		t.Errorf("sent %d messages, expected max %d", totalSent, expectedMax)
	}

	t.Logf("Total messages: %d", len(messages))
	t.Logf("Messages claimed: %d", len(queueRepo.claimed))
	t.Logf("Messages sent: %d", totalSent)
	t.Logf("Repo call count: %d", atomic.LoadInt32(&queueRepo.callCount))
}

func TestConcurrentWorker_RetryFlow(t *testing.T) {
	// Arrange: 1 message that fails, worker schedules retry
	connID := "conn-1"
	msg := domain.Message{
		BaseModel:    domain.BaseModel{ID: "msg-1", Version: 1},
		TenantID:     "tenant-1",
		ConnectorID: &connID,
		Status:       domain.MessageStatusQueued,
		Source:       "Sender",
		Destination: "1234567890",
		Text:        "Test retry",
		MaxRetries:  3,
		RetryCount:  0,
	}

	queueRepo := newMockQueueRepo([]domain.Message{msg})
	failingSender := newMockSender(true) // always fails
	connRepo := newMockConnRepo()
	msgRepo := newMockMsgRepo()

	senders := map[domain.ConnectorType]domain.Sender{
		domain.ConnectorTypeMock: failingSender,
	}

	retryPolicy := NewDefaultRetryPolicy()

	worker := NewQueueWorker(
		queueRepo, msgRepo, connRepo, senders, retryPolicy, nil, nil,
		WithBatchSize(10),
	)

	// Act: Process the message (it should fail and be scheduled for retry)
	worker.processBatch()

	// Assert: Message should be in retrying status (since retry_count < max_retries)
	queueRepo.mu.Lock()
	updatedMsg := queueRepo.messages["msg-1"]
	queueRepo.mu.Unlock()

	if updatedMsg == nil {
		t.Fatal("message not found after processing")
	}

	if updatedMsg.Status != domain.MessageStatusRetrying {
		t.Errorf("expected status retrying, got %s", updatedMsg.Status)
	}
	if updatedMsg.RetryCount != 1 {
		t.Errorf("expected retry count 1, got %d", updatedMsg.RetryCount)
	}

	t.Logf("Message status: %s", updatedMsg.Status)
	t.Logf("Message retry count: %d", updatedMsg.RetryCount)
}
