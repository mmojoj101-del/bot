package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/service"
)

// APIKeyHandler handles API key management endpoints.
type APIKeyHandler struct {
	apiKeyService *service.APIKeyService
}

// NewAPIKeyHandler creates a new API key handler.
func NewAPIKeyHandler(apiKeyService *service.APIKeyService) *APIKeyHandler {
	return &APIKeyHandler{apiKeyService: apiKeyService}
}

// Create creates a new API key.
func (h *APIKeyHandler) Create(c *fiber.Ctx) error {
	var req domain.CreateAPIKeyInput
	if err := c.BodyParser(&req); err != nil {
		return Error(c, err)
	}

	// Extract tenant from context
	tenantID := c.Locals("tenant_id").(string)
	req.TenantID = tenantID

	userID := c.Locals("user_id").(string)
	rid := c.Locals("request_id").(string)

	result, err := h.apiKeyService.Create(c.Context(), req, userID, rid, c.IP())
	if err != nil {
		return Error(c, err)
	}

	return Created(c, result)
}

// GetByID retrieves an API key by ID.
func (h *APIKeyHandler) GetByID(c *fiber.Ctx) error {
	id := c.Params("id")
	key, err := h.apiKeyService.GetByID(c.Context(), id)
	if err != nil {
		return Error(c, err)
	}
	return Success(c, key)
}

// Update updates an API key.
func (h *APIKeyHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var req domain.UpdateAPIKeyInput
	if err := c.BodyParser(&req); err != nil {
		return Error(c, err)
	}

	userID := c.Locals("user_id").(string)
	rid := c.Locals("request_id").(string)

	key, err := h.apiKeyService.Update(c.Context(), id, req, userID, rid, c.IP())
	if err != nil {
		return Error(c, err)
	}

	return Success(c, key)
}

// Delete soft-deletes an API key.
func (h *APIKeyHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	userID := c.Locals("user_id").(string)
	rid := c.Locals("request_id").(string)

	if err := h.apiKeyService.Delete(c.Context(), id, userID, rid, c.IP()); err != nil {
		return Error(c, err)
	}

	return NoContent(c)
}

// ListByTenant lists API keys for the current tenant.
func (h *APIKeyHandler) ListByTenant(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id").(string)
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	if limit < 1 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	result, err := h.apiKeyService.ListByTenant(c.Context(), tenantID, domain.Page{Limit: limit, Offset: offset})
	if err != nil {
		return Error(c, err)
	}

	return SuccessWithTotal(c, result.Items, result.Total)
}
