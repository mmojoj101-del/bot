package pipeline

import (
	"context"
	"errors"
	"testing"
)

func TestPrepareStage_ValidMessageGSM7(t *testing.T) {
	s := NewPrepareStage()
	state := validState()
	state.Message.Text = "Hello, World!"

	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.SendRequest == nil {
		t.Fatal("expected SendRequest to be set")
	}
	if result.SendRequest.Encoding != "GSM7" {
		t.Fatalf("expected GSM7 encoding, got %q", result.SendRequest.Encoding)
	}
	if result.SendRequest.Parts != 1 {
		t.Fatalf("expected 1 part for short message, got %d", result.SendRequest.Parts)
	}
	if result.SendRequest.Destination != "+1234567890" {
		t.Fatalf("expected normalized destination, got %q", result.SendRequest.Destination)
	}
}

func TestPrepareStage_UCS2Encoding(t *testing.T) {
	s := NewPrepareStage()
	state := validState()
	state.Message.Text = "مرحبا بالعالم" // Arabic — requires UCS-2

	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.SendRequest.Encoding != "UCS2" {
		t.Fatalf("expected UCS2 encoding for Arabic text, got %q", result.SendRequest.Encoding)
	}
}

func TestPrepareStage_EuroEncoding(t *testing.T) {
	s := NewPrepareStage()
	state := validState()
	state.Message.Text = "Price: €100" // Euro sign is GSM7 extension

	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	// € is a GSM-7 extension character, still GSM7
	if result.SendRequest.Encoding != "GSM7" {
		t.Fatalf("expected GSM7 encoding for text with €, got %q", result.SendRequest.Encoding)
	}
}

func TestPrepareStage_PartsCount_GSM7_Short(t *testing.T) {
	s := NewPrepareStage()
	state := validState()
	state.Message.Text = string(repeat('a', 160))

	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.SendRequest.Parts != 1 {
		t.Fatalf("expected 1 part for 160 GSM7 chars, got %d", result.SendRequest.Parts)
	}
}

func TestPrepareStage_PartsCount_GSM7_Multi(t *testing.T) {
	s := NewPrepareStage()
	state := validState()
	state.Message.Text = string(repeat('a', 161)) // 161 chars → 2 parts (153 + 8)

	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.SendRequest.Parts != 2 {
		t.Fatalf("expected 2 parts for 161 GSM7 chars, got %d", result.SendRequest.Parts)
	}
}

func TestPrepareStage_PartsCount_UCS2_Short(t *testing.T) {
	s := NewPrepareStage()
	state := validState()
	state.Message.Text = string(repeat('界', 70)) // 70 UCS-2 chars

	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.SendRequest.Parts != 1 {
		t.Fatalf("expected 1 part for 70 UCS2 chars, got %d", result.SendRequest.Parts)
	}
}

func TestPrepareStage_PartsCount_UCS2_Multi(t *testing.T) {
	s := NewPrepareStage()
	state := validState()
	state.Message.Text = string(repeat('界', 71)) // 71 UCS-2 chars → 2 parts

	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.SendRequest.Parts != 2 {
		t.Fatalf("expected 2 parts for 71 UCS2 chars, got %d", result.SendRequest.Parts)
	}
}

func TestPrepareStage_PhoneNormalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"+1234567890", "+1234567890"},
		{"1234567890", "+1234567890"},
		{"+1 (234) 567-8900", "+12345678900"},
		{"+1-234-567-8900", "+12345678900"},
		{"  +1 234 567 8900  ", "+12345678900"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			state := validState()
			state.Message.Destination = tt.input
			s := NewPrepareStage()

			_, err := s.Process(context.Background(), state)
			if tt.expected == "" {
				if !errors.Is(err, ErrInvalidDestination) {
					t.Fatalf("expected ErrInvalidDestination for empty input, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if state.Message.Destination != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, state.Message.Destination)
			}
		})
	}
}

func TestPrepareStage_Name(t *testing.T) {
	s := NewPrepareStage()
	if s.Name() != "prepare" {
		t.Fatalf("expected 'prepare', got %q", s.Name())
	}
}

func TestPrepareStage_EmptyText(t *testing.T) {
	s := NewPrepareStage()
	state := validState()
	state.Message.Text = ""

	_, err := s.Process(context.Background(), state)
	if !errors.Is(err, ErrInvalidEncoding) {
		t.Fatalf("expected ErrInvalidEncoding for empty text, got: %v", err)
	}
}

func TestPipeline_ValidateThenPrepare(t *testing.T) {
	// Integration test: Validate → Prepare sequence
	p := New(NewValidateStage(), NewPrepareStage())

	state := validState()
	err := p.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("expected pipeline to succeed, got: %v", err)
	}
	if state.SendRequest == nil {
		t.Fatal("expected SendRequest after Prepare stage")
	}
	if state.SendRequest.Encoding != "GSM7" {
		t.Fatalf("expected GSM7 encoding, got %q", state.SendRequest.Encoding)
	}
	if state.SendRequest.Parts != 1 {
		t.Fatalf("expected 1 part, got %d", state.SendRequest.Parts)
	}
}

// repeat creates a string with n repetitions of character c.
func repeat(c rune, n int) string {
	result := make([]rune, n)
	for i := range result {
		result[i] = c
	}
	return string(result)
}
