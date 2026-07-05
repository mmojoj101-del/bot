package connector

import (
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// DefaultDLRMapper maps common provider delivery statuses.
type DefaultDLRMapper struct {
	connectorType domain.ConnectorType
}

func NewDefaultDLRMapper(connectorType domain.ConnectorType) *DefaultDLRMapper {
	return &DefaultDLRMapper{connectorType: connectorType}
}

func (m *DefaultDLRMapper) ConnectorType() domain.ConnectorType {
	return m.connectorType
}

// MapProviderStatus maps a provider-specific status string to internal DLR and message statuses.
// Supports common formats: DELIVRD, DELIVERED, FAILED, EXPIRED, UNDELIV, UNKNOWN.
func (m *DefaultDLRMapper) MapProviderStatus(providerStatus string) (domain.DLRStatus, domain.MessageStatus) {
	switch providerStatus {
	case "DELIVRD", "DELIVERED", "SUCCESS", "OK", "0":
		return domain.DLRStatusDelivered, domain.MessageStatusDelivered
	case "FAILED", "REJECTED", "UNDELIV", "UNDELIVERED", "EXPIRED", "UNKNOWN":
		return domain.DLRStatusFailed, domain.MessageStatusFailed
	case "PENDING", "ENROUTE", "ACCEPTED", "SUBMITTED":
		return domain.DLRStatusPending, domain.MessageStatusSent
	case "sent", "SENT":
		return domain.DLRStatusPending, domain.MessageStatusSent
	default:
		return domain.DLRStatusPending, domain.MessageStatusSent
	}
}

var _ domain.DLRMapper = (*DefaultDLRMapper)(nil)
