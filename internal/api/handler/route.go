package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/service"
)

// RouteHandler handles route management endpoints.
type RouteHandler struct {
	routeService *service.RouteService
}

// NewRouteHandler creates a new route handler.
func NewRouteHandler(routeService *service.RouteService) *RouteHandler {
	return &RouteHandler{routeService: routeService}
}

// Create creates a new route.
func (h *RouteHandler) Create(c *fiber.Ctx) error {
	var req domain.CreateRouteInput
	if err := c.BodyParser(&req); err != nil {
		return Error(c, err)
	}

	tenantID := c.Locals("tenant_id").(string)
	req.TenantID = tenantID

	userID := c.Locals("user_id").(string)
	rid := c.Locals("request_id").(string)

	route, err := h.routeService.Create(c.Context(), req, userID, rid, c.IP())
	if err != nil {
		return Error(c, err)
	}

	return Created(c, route)
}

// GetByID retrieves a route by ID.
func (h *RouteHandler) GetByID(c *fiber.Ctx) error {
	id := c.Params("id")
	route, err := h.routeService.GetByID(c.Context(), id)
	if err != nil {
		return Error(c, err)
	}
	return Success(c, route)
}

// Update updates a route.
func (h *RouteHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var req domain.UpdateRouteInput
	if err := c.BodyParser(&req); err != nil {
		return Error(c, err)
	}

	userID := c.Locals("user_id").(string)
	rid := c.Locals("request_id").(string)

	route, err := h.routeService.Update(c.Context(), id, req, userID, rid, c.IP())
	if err != nil {
		return Error(c, err)
	}

	return Success(c, route)
}

// Delete soft-deletes a route.
func (h *RouteHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	userID := c.Locals("user_id").(string)
	rid := c.Locals("request_id").(string)

	if err := h.routeService.Delete(c.Context(), id, userID, rid, c.IP()); err != nil {
		return Error(c, err)
	}

	return NoContent(c)
}

// ListByTenant lists routes for the current tenant with optional filtering.
func (h *RouteHandler) ListByTenant(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id").(string)
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	if limit < 1 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var typeFilter *domain.RouteType
	if t := c.Query("type"); t != "" {
		rt := domain.RouteType(t)
		typeFilter = &rt
	}

	var strategyFilter *domain.RouteStrategy
	if s := c.Query("strategy"); s != "" {
		rs := domain.RouteStrategy(s)
		strategyFilter = &rs
	}

	filter := domain.RouteFilter{
		TenantID:    tenantID,
		Type:        typeFilter,
		Strategy:    strategyFilter,
		Prefix:      c.Query("prefix"),
		ConnectorID: c.Query("connector_id"),
		Search:      c.Query("search"),
		Page:        domain.Page{Limit: limit, Offset: offset},
	}

	result, err := h.routeService.ListByTenant(c.Context(), filter)
	if err != nil {
		return Error(c, err)
	}

	return SuccessWithTotal(c, result.Items, result.Total)
}
