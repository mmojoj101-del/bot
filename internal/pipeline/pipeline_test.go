package pipeline

import (
	"context"
	"errors"
	"testing"
)

// mockStage is a test helper that implements Stage.
type mockStage struct {
	name    string
	process func(ctx context.Context, state *PipelineState) (*PipelineState, error)
}

func (m *mockStage) Name() string { return m.name }
func (m *mockStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	if m.process != nil {
		return m.process(ctx, state)
	}
	return state, nil
}

func TestPipeline_Execute_AllStagesRun(t *testing.T) {
	var order []string

	stage1 := &mockStage{
		name: "stage1",
		process: func(ctx context.Context, state *PipelineState) (*PipelineState, error) {
			order = append(order, "stage1")
			return state, nil
		},
	}
	stage2 := &mockStage{
		name: "stage2",
		process: func(ctx context.Context, state *PipelineState) (*PipelineState, error) {
			order = append(order, "stage2")
			return state, nil
		},
	}
	stage3 := &mockStage{
		name: "stage3",
		process: func(ctx context.Context, state *PipelineState) (*PipelineState, error) {
			order = append(order, "stage3")
			return state, nil
		},
	}

	p := New(stage1, stage2, stage3)
	state := &PipelineState{Metadata: make(map[string]interface{})}

	err := p.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(order) != 3 {
		t.Fatalf("expected 3 stages to run, got %d", len(order))
	}
	if order[0] != "stage1" || order[1] != "stage2" || order[2] != "stage3" {
		t.Fatalf("wrong execution order: got %v", order)
	}
}

func TestPipeline_Execute_StopsOnError(t *testing.T) {
	var order []string
	expectedErr := errors.New("stage2 failed")

	stage1 := &mockStage{
		name: "stage1",
		process: func(ctx context.Context, state *PipelineState) (*PipelineState, error) {
			order = append(order, "stage1")
			return state, nil
		},
	}
	stage2 := &mockStage{
		name: "stage2",
		process: func(ctx context.Context, state *PipelineState) (*PipelineState, error) {
			order = append(order, "stage2")
			return state, expectedErr
		},
	}
	stage3 := &mockStage{
		name: "stage3",
		process: func(ctx context.Context, state *PipelineState) (*PipelineState, error) {
			order = append(order, "stage3")
			return state, nil
		},
	}

	p := New(stage1, stage2, stage3)
	state := &PipelineState{Metadata: make(map[string]interface{})}

	err := p.Execute(context.Background(), state)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}

	if len(order) != 2 {
		t.Fatalf("expected 2 stages to run before error, got %d", len(order))
	}
}

func TestPipeline_Execute_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	stage := &mockStage{
		name: "stage",
		process: func(ctx context.Context, state *PipelineState) (*PipelineState, error) {
			return state, nil
		},
	}

	p := New(stage)
	state := &PipelineState{Metadata: make(map[string]interface{})}

	err := p.Execute(ctx, state)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
}

func TestPipeline_Execute_NilState(t *testing.T) {
	p := New(&mockStage{name: "stage"})
	err := p.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil state, got nil")
	}
}

func TestPipeline_StageCount(t *testing.T) {
	p := New(
		&mockStage{name: "a"},
		&mockStage{name: "b"},
		&mockStage{name: "c"},
	)
	if p.StageCount() != 3 {
		t.Fatalf("expected 3 stages, got %d", p.StageCount())
	}
}

func TestPipeline_AddStage(t *testing.T) {
	p := New(&mockStage{name: "a"})
	p.AddStage(&mockStage{name: "b"})
	if p.StageCount() != 2 {
		t.Fatalf("expected 2 stages after AddStage, got %d", p.StageCount())
	}
}

func TestPipelineState_New(t *testing.T) {
	state := NewPipelineState(nil, "trace-1")
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if state.TraceID != "trace-1" {
		t.Fatalf("expected trace-1, got %s", state.TraceID)
	}
	if state.Metadata == nil {
		t.Fatal("expected non-nil metadata map")
	}
}
