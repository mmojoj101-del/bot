package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/service"
)

// AuditHandler handles audit log endpoints.
type AuditHandler struct {
	auditService *service.AuditService
}

// NewAuditHandler creates a new audit handler.
func NewAuditHandler(auditService *service.AuditService) *AuditHandler {
	return &AuditHandler{auditService: auditService}
}

// ListByTenant lists audit logs for the current tenant.
func (h *AuditHandler) ListByTenant(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id").(string)
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	cursor := c.Query("cursor", "")

	if limit < 1 || limit > 100 {
		limit = 20
	}

	result, err := h.auditService.ListByTenant(c.Context(), tenantID, domain.CursorPage{
		Limit:  limit,
		Cursor: domain.Cursor(cursor),
	})
	if err != nil {
		return Error(c, err)
	}

	return SuccessPaginated(c, result.Items, int64(len(result.Items)), string(result.NextCursor))
}

// ListByUser lists audit logs for the current user.
func (h *AuditHandler) ListByUser(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(string)
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	cursor := c.Query("cursor", "")

	if limit < 1 || limit > 100 {
		limit = 20
	}

	result, err := h.auditService.ListByUser(c.Context(), userID, domain.CursorPage{
		Limit:  limit,
		Cursor: domain.Cursor(cursor),
	})
	if err != nil {
		return Error(c, err)
	}

	return SuccessPaginated(c, result.Items, int64(len(result.Items)), string(result.NextCursor))
}
