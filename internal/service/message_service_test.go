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

type mockMessageRepo struct {
	mu       sync.Mutex
	messages map[string]*domain.Message
}

func newMockMessageRepo() *mockMessageRepo {
	return &mockMessageRepo{
		messages: make(map[string]*domain.Message),
	}
}

func (r *mockMessageRepo) Create(ctx context.Context, input domain.CreateMessageInput, createdBy string) (*domain.Message, error) {
	m := &domain.Message{
		BaseModel: domain.BaseModel{
			ID:        uuid.New().String(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Version:   1,
		},
		TenantID:    input.TenantID,
		ClientID:    input.ClientID,
		Direction:   input.Direction,
		Status:      domain.MessageStatusAccepted,
		Source:      input.Source,
		Destination: input.Destination,
		Text:        input.Text,
		Encoding:    input.Encoding,
		Priority:    input.Priority,
		Parts:       1,
		RetryCount:  0,
		MaxRetries:  3,
		ClientRef:   input.ClientRef,
	}
	if m.Encoding == "" {
		m.Encoding = domain.EncodingGSM7
	}
	if m.Priority == "" {
		m.Priority = domain.MessagePriorityNormal
	}
	r.mu.Lock()
	r.messages[m.ID] = m
	r.mu.Unlock()
	return m, nil
}

func (r *mockMessageRepo) GetByID(ctx context.Context, id string) (*domain.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.messages[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return m, nil
}

func (r *mockMessageRepo) GetByClientRef(ctx context.Context, tenantID, clientRef string) (*domain.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range r.messages {
		if m.TenantID == tenantID && m.ClientRef == clientRef {
			return m, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *mockMessageRepo) GetByExternalID(ctx context.Context, externalID string) (*domain.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range r.messages {
		if m.ExternalID == externalID && m.ExternalID != "" {
			return m, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *mockMessageRepo) UpdateStatus(ctx context.Context, id string, input domain.UpdateMessageInput, version int) (*domain.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.messages[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	if m.Version != version {
		return nil, domain.ErrConflict
	}
	if input.Status != nil {
		m.Status = *input.Status
	}
	if input.ConnectorID != nil {
		m.ConnectorID = input.ConnectorID
	}
	if input.RouteID != nil {
		m.RouteID = input.RouteID
	}
	if input.ExternalID != nil {
		m.ExternalID = *input.ExternalID
	}
	if input.DLRStatus != nil {
		m.DLRStatus = input.DLRStatus
	}
	if input.DLRID != nil {
		m.DLRID = *input.DLRID
	}
	if input.ErrorCode != nil {
		m.ErrorCode = *input.ErrorCode
	}
	if input.ErrorMessage != nil {
		m.ErrorMessage = *input.ErrorMessage
	}
	if input.Parts != nil {
		m.Parts = *input.Parts
	}
	if input.Price != nil {
		m.Price = *input.Price
	}
	if input.Cost != nil {
		m.Cost = *input.Cost
	}
	if input.SentAt != nil {
		m.SentAt = input.SentAt
	}
	if input.DeliveredAt != nil {
		m.DeliveredAt = input.DeliveredAt
	}
	if input.FailedAt != nil {
		m.FailedAt = input.FailedAt
	}
	if m.Status == domain.MessageStatusRetrying {
		m.RetryCount++
	}
	m.Version++
	return m, nil
}

func (r *mockMessageRepo) AppendDLR(ctx context.Context, dlr *domain.DLRRecord) error {
	return nil
}

func (r *mockMessageRepo) List(ctx context.Context, filter domain.MessageFilter) (domain.PageResult[domain.Message], error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var items []domain.Message
	for _, m := range r.messages {
		if m.TenantID != filter.TenantID {
			continue
		}
		items = append(items, *m)
	}
	return domain.PageResult[domain.Message]{Items: items, Total: int64(len(items)), Page: filter.Page}, nil
}

func (r *mockMessageRepo) Count(ctx context.Context, filter domain.MessageFilter) (int64, error) {
	return 0, nil
}

func (r *mockMessageRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.messages, id)
	return nil
}

func setupMessageService() (*MessageService, *mockMessageRepo) {
	repo := newMockMessageRepo()
	bus := event.NewMemoryBus()
	clock := &mockClock{now: time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)}
	return NewMessageService(repo, &mockAuditRepo{}, bus, clock), repo
}

func TestCreateMessage_Success(t *testing.T) {
	svc, _ := setupMessageService()

	msg, err := svc.Create(context.Background(), domain.CreateMessageInput{
		TenantID:    "tenant-1",
		ClientID:    "api-key-1",
		Source:      "FurySMS",
		Destination: "201234567890",
		Text:        "Hello World",
		DLRURL:      "https://example.com/dlr",
	}, "user-1", uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if msg.Status != domain.MessageStatusAccepted {
		t.Fatalf("status = %s, want accepted", msg.Status)
	}
	if msg.Source != "FurySMS" {
		t.Fatalf("source = %s, want FurySMS", msg.Source)
	}
	if msg.Text != "Hello World" {
		t.Fatalf("text = %s, want Hello World", msg.Text)
	}
}

func TestCreateMessage_Idempotency(t *testing.T) {
	svc, _ := setupMessageService()

	clientRef := "ref-123"
	first, err := svc.Create(context.Background(), domain.CreateMessageInput{
		TenantID:    "tenant-1",
		ClientID:    "api-key-1",
		Source:      "FurySMS",
		Destination: "201234567890",
		Text:        "Hello",
		ClientRef:   clientRef,
	}, "user-1", uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("First Create() failed: %v", err)
	}

	// Same client_ref should return the existing message
	second, err := svc.Create(context.Background(), domain.CreateMessageInput{
		TenantID:    "tenant-1",
		ClientID:    "api-key-1",
		Source:      "FurySMS",
		Destination: "201234567890",
		Text:        "Hello",
		ClientRef:   clientRef,
	}, "user-1", uuid.New().String(), "127.0.0.1")
	if err != nil {
		t.Fatalf("Second Create() failed: %v", err)
	}
	if first.ID != second.ID {
		t.Fatal("idempotency failed: different IDs for same client_ref")
	}
}

func TestMessageStateMachine_FullFlow(t *testing.T) {
	svc, _ := setupMessageService()

	msg, _ := svc.Create(context.Background(), domain.CreateMessageInput{
		TenantID: "tenant-1", ClientID: "key-1",
		Source: "FurySMS", Destination: "201234567890", Text: "Test",
	}, "user-1", uuid.New().String(), "127.0.0.1")

	// accepted → queued
	msg, err := svc.QueueMessage(context.Background(), msg.ID)
	if err != nil {
		t.Fatalf("QueueMessage failed: %v", err)
	}
	if msg.Status != domain.MessageStatusQueued {
		t.Fatalf("status = %s, want queued", msg.Status)
	}

	// queued → sending
	msg, err = svc.SendMessage(context.Background(), msg.ID, "conn-1", "route-1")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	if msg.Status != domain.MessageStatusSending {
		t.Fatalf("status = %s, want sending", msg.Status)
	}

	// sending → sent
	msg, err = svc.MarkSent(context.Background(), msg.ID, "ext-123", 1, 5000, 2000)
	if err != nil {
		t.Fatalf("MarkSent failed: %v", err)
	}
	if msg.Status != domain.MessageStatusSent {
		t.Fatalf("status = %s, want sent", msg.Status)
	}

	// sent → delivered
	msg, err = svc.MarkDelivered(context.Background(), msg.ID, "dlr-abc")
	if err != nil {
		t.Fatalf("MarkDelivered failed: %v", err)
	}
	if msg.Status != domain.MessageStatusDelivered {
		t.Fatalf("status = %s, want delivered", msg.Status)
	}
}

func TestMessageStateMachine_InvalidTransition(t *testing.T) {
	svc, _ := setupMessageService()

	msg, _ := svc.Create(context.Background(), domain.CreateMessageInput{
		TenantID: "tenant-1", ClientID: "key-1",
		Source: "FurySMS", Destination: "201234567890", Text: "Test",
	}, "user-1", uuid.New().String(), "127.0.0.1")

	// Try accepted → delivered (invalid)
	_, err := svc.MarkDelivered(context.Background(), msg.ID, "dlr-abc")
	if err == nil {
		t.Fatal("expected error for invalid transition: accepted -> delivered")
	}
}

func TestMessageStateMachine_FailedFlow(t *testing.T) {
	svc, _ := setupMessageService()

	msg, _ := svc.Create(context.Background(), domain.CreateMessageInput{
		TenantID: "tenant-1", ClientID: "key-1",
		Source: "FurySMS", Destination: "201234567890", Text: "Test",
	}, "user-1", uuid.New().String(), "127.0.0.1")

	// accepted → failed
	msg, err := svc.MarkFailed(context.Background(), msg.ID, "ERR001", "Insufficient balance")
	if err != nil {
		t.Fatalf("MarkFailed failed: %v", err)
	}
	if msg.Status != domain.MessageStatusFailed {
		t.Fatalf("status = %s, want failed", msg.Status)
	}
}

func TestMessageStateMachine_RetryFlow(t *testing.T) {
	svc, _ := setupMessageService()

	msg, _ := svc.Create(context.Background(), domain.CreateMessageInput{
		TenantID: "tenant-1", ClientID: "key-1",
		Source: "FurySMS", Destination: "201234567890", Text: "Test",
	}, "user-1", uuid.New().String(), "127.0.0.1")

	// accepted → queued → sending → sent → retrying → sending → sent
	msg, _ = svc.QueueMessage(context.Background(), msg.ID)
	msg, _ = svc.SendMessage(context.Background(), msg.ID, "conn-1", "route-1")
	msg, _ = svc.MarkSent(context.Background(), msg.ID, "ext-1", 1, 5000, 2000)

	msg, err := svc.MarkRetrying(context.Background(), msg.ID, "TIMEOUT", "Connection timeout")
	if err != nil {
		t.Fatalf("MarkRetrying failed: %v", err)
	}
	if msg.Status != domain.MessageStatusRetrying {
		t.Fatalf("status = %s, want retrying", msg.Status)
	}
	if msg.RetryCount != 1 {
		t.Fatalf("retry_count = %d, want 1", msg.RetryCount)
	}

	// retrying → sending
	msg, err = svc.SendMessage(context.Background(), msg.ID, "conn-1", "route-1")
	if err != nil {
		t.Fatalf("SendMessage after retry failed: %v", err)
	}
	if msg.Status != domain.MessageStatusSending {
		t.Fatalf("status = %s, want sending", msg.Status)
	}
}

func TestGetMessageByClientRef(t *testing.T) {
	svc, _ := setupMessageService()

	created, _ := svc.Create(context.Background(), domain.CreateMessageInput{
		TenantID: "tenant-1", ClientID: "key-1",
		Source: "FurySMS", Destination: "201234567890", Text: "Hello",
		ClientRef: "my-ref-1",
	}, "user-1", uuid.New().String(), "127.0.0.1")

	fetched, err := svc.GetByClientRef(context.Background(), "tenant-1", "my-ref-1")
	if err != nil {
		t.Fatalf("GetByClientRef failed: %v", err)
	}
	if fetched.ID != created.ID {
		t.Fatal("message ID mismatch")
	}
}

func TestListMessages_ByTenant(t *testing.T) {
	svc, _ := setupMessageService()

	for i := 0; i < 3; i++ {
		svc.Create(context.Background(), domain.CreateMessageInput{
			TenantID: "tenant-1", ClientID: "key-1",
			Source: "FurySMS", Destination: "201234567890", Text: "Msg",
		}, "user-1", uuid.New().String(), "127.0.0.1")
	}

	result, err := svc.List(context.Background(), domain.MessageFilter{
		TenantID: "tenant-1",
		Page:     domain.Page{Limit: 10, Offset: 0},
	})
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result.Items))
	}
}
