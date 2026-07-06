// fiber router registration
package api

import (
	"github.com/gofiber/fiber/v3"
	"github.com/vsktop/backend/internal/api/v1/auth"
)

func RegisterV1(app *fiber.App, authH *auth.Handler, authMW fiber.Handler) {
	v1 := app.Group("/v1")
	authH.RegisterRoutes(v1.Group("/auth"), authMW)
}
