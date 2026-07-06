package pipeline

import "context"

// Stage is a single step in the message lifecycle pipeline.
// Each stage is:
//   - Self-contained (single responsibility)
//   - Pluggable (add/remove/reorder without changing other stages)
//   - Testable in isolation (mock adjacent stages)
//   - Observable (TraceID + metrics per stage via PipelineState)
type Stage interface {
	// Name returns the stage name for logging, metrics, and tracing.
	Name() string

	// Process executes this stage's logic.
	// It receives and returns *PipelineState — the single state object
	// that flows through all stages.
	Process(ctx context.Context, state *PipelineState) (*PipelineState, error)
}
