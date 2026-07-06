package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

func TestValidateStage_ValidState(t *testing.T) {
	s := NewValidateStage()
	state := validState()

	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result state")
	}
}

func TestValidateStage_NilState(t *testing.T) {
	s := NewValidateStage()
	_, err := s.Process(context.Background(), nil)
	if !errors.Is(err, ErrNilMessage) {
		t.Fatalf("expected ErrNilMessage, got: %v", err)
	}
}

func TestValidateStage_NilMessage(t *testing.T) {
	s := NewValidateStage()
	state := &PipelineState{Metadata: make(map[string]interface{})}
	_, err := s.Process(context.Background(), state)
	if !errors.Is(err, ErrNilMessage) {
		t.Fatalf("expected ErrNilMessage, got: %v", err)
	}
}

func TestValidateStage_MissingTenantID(t *testing.T) {
	s := NewValidateStage()
	state := validState()
	state.Message.TenantID = ""

	_, err := s.Process(context.Background(), state)
	if !errors.Is(err, ErrMissingTenantID) {
		t.Fatalf("expected ErrMissingTenantID, got: %v", err)
	}
}

func TestValidateStage_MissingDestination(t *testing.T) {
	s := NewValidateStage()
	state := validState()
	state.Message.Destination = ""

	_, err := s.Process(context.Background(), state)
	if !errors.Is(err, ErrMissingDestination) {
		t.Fatalf("expected ErrMissingDestination, got: %v", err)
	}
}

func TestValidateStage_MissingContent(t *testing.T) {
	s := NewValidateStage()
	state := validState()
	state.Message.Text = ""

	_, err := s.Process(context.Background(), state)
	if !errors.Is(err, ErrMissingContent) {
		t.Fatalf("expected ErrMissingContent, got: %v", err)
	}
}

func TestValidateStage_InvalidStatus(t *testing.T) {
	tests := []struct {
		name   string
		status domain.MessageStatus
	}{
		{"accepted", domain.MessageStatusAccepted},
		{"sending", domain.MessageStatusSending},
		{"sent", domain.MessageStatusSent},
		{"delivered", domain.MessageStatusDelivered},
		{"failed", domain.MessageStatusFailed},
		{"retrying", domain.MessageStatusRetrying},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewValidateStage()
			state := validState()
			state.Message.Status = tt.status

			_, err := s.Process(context.Background(), state)
			if !errors.Is(err, ErrInvalidStatus) {
				t.Fatalf("expected ErrInvalidStatus for status %q, got: %v", tt.status, err)
			}
		})
	}
}

func TestValidateStage_ValidStatuses(t *testing.T) {
	tests := []domain.MessageStatus{
		domain.MessageStatusQueued,
	}

	for _, status := range tests {
		t.Run(string(status), func(t *testing.T) {
			s := NewValidateStage()
			state := validState()
			state.Message.Status = status

			_, err := s.Process(context.Background(), state)
			if err != nil {
				t.Fatalf("expected no error for status %q, got: %v", status, err)
			}
		})
	}
}

func TestValidateStage_Name(t *testing.T) {
	s := NewValidateStage()
	if s.Name() != "validate" {
		t.Fatalf("expected 'validate', got %q", s.Name())
	}
}

func TestPipeline_WithValidateStage_Valid(t *testing.T) {
	validateStage := NewValidateStage()
	p := New(validateStage)

	state := validState()
	err := p.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("expected pipeline to succeed, got: %v", err)
	}
}

func TestPipeline_WithValidateStage_InvalidMessage(t *testing.T) {
	validateStage := NewValidateStage()
	p := New(validateStage)

	state := validState()
	state.Message.TenantID = ""

	err := p.Execute(context.Background(), state)
	if err == nil {
		t.Fatal("expected pipeline to fail with missing tenant")
	}
}

// validState returns a PipelineState with a valid message that passes ValidateStage.
func validState() *PipelineState {
	return &PipelineState{
		Message: &domain.Message{
			TenantID:    "tenant-1",
			Destination: "+1234567890",
			Text:        "Hello, World!",
			Status:      domain.MessageStatusQueued,
		},
		Attempt:    0,
		MaxRetries: 3,
		Metadata:   make(map[string]interface{}),
	}
}
