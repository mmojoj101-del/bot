package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/service"
)

// MessageHandler handles message CRUD and sending.
type MessageHandler struct {
	msgService *service.MessageService
}

// NewMessageHandler creates a new message handler.
func NewMessageHandler(msgService *service.MessageService) *MessageHandler {
	return &MessageHandler{
		msgService: msgService,
	}
}

// CreateMessage creates a new message.
// POST /api/v1/messages
func (h *MessageHandler) CreateMessage(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(string)
	tenantID := c.Locals("tenant_id").(string)
	requestID := c.Locals("request_id").(string)

	var input domain.CreateMessageInput
	if err := c.BodyParser(&input); err != nil {
		return BadRequest(c, err.Error())
	}
	input.TenantID = tenantID

	msg, err := h.msgService.Create(c.Context(), input, userID, requestID, c.IP())
	if err != nil {
		return InternalError(c, err.Error())
	}

	return Created(c, msg)
}

// GetMessage retrieves a message by ID.
// GET /api/v1/messages/:id
func (h *MessageHandler) GetMessage(c *fiber.Ctx) error {
	id := c.Params("id")

	msg, err := h.msgService.GetByID(c.Context(), id)
	if err != nil {
		return NotFound(c, "message not found")
	}

	return Success(c, msg)
}

// ListMessages lists messages for a tenant.
// GET /api/v1/messages?page=1&per_page=20&status=queued
func (h *MessageHandler) ListMessages(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id").(string)

	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("per_page", 20)
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	filter := domain.MessageFilter{
		TenantID: tenantID,
		Page: domain.Page{
			Limit:  perPage,
			Offset: offset,
		},
	}

	if status := c.Query("status"); status != "" {
		s := domain.MessageStatus(status)
		filter.Status = &s
	}

	messages, err := h.msgService.List(c.Context(), filter)
	if err != nil {
		return InternalError(c, err.Error())
	}

	total, err := h.msgService.Count(c.Context(), filter)
	if err != nil {
		return InternalError(c, err.Error())
	}

	return SuccessPaginated(c, messages, total, "messages")
}
