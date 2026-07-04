package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/service"
)

// MemberHandler handles tenant member management endpoints.
type MemberHandler struct {
	memberService *service.MemberService
}

// NewMemberHandler creates a new member handler.
func NewMemberHandler(memberService *service.MemberService) *MemberHandler {
	return &MemberHandler{memberService: memberService}
}

// Add adds a user to a tenant.
func (h *MemberHandler) Add(c *fiber.Ctx) error {
	tenantID := c.Params("tenantID")
	var req struct {
		UserID string            `json:"user_id"`
		Role   domain.MemberRole `json:"role"`
	}
	if err := c.BodyParser(&req); err != nil {
		return Error(c, err)
	}

	userID := c.Locals("user_id").(string)
	rid := c.Locals("request_id").(string)

	member, err := h.memberService.Add(c.Context(), domain.AddMemberInput{
		TenantID: tenantID,
		UserID:   req.UserID,
		Role:     req.Role,
	}, userID, rid, c.IP())
	if err != nil {
		return Error(c, err)
	}

	return Created(c, member)
}

// Remove removes a user from a tenant.
func (h *MemberHandler) Remove(c *fiber.Ctx) error {
	tenantID := c.Params("tenantID")
	userID := c.Params("userID")
	removedBy := c.Locals("user_id").(string)
	rid := c.Locals("request_id").(string)

	if err := h.memberService.Remove(c.Context(), tenantID, userID, removedBy, rid, c.IP()); err != nil {
		return Error(c, err)
	}

	return NoContent(c)
}

// ListByTenant lists members of a tenant.
func (h *MemberHandler) ListByTenant(c *fiber.Ctx) error {
	tenantID := c.Params("tenantID")
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	if limit < 1 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	result, err := h.memberService.ListByTenant(c.Context(), tenantID, domain.Page{Limit: limit, Offset: offset})
	if err != nil {
		return Error(c, err)
	}

	return SuccessWithTotal(c, result.Items, result.Total)
}

// ListByUser lists tenants for the current user.
func (h *MemberHandler) ListByUser(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(string)
	members, err := h.memberService.ListByUser(c.Context(), userID)
	if err != nil {
		return Error(c, err)
	}
	return Success(c, members)
}

// UpdateRole updates a member's role.
func (h *MemberHandler) UpdateRole(c *fiber.Ctx) error {
	tenantID := c.Params("tenantID")
	userID := c.Params("userID")
	var req struct {
		Role domain.MemberRole `json:"role"`
	}
	if err := c.BodyParser(&req); err != nil {
		return Error(c, err)
	}

	rid := c.Locals("request_id").(string)
	if err := h.memberService.UpdateRole(c.Context(), tenantID, userID, req.Role, rid, c.IP()); err != nil {
		return Error(c, err)
	}

	return Success(c, fiber.Map{"message": "role updated"})
}
