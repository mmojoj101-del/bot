package handler

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// DLRHandler handles delivery receipt callbacks.
type DLRHandler struct {
	msgRepo     domain.MessageRepository
	connRepo    domain.ConnectorRepository
	dlrMapper   domain.DLRMapper
	metrics     domain.MetricsRecorder
}

func NewDLRHandler(
	msgRepo domain.MessageRepository,
	connRepo domain.ConnectorRepository,
	dlrMapper domain.DLRMapper,
	metrics domain.MetricsRecorder,
) *DLRHandler {
	return &DLRHandler{
		msgRepo:   msgRepo,
		connRepo:  connRepo,
		dlrMapper: dlrMapper,
		metrics:   metrics,
	}
}

// ReceiveDLR handles an incoming delivery receipt callback.
// POST /api/v1/dlr/:connector_id
func (h *DLRHandler) ReceiveDLR(c *fiber.Ctx) error {
	connectorID := c.Params("connector_id")
	if connectorID == "" {
		return BadRequest(c, "connector_id is required")
	}

	// Read raw body
	body := c.Body()

	// Get connector to determine the DLR format
	connector, err := h.connRepo.GetByID(c.Context(), connectorID)
	if err != nil {
		slog.Warn("dlr for unknown connector", "connector_id", connectorID)
		return NotFound(c, "connector not found")
	}

	// Parse DLR payload (provider-specific)
	dlrPayload := parseDLRPayload(body, c.GetReqHeaders())
	if dlrPayload.ExternalID == "" {
		return BadRequest(c, "external_id is required in DLR")
	}

	// Find the message by external ID
	msg, err := h.msgRepo.GetByExternalID(c.Context(), dlrPayload.ExternalID)
	if err != nil {
		slog.Warn("dlr for unknown message",
			"external_id", dlrPayload.ExternalID,
			"connector_id", connectorID,
		)
		return NotFound(c, "message not found")
	}

	// Idempotent check: skip if message is already in terminal state
	if msg.Status == domain.MessageStatusDelivered || msg.Status == domain.MessageStatusFailed {
		// Log the duplicate DLR but don't error
		h.appendDLR(c, msg, connector, dlrPayload)
		return NoContent(c)
	}

	// Map provider status to internal status
	dlrStatus, msgStatus := h.dlrMapper.MapProviderStatus(dlrPayload.ProviderStatus)

	// Record DLR
	h.appendDLR(c, msg, connector, dlrPayload)

	// Update message status (idempotent: UpdateStatus checks version)
	update := domain.UpdateMessageInput{
		Status:      &msgStatus,
		DLRStatus:   &dlrStatus,
		DLRID:       &dlrPayload.DLRID,
		ErrorCode:   &dlrPayload.ErrorCode,
		ErrorMessage: &dlrPayload.Description,
	}
	if msgStatus == domain.MessageStatusDelivered {
		now := time.Now().UTC()
		update.DeliveredAt = &now
	}
	if msgStatus == domain.MessageStatusFailed {
		now := time.Now().UTC()
		update.FailedAt = &now
	}

	_, err = h.msgRepo.UpdateStatus(c.Context(), msg.ID, update, int(msg.Version))
	if err == domain.ErrConflict {
		// Version conflict means another DLR already updated this message
		slog.Warn("dlr version conflict, ignoring duplicate",
			"message_id", msg.ID,
			"external_id", dlrPayload.ExternalID,
		)
		return NoContent(c)
	}
	if err != nil {
		slog.Error("dlr update failed", "message_id", msg.ID, "error", err)
		return InternalError(c, "failed to update message")
	}

	if h.metrics != nil {
		h.metrics.RecordDLRReceived(connectorID, string(dlrStatus))
	}

	slog.Info("dlr processed",
		"message_id", msg.ID,
		"external_id", dlrPayload.ExternalID,
		"status", msgStatus,
		"dlr_status", dlrStatus,
	)

	return Success(c, fiber.Map{"status": "ok"})
}

// dlrPayload represents a parsed delivery receipt.
type dlrPayload struct {
	ExternalID     string
	DLRID          string
	ProviderStatus string
	ErrorCode      string
	Description    string
}

// parseDLRPayload extracts DLR fields from the request body and headers.
// Supports both JSON and form-encoded DLRs.
func parseDLRPayload(body []byte, headers map[string][]string) dlrPayload {
	// Try JSON first
	var payload dlrPayload

	// Common JSON fields
	var jsonData map[string]interface{}
	if err := json.Unmarshal(body, &jsonData); err == nil {
		payload.ExternalID = stringField(jsonData, "external_id", "message_id", "id", "msgid")
		payload.ProviderStatus = stringField(jsonData, "status", "dlr_status", "state", "delivery_status")
		payload.ErrorCode = stringField(jsonData, "error_code", "err", "error")
		payload.Description = stringField(jsonData, "description", "error_message", "reason")
		payload.DLRID = stringField(jsonData, "dlr_id", "receipt_id", "callback_id")
	}

	return payload
}

func stringField(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := data[key]; ok {
			switch s := v.(type) {
			case string:
				return s
			case float64:
				return ""
			default:
				return ""
			}
		}
	}
	return ""
}

func (h *DLRHandler) appendDLR(c *fiber.Ctx, msg *domain.Message, conn *domain.Connector, payload dlrPayload) {
	dlr := &domain.DLRRecord{
		MessageID:     msg.ID,
		TenantID:      msg.TenantID,
		Status:        "", // will be determined by mapper
		ExternalID:    payload.ExternalID,
		ConnectorName: conn.Name,
		RemoteIP:      c.IP(),
		Headers:       toJSONBytes(c.GetReqHeaders()),
		RawPayload:    c.Body(),
		ErrorCode:     payload.ErrorCode,
		Description:   payload.Description,
		CreatedAt:     time.Now().UTC(),
	}

	// Set the DLR status based on the mapper
	dlrStatus, _ := h.dlrMapper.MapProviderStatus(payload.ProviderStatus)
	dlr.Status = dlrStatus

	if err := h.msgRepo.AppendDLR(c.Context(), dlr); err != nil {
		slog.Error("append dlr record", "message_id", msg.ID, "error", err)
	}
}

func toJSONBytes(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}
