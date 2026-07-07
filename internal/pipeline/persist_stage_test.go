package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// mockMessageRepo implements domain.MessageRepository for testing.
type mockMessageRepo struct {
	updatedID      string
	updatedInput   domain.UpdateMessageInput
	updatedVersion int
	updateErr      error
}

func (m *mockMessageRepo) UpdateStatus(_ context.Context, id string, input domain.UpdateMessageInput, version int) (*domain.Message, error) {
	m.updatedID = id
	m.updatedInput = input
	m.updatedVersion = version
	if m.updateErr != nil {
		return nil, m.updateErr
	}
	return &domain.Message{BaseModel: domain.BaseModel{ID: id}, Status: *input.Status}, nil
}

func (m *mockMessageRepo) Create(_ context.Context, _ domain.CreateMessageInput, _ string) (*domain.Message, error) {
	return nil, nil
}
func (m *mockMessageRepo) GetByID(_ context.Context, _ string) (*domain.Message, error) {
	return nil, nil
}
func (m *mockMessageRepo) GetByClientRef(_ context.Context, _, _ string) (*domain.Message, error) {
	return nil, nil
}
func (m *mockMessageRepo) GetByExternalID(_ context.Context, _ string) (*domain.Message, error) {
	return nil, nil
}
func (m *mockMessageRepo) AppendDLR(_ context.Context, _ *domain.DLRRecord) error {
	return nil
}
func (m *mockMessageRepo) List(_ context.Context, _ domain.MessageFilter) (domain.PageResult[domain.Message], error) {
	return domain.PageResult[domain.Message]{}, nil
}
func (m *mockMessageRepo) Count(_ context.Context, _ domain.MessageFilter) (int64, error) {
	return 0, nil
}
func (m *mockMessageRepo) Delete(_ context.Context, _ string) error {
	return nil
}

func bm(id string, version int) domain.BaseModel {
	return domain.BaseModel{ID: id, Version: version}
}

func TestPersistStage_Name(t *testing.T) {
	s := NewPersistStage(&mockMessageRepo{})
	if got := s.Name(); got != "persist" {
		t.Errorf("Name() = %q, want %q", got, "persist")
	}
}

func TestPersistStage_MissingArtifacts(t *testing.T) {
	tests := []struct {
		name  string
		state *PipelineState
		err   error
	}{
		{"nil message", NewPipelineState(nil, "trace-1"), ErrPersistNoMessage},
		{"nil send result", func() *PipelineState {
			s := NewPipelineState(&domain.Message{BaseModel: bm("msg-1", 0)}, "trace-1")
			s.Decision = &RoutingDecision{}
			s.DeliveryOutcome = &DeliveryOutcome{Status: domain.MessageStatusSent}
			return s
		}(), ErrPersistNoSendResult},
		{"nil delivery outcome", func() *PipelineState {
			s := NewPipelineState(&domain.Message{BaseModel: bm("msg-1", 0)}, "trace-1")
			s.SendResult = &SendResult{}
			s.Decision = &RoutingDecision{}
			return s
		}(), ErrPersistNoDeliveryOutcome},
		{"nil decision", func() *PipelineState {
			s := NewPipelineState(&domain.Message{BaseModel: bm("msg-1", 0)}, "trace-1")
			s.SendResult = &SendResult{}
			s.DeliveryOutcome = &DeliveryOutcome{Status: domain.MessageStatusSent}
			return s
		}(), ErrPersistNoDecision},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewPersistStage(&mockMessageRepo{})
			_, err := s.Process(context.Background(), tt.state)
			if err == nil || err != tt.err {
				t.Errorf("got %v, want %v", err, tt.err)
			}
		})
	}
}

func TestPersistStage_Sent(t *testing.T) {
	repo := &mockMessageRepo{}
	now := time.Now().UTC()

	msg := &domain.Message{BaseModel: bm("msg-1", 3)}
	state := NewPipelineState(msg, "trace-1")
	state.SendResult = &SendResult{
		Success:    true,
		ExternalID: "ext-123",
		Parts:      1,
	}
	state.Decision = &RoutingDecision{
		ConnectorID: "http-1",
		RouteID:     "route-1",
	}
	state.DeliveryOutcome = &DeliveryOutcome{
		Status: domain.MessageStatusSent,
		SentAt: &now,
	}

	s := NewPersistStage(repo)
	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != state {
		t.Error("expected state to be returned unchanged")
	}

	if repo.updatedID != "msg-1" {
		t.Errorf("updatedID = %q, want msg-1", repo.updatedID)
	}
	if repo.updatedVersion != 3 {
		t.Errorf("updatedVersion = %d, want 3", repo.updatedVersion)
	}
	if repo.updatedInput.Status == nil || *repo.updatedInput.Status != domain.MessageStatusSent {
		t.Errorf("Status = %v, want sent", repo.updatedInput.Status)
	}
	if repo.updatedInput.ExternalID == nil || *repo.updatedInput.ExternalID != "ext-123" {
		t.Errorf("ExternalID = %v, want ext-123", repo.updatedInput.ExternalID)
	}
	if repo.updatedInput.Parts == nil || *repo.updatedInput.Parts != 1 {
		t.Errorf("Parts = %v, want 1", repo.updatedInput.Parts)
	}
	// Timestamps copied directly from DeliveryOutcome
	if repo.updatedInput.SentAt == nil || !repo.updatedInput.SentAt.Equal(now) {
		t.Errorf("SentAt = %v, want %v", repo.updatedInput.SentAt, now)
	}
}

func TestPersistStage_Sent_NoExternalID(t *testing.T) {
	repo := &mockMessageRepo{}

	msg := &domain.Message{BaseModel: bm("msg-1", 1)}
	state := NewPipelineState(msg, "trace-1")
	state.SendResult = &SendResult{
		Success:    true,
		ExternalID: "",
		Parts:      1,
	}
	state.Decision = &RoutingDecision{ConnectorID: "http-1", RouteID: "route-1"}
	state.DeliveryOutcome = &DeliveryOutcome{
		Status:      domain.MessageStatusSent,
		AwaitingDLR: true,
	}

	s := NewPersistStage(repo)
	_, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.updatedInput.ExternalID != nil {
		t.Errorf("ExternalID = %v, want nil (no external ID)", *repo.updatedInput.ExternalID)
	}
}

func TestPersistStage_Delivered(t *testing.T) {
	repo := &mockMessageRepo{}
	now := time.Now().UTC()

	msg := &domain.Message{BaseModel: bm("msg-1", 2)}
	state := NewPipelineState(msg, "trace-1")
	state.SendResult = &SendResult{
		Success:    true,
		ExternalID: "ext-456",
		Parts:      1,
	}
	state.Decision = &RoutingDecision{ConnectorID: "http-1", RouteID: "route-1"}
	state.DeliveryOutcome = &DeliveryOutcome{
		Status:      domain.MessageStatusDelivered,
		SentAt:      &now,
		DeliveredAt: &now,
	}

	s := NewPersistStage(repo)
	_, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.updatedInput.DeliveredAt == nil {
		t.Error("Expected DeliveredAt to be copied")
	}
	if repo.updatedInput.SentAt == nil {
		t.Error("Expected SentAt to be copied")
	}
}

func TestPersistStage_Failed(t *testing.T) {
	repo := &mockMessageRepo{}
	now := time.Now().UTC()

	msg := &domain.Message{BaseModel: bm("msg-1", 1)}
	state := NewPipelineState(msg, "trace-1")
	state.SendResult = &SendResult{
		Success:      false,
		ExternalID:   "",
		ErrorCode:    "400",
		ErrorMessage: "invalid destination",
		Parts:        0,
	}
	state.Decision = &RoutingDecision{ConnectorID: "http-1", RouteID: "route-1"}
	state.DeliveryOutcome = &DeliveryOutcome{
		Status:      domain.MessageStatusFailed,
		FailureKind: FailurePermanent,
		FailedAt:    &now,
		SentAt:      &now,
	}

	s := NewPersistStage(repo)
	_, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.updatedInput.FailedAt == nil {
		t.Error("Expected FailedAt to be copied")
	}
	if repo.updatedInput.ErrorCode == nil || *repo.updatedInput.ErrorCode != "400" {
		t.Errorf("ErrorCode = %v, want 400", repo.updatedInput.ErrorCode)
	}
	if repo.updatedInput.ErrorMessage == nil || *repo.updatedInput.ErrorMessage != "invalid destination" {
		t.Errorf("ErrorMessage = %v, want invalid destination", repo.updatedInput.ErrorMessage)
	}
}

func TestPersistStage_ExistingTimestampsCopied(t *testing.T) {
	repo := &mockMessageRepo{}
	sentAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	deliveredAt := time.Date(2025, 1, 1, 0, 5, 0, 0, time.UTC)

	msg := &domain.Message{BaseModel: bm("msg-1", 5)}
	state := NewPipelineState(msg, "trace-1")
	state.SendResult = &SendResult{Success: true, ExternalID: "ext-789", Parts: 1}
	state.Decision = &RoutingDecision{ConnectorID: "http-1", RouteID: "route-1"}
	state.DeliveryOutcome = &DeliveryOutcome{
		Status:      domain.MessageStatusDelivered,
		SentAt:      &sentAt,
		DeliveredAt: &deliveredAt,
	}

	s := NewPersistStage(repo)
	_, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both timestamps are copied verbatim from DeliveryOutcome.
	if repo.updatedInput.SentAt == nil || !repo.updatedInput.SentAt.Equal(sentAt) {
		t.Errorf("SentAt = %v, want %v", repo.updatedInput.SentAt, sentAt)
	}
	if repo.updatedInput.DeliveredAt == nil || !repo.updatedInput.DeliveredAt.Equal(deliveredAt) {
		t.Errorf("DeliveredAt = %v, want %v", repo.updatedInput.DeliveredAt, deliveredAt)
	}
}

func TestPersistStage_ConnectorIDAndRouteID(t *testing.T) {
	repo := &mockMessageRepo{}

	msg := &domain.Message{BaseModel: bm("msg-1", 1)}
	state := NewPipelineState(msg, "trace-1")
	state.SendResult = &SendResult{Success: true, ExternalID: "ext", Parts: 1}
	state.Decision = &RoutingDecision{
		ConnectorID: "smpp-1",
		RouteID:     "route-premium",
	}
	state.DeliveryOutcome = &DeliveryOutcome{Status: domain.MessageStatusSent}

	s := NewPersistStage(repo)
	_, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.updatedInput.ConnectorID == nil || *repo.updatedInput.ConnectorID != "smpp-1" {
		t.Errorf("ConnectorID = %v, want smpp-1", repo.updatedInput.ConnectorID)
	}
	if repo.updatedInput.RouteID == nil || *repo.updatedInput.RouteID != "route-premium" {
		t.Errorf("RouteID = %v, want route-premium", repo.updatedInput.RouteID)
	}
}

func TestPersistStage_RepositoryError(t *testing.T) {
	repo := &mockMessageRepo{updateErr: ErrPersistUpdateFailed}

	msg := &domain.Message{BaseModel: bm("msg-1", 1)}
	state := NewPipelineState(msg, "trace-1")
	state.SendResult = &SendResult{Success: true, ExternalID: "ext", Parts: 1}
	state.Decision = &RoutingDecision{ConnectorID: "http-1", RouteID: "route-1"}
	state.DeliveryOutcome = &DeliveryOutcome{Status: domain.MessageStatusSent}

	s := NewPersistStage(repo)
	_, err := s.Process(context.Background(), state)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPersistStage_VerifiesErrorMapping(t *testing.T) {
	repo := &mockMessageRepo{}
	msg := &domain.Message{BaseModel: bm("msg-1", 1)}

	state := NewPipelineState(msg, "trace-1")
	state.SendResult = &SendResult{
		Success:      true,
		ExternalID:   "ext-1",
		ErrorCode:    "0",
		ErrorMessage: "ok",
		Parts:        2,
	}
	state.Decision = &RoutingDecision{ConnectorID: "http-1", RouteID: "route-1"}
	state.DeliveryOutcome = &DeliveryOutcome{Status: domain.MessageStatusSent}

	s := NewPersistStage(repo)
	_, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.updatedInput.ErrorCode == nil || *repo.updatedInput.ErrorCode != "0" {
		t.Errorf("ErrorCode = %v, want 0", repo.updatedInput.ErrorCode)
	}
	if repo.updatedInput.Parts == nil || *repo.updatedInput.Parts != 2 {
		t.Errorf("Parts = %v, want 2", repo.updatedInput.Parts)
	}
}
