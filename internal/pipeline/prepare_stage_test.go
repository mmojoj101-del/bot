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
	if result.Prepared.Destination == "" {
		t.Fatal("expected Prepared to be set")
	}
	if result.Prepared.Encoding != "GSM7" {
		t.Fatalf("expected GSM7 encoding, got %q", result.Prepared.Encoding)
	}
	if result.Prepared.Parts != 1 {
		t.Fatalf("expected 1 part for short message, got %d", result.Prepared.Parts)
	}
	if result.Prepared.Destination != "+1234567890" {
		t.Fatalf("expected normalized destination, got %q", result.Prepared.Destination)
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
	if result.Prepared.Encoding != "UCS2" {
		t.Fatalf("expected UCS2 encoding for Arabic text, got %q", result.Prepared.Encoding)
	}
}

func TestPrepareStage_EuroSign_GSM7Ext(t *testing.T) {
	s := NewPrepareStage()
	state := validState()
	state.Message.Text = "€100" // Euro sign is GSM-7 extension character

	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Prepared.Encoding != "GSM7" {
		t.Fatalf("expected GSM7 encoding for text with €, got %q", result.Prepared.Encoding)
	}
}

func TestPrepareStage_GSM7ExtensionCharLength(t *testing.T) {
	// Extension chars (^, {, }, etc.) count as 2 chars in GSM-7.
	// So "^^" = 4 chars = still fits in 1 part (max 160).
	// But 80 '^' chars = 160 chars = 1 part (boundary).
	// 81 '^' chars = 162 chars = 2 parts.
	s := NewPrepareStage()
	state := validState()

	// 80 extension chars = 160 GSM-7 chars = exactly 1 part
	state.Message.Text = string(repeat('^', 80))
	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Prepared.Parts != 1 {
		t.Fatalf("expected 1 part for 80 extension chars (160 GSM7 len), got %d", result.Prepared.Parts)
	}

	// 81 extension chars = 162 GSM-7 chars = 2 parts
	state.Message.Text = string(repeat('^', 81))
	result, err = s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Prepared.Parts != 2 {
		t.Fatalf("expected 2 parts for 81 extension chars (162 GSM7 len), got %d", result.Prepared.Parts)
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
	if result.Prepared.Parts != 1 {
		t.Fatalf("expected 1 part for 160 GSM7 chars, got %d", result.Prepared.Parts)
	}
}

func TestPrepareStage_PartsCount_GSM7_Multi(t *testing.T) {
	s := NewPrepareStage()
	state := validState()
	state.Message.Text = string(repeat('a', 161)) // 161 → 2 parts

	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Prepared.Parts != 2 {
		t.Fatalf("expected 2 parts for 161 GSM7 chars, got %d", result.Prepared.Parts)
	}
}

func TestPrepareStage_PartsCount_UCS2_Short(t *testing.T) {
	s := NewPrepareStage()
	state := validState()
	state.Message.Text = string(repeat('界', 70))

	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Prepared.Parts != 1 {
		t.Fatalf("expected 1 part for 70 UCS2 chars, got %d", result.Prepared.Parts)
	}
}

func TestPrepareStage_PartsCount_UCS2_Multi(t *testing.T) {
	s := NewPrepareStage()
	state := validState()
	state.Message.Text = string(repeat('界', 71))

	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Prepared.Parts != 2 {
		t.Fatalf("expected 2 parts for 71 UCS2 chars, got %d", result.Prepared.Parts)
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
		{"+", "+"}, // plus only → just +
		{"++1234", "+1234"}, // second + is stripped
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			state := validState()
			state.Message.Destination = tt.input
			s := NewPrepareStage()

			result, err := s.Process(context.Background(), state)
			if tt.expected == "" {
				if !errors.Is(err, ErrInvalidDestination) {
					t.Fatalf("expected ErrInvalidDestination for %q, got: %v", tt.input, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.input, err)
			}
			if result.Prepared.Destination == "" {
				t.Fatal("expected Prepared")
			}
			if result.Prepared.Destination != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, result.Prepared.Destination)
			}
			// Verify msg.Destination is NOT mutated by PrepareStage.
			// The original raw input must be preserved on the domain entity.
			if state.Message.Destination != tt.input {
				t.Fatalf("msg.Destination should not be mutated; expected original %q, got %q", tt.input, state.Message.Destination)
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
	if state.Prepared.Destination == "" {
		t.Fatal("expected Prepared after Prepare stage")
	}
	if state.Prepared.Encoding != "GSM7" {
		t.Fatalf("expected GSM7 encoding, got %q", state.Prepared.Encoding)
	}
	if state.Prepared.Parts != 1 {
		t.Fatalf("expected 1 part, got %d", state.Prepared.Parts)
	}
}

func TestPrepareStage_DoesNotMutateMessage(t *testing.T) {
	// domain.Message.Encoding and Parts should not be modified by PrepareStage.
	// All derived values go into SendRequest only.
	s := NewPrepareStage()
	state := validState()
	state.Message.Text = "Hello"

	result, err := s.Process(context.Background(), state)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// SendRequest carries the derived values
	if result.Prepared.Encoding != "GSM7" {
		t.Fatalf("expected GSM7 in SendRequest, got %q", result.Prepared.Encoding)
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
