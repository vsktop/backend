package main

import "github.com/gofiber/fiber/v3"

func main() {
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  fiber.StatusOK,
			"message": "Hello world!",
		})

	})
	app.Listen(":4001")
}
