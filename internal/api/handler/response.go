package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// APIResponse is the standard API response wrapper.
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Total   *int64      `json:"total,omitempty"`
	Next    string      `json:"next,omitempty"`
}

// Success sends a success response.
func Success(c *fiber.Ctx, data interface{}) error {
	return c.JSON(APIResponse{
		Success: true,
		Data:    data,
	})
}

// SuccessWithTotal sends a success response with total count.
func SuccessWithTotal(c *fiber.Ctx, data interface{}, total int64) error {
	return c.JSON(APIResponse{
		Success: true,
		Data:    data,
		Total:   &total,
	})
}

// SuccessPaginated sends a paginated success response.
func SuccessPaginated(c *fiber.Ctx, data interface{}, total int64, next string) error {
	resp := APIResponse{
		Success: true,
		Data:    data,
		Total:   &total,
	}
	if next != "" {
		resp.Next = next
	}
	return c.JSON(resp)
}

// Error sends an error response with the appropriate HTTP status code.
func Error(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError

	switch {
	case errors.Is(err, domain.ErrNotFound):
		code = fiber.StatusNotFound
	case errors.Is(err, domain.ErrConflict):
		code = fiber.StatusConflict
	case errors.Is(err, domain.ErrDuplicate):
		code = fiber.StatusConflict
	case errors.Is(err, domain.ErrUnauthorized):
		code = fiber.StatusUnauthorized
	case errors.Is(err, domain.ErrForbidden):
		code = fiber.StatusForbidden
	case errors.Is(err, domain.ErrInvalidInput):
		code = fiber.StatusBadRequest
	case errors.Is(err, domain.ErrExpired):
		code = fiber.StatusUnauthorized
	case errors.Is(err, domain.ErrSuspended):
		code = fiber.StatusForbidden
	case errors.Is(err, domain.ErrRateLimited):
		code = fiber.StatusTooManyRequests
	}

	return c.Status(code).JSON(APIResponse{
		Success: false,
		Error:   err.Error(),
	})
}

// ValidationErrors sends a validation error response.
func ValidationErrors(c *fiber.Ctx, errs domain.ValidationErrors) error {
	return c.Status(fiber.StatusBadRequest).JSON(APIResponse{
		Success: false,
		Error:   errs.Error(),
	})
}

// Created sends a 201 created response.
func Created(c *fiber.Ctx, data interface{}) error {
	return c.Status(fiber.StatusCreated).JSON(APIResponse{
		Success: true,
		Data:    data,
	})
}

// NoContent sends a 204 no content response.
func NoContent(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNoContent)
}

// BadRequest sends a 400 bad request error.
func BadRequest(c *fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusBadRequest).JSON(APIResponse{
		Success: false,
		Error:   msg,
	})
}

// NotFound sends a 404 not found error.
func NotFound(c *fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusNotFound).JSON(APIResponse{
		Success: false,
		Error:   msg,
	})
}

// InternalError sends a 500 internal server error.
func InternalError(c *fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusInternalServerError).JSON(APIResponse{
		Success: false,
		Error:   msg,
	})
}
