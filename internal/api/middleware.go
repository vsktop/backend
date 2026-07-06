// auth, rate limit, request ID
package api

import (
	"strings"

	"github.com/gofiber/fiber/v3"

	rstore "github.com/vsktop/backend/store/redis"
)

func AuthMiddleware(sessions *rstore.SessionStore) fiber.Handler {
	return func(c fiber.Ctx) error {
		raw := c.Get("Authorization")
		token, found := strings.CutPrefix(raw, "Bearer ")
		if !found || token == "" {
			return fiber.ErrUnauthorized
		}

		sess, err := sessions.Get(c.Context(), token)
		if err != nil {
			return fiber.ErrUnauthorized
		}

		c.Locals("account_id", sess.AccountID)
		c.Locals("device_id", sess.DeviceID)
		c.Locals("session_token", token)
		return c.Next()
	}
}
