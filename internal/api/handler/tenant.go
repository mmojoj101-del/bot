package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/service"
)

// TenantHandler handles tenant management endpoints.
type TenantHandler struct {
	tenantService *service.TenantService
}

// NewTenantHandler creates a new tenant handler.
func NewTenantHandler(tenantService *service.TenantService) *TenantHandler {
	return &TenantHandler{tenantService: tenantService}
}

// Create creates a new tenant.
func (h *TenantHandler) Create(c *fiber.Ctx) error {
	var req domain.CreateTenantInput
	if err := c.BodyParser(&req); err != nil {
		return Error(c, err)
	}

	userID := c.Locals("user_id").(string)
	rid := c.Locals("request_id").(string)
	tenant, err := h.tenantService.Create(c.Context(), req, userID, rid, c.IP())
	if err != nil {
		return Error(c, err)
	}

	return Created(c, tenant)
}

// GetByID retrieves a tenant by ID.
func (h *TenantHandler) GetByID(c *fiber.Ctx) error {
	id := c.Params("id")
	tenant, err := h.tenantService.GetByID(c.Context(), id)
	if err != nil {
		return Error(c, err)
	}
	return Success(c, tenant)
}

// Update updates a tenant.
func (h *TenantHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var req domain.UpdateTenantInput
	if err := c.BodyParser(&req); err != nil {
		return Error(c, err)
	}

	userID := c.Locals("user_id").(string)
	rid := c.Locals("request_id").(string)
	tenant, err := h.tenantService.Update(c.Context(), id, req, userID, rid, c.IP())
	if err != nil {
		return Error(c, err)
	}

	return Success(c, tenant)
}

// Delete soft-deletes a tenant.
func (h *TenantHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	userID := c.Locals("user_id").(string)
	rid := c.Locals("request_id").(string)

	if err := h.tenantService.Delete(c.Context(), id, userID, rid, c.IP()); err != nil {
		return Error(c, err)
	}

	return NoContent(c)
}

// List returns a paginated list of tenants.
func (h *TenantHandler) List(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	if limit < 1 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	result, err := h.tenantService.List(c.Context(), domain.Page{Limit: limit, Offset: offset})
	if err != nil {
		return Error(c, err)
	}

	return SuccessWithTotal(c, result.Items, result.Total)
}
