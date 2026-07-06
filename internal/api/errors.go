// fiber error handler
package api

import (
	"errors"

	"github.com/gofiber/fiber/v3"

	"github.com/vsktop/backend/internal/domain/account"
	"github.com/vsktop/backend/internal/domain/device"
	"github.com/vsktop/backend/internal/domain/guild"
	"github.com/vsktop/backend/internal/domain/prekey"
)

type errorResponse struct {
	Error string `json:"error"`
}

func ErrorHandler(c fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	msg := "internal server error"

	var fe *fiber.Error
	if errors.As(err, &fe) {
		code = fe.Code
		msg = fe.Message
	} else {
		switch {
		case errors.Is(err, account.ErrNotFound),
			errors.Is(err, device.ErrNotFound),
			errors.Is(err, guild.ErrNotFound),
			errors.Is(err, prekey.ErrNotFound):
			code = fiber.StatusNotFound
			msg = "not found"
		case errors.Is(err, account.ErrAlreadyExists),
			errors.Is(err, device.ErrAlreadyExists):
			code = fiber.StatusConflict
			msg = "already exists"
		case errors.Is(err, account.ErrSuspended):
			code = fiber.StatusForbidden
			msg = "account suspended"
		case errors.Is(err, device.ErrLimitExceeded):
			code = fiber.StatusUnprocessableEntity
			msg = "device limit reached"
		case errors.Is(err, prekey.ErrOTKExhausted):
			code = fiber.StatusGone
			msg = "one-time prekey pool exhausted"
		}
	}

	return c.Status(code).JSON(errorResponse{Error: msg})
}
