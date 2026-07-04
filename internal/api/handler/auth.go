package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/raghna/fury-sms-gateway/internal/service"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	authService *service.AuthService
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// Login handles user login.
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req service.LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return Error(c, err)
	}

	rid := c.Locals("request_id").(string)
	resp, err := h.authService.Login(c.Context(), req, rid, c.IP())
	if err != nil {
		return Error(c, err)
	}

	return Success(c, resp)
}

// RefreshToken handles token refresh.
func (h *AuthHandler) RefreshToken(c *fiber.Ctx) error {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.BodyParser(&req); err != nil {
		return Error(c, err)
	}

	if req.RefreshToken == "" {
		return Error(c, fiber.ErrBadRequest)
	}

	rid := c.Locals("request_id").(string)
	resp, err := h.authService.RefreshToken(c.Context(), req.RefreshToken, rid, c.IP())
	if err != nil {
		return Error(c, err)
	}

	return Success(c, resp)
}

// Logout handles user logout.
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.BodyParser(&req); err != nil {
		return Error(c, err)
	}

	rid := c.Locals("request_id").(string)
	if err := h.authService.Logout(c.Context(), req.RefreshToken, rid, c.IP()); err != nil {
		return Error(c, err)
	}

	return Success(c, fiber.Map{"message": "logged out successfully"})
}

// Me returns the current user's profile.
func (h *AuthHandler) Me(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(string)
	user, err := h.authService.GetUser(c.Context(), userID)
	if err != nil {
		return Error(c, err)
	}
	return Success(c, user)
}

// SwitchTenant switches the active tenant.
func (h *AuthHandler) SwitchTenant(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(string)
	tenantID := c.Params("tenantID")

	rid := c.Locals("request_id").(string)
	resp, err := h.authService.SwitchTenant(c.Context(), userID, tenantID, rid, c.IP())
	if err != nil {
		return Error(c, err)
	}

	return Success(c, resp)
}
