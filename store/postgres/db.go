package postgres

import (
	"fmt"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Connect() (*gorm.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, fmt.Errorf("postgres: DATABASE_URL is not set")
	}
	return gorm.Open(postgres.Open(dsn), &gorm.Config{})
}
