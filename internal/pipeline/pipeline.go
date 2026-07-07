package pipeline

import (
	"context"
	"fmt"
)

// Pipeline executes a sequence of Stage implementations in order.
// Each stage receives *PipelineState and returns (*PipelineState, error).
// If any stage returns an error, pipeline execution stops.
type Pipeline struct {
	stages []Stage
}

// New creates a new Pipeline with the given stages.
func New(stages ...Stage) *Pipeline {
	return &Pipeline{stages: stages}
}

// Execute runs all stages in order.
// If a stage returns an error, execution stops and the error is returned
// along with the state at the point of failure.
func (p *Pipeline) Execute(ctx context.Context, state *PipelineState) error {
	if state == nil {
		return fmt.Errorf("pipeline: state is nil")
	}

	for i, stage := range p.stages {
		select {
		case <-ctx.Done():
			return fmt.Errorf("pipeline: cancelled at stage %d (%s): %w", i, stage.Name(), ctx.Err())
		default:
		}

		var err error
		state, err = stage.Process(ctx, state)
		if err != nil {
			// Error is returned directly — not duplicated on state.
			// Stages communicate through typed fields (Prepared, Decision, SendResult).
			return fmt.Errorf("pipeline: stage %q failed: %w", stage.Name(), err)
		}
		if state == nil {
			return fmt.Errorf("pipeline: stage %q returned nil state", stage.Name())
		}
	}
	return nil
}

// AddStage appends a stage to the pipeline.
// This is safe only during pipeline construction, not during execution.
func (p *Pipeline) AddStage(stage Stage) {
	p.stages = append(p.stages, stage)
}

// StageCount returns the number of registered stages.
func (p *Pipeline) StageCount() int {
	return len(p.stages)
}
