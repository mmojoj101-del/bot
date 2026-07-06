package events

import "context"

// DomainEventPublisher is the interface the Worker depends on.
// The Worker calls Publish() and never knows the implementation:
//   - In-memory (MemoryBus)
//   - Outbox-pattern (PostgreSQL outbox_events table)
//   - Kafka / NATS / RabbitMQ
type DomainEventPublisher interface {
	// Publish publishes a domain event to all subscribers.
	Publish(ctx context.Context, envelope EventEnvelope) error
}

// DomainEventSubscriber is the interface event subscribers implement.
type DomainEventSubscriber interface {
	// HandleEvent processes a published event.
	HandleEvent(ctx context.Context, envelope EventEnvelope) error
}

// Ensure we implement basic compilation checks.
var _ DomainEventPublisher = (*NoopPublisher)(nil)

// NoopPublisher is a no-op implementation for testing.
type NoopPublisher struct{}

func (p *NoopPublisher) Publish(_ context.Context, _ EventEnvelope) error {
	return nil
}
