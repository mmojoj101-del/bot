package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/service"
)

// ConnectorHandler handles connector management endpoints.
type ConnectorHandler struct {
	connectorService *service.ConnectorService
}

// NewConnectorHandler creates a new connector handler.
func NewConnectorHandler(connectorService *service.ConnectorService) *ConnectorHandler {
	return &ConnectorHandler{connectorService: connectorService}
}

// Create creates a new connector.
func (h *ConnectorHandler) Create(c *fiber.Ctx) error {
	var req domain.CreateConnectorInput
	if err := c.BodyParser(&req); err != nil {
		return Error(c, err)
	}

	tenantID := c.Locals("tenant_id").(string)
	req.TenantID = tenantID

	userID := c.Locals("user_id").(string)
	rid := c.Locals("request_id").(string)

	connector, err := h.connectorService.Create(c.Context(), req, userID, rid, c.IP())
	if err != nil {
		return Error(c, err)
	}

	return Created(c, connector)
}

// GetByID retrieves a connector by ID.
func (h *ConnectorHandler) GetByID(c *fiber.Ctx) error {
	id := c.Params("id")
	connector, err := h.connectorService.GetByID(c.Context(), id)
	if err != nil {
		return Error(c, err)
	}
	return Success(c, connector)
}

// Update updates a connector.
func (h *ConnectorHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var req domain.UpdateConnectorInput
	if err := c.BodyParser(&req); err != nil {
		return Error(c, err)
	}

	userID := c.Locals("user_id").(string)
	rid := c.Locals("request_id").(string)

	connector, err := h.connectorService.Update(c.Context(), id, req, userID, rid, c.IP())
	if err != nil {
		return Error(c, err)
	}

	return Success(c, connector)
}

// Delete soft-deletes a connector.
func (h *ConnectorHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	userID := c.Locals("user_id").(string)
	rid := c.Locals("request_id").(string)

	if err := h.connectorService.Delete(c.Context(), id, userID, rid, c.IP()); err != nil {
		return Error(c, err)
	}

	return NoContent(c)
}

// ListByTenant lists connectors for the current tenant with optional filtering.
func (h *ConnectorHandler) ListByTenant(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id").(string)
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	if limit < 1 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var typeFilter *domain.ConnectorType
	if t := c.Query("type"); t != "" {
		ct := domain.ConnectorType(t)
		typeFilter = &ct
	}

	var statusFilter *domain.ConnectorStatus
	if s := c.Query("status"); s != "" {
		cs := domain.ConnectorStatus(s)
		statusFilter = &cs
	}

	filter := domain.ConnectorFilter{
		TenantID: tenantID,
		Type:     typeFilter,
		Status:   statusFilter,
		Search:   c.Query("search"),
		Page:     domain.Page{Limit: limit, Offset: offset},
	}

	result, err := h.connectorService.ListByTenant(c.Context(), filter)
	if err != nil {
		return Error(c, err)
	}

	return SuccessWithTotal(c, result.Items, result.Total)
}

// TestConnection tests a connector connection.
func (h *ConnectorHandler) TestConnection(c *fiber.Ctx) error {
	id := c.Params("id")
	rid := c.Locals("request_id").(string)

	if err := h.connectorService.TestConnector(c.Context(), id, rid, c.IP()); err != nil {
		return Error(c, err)
	}

	return Success(c, fiber.Map{"message": "connection test initiated"})
}
