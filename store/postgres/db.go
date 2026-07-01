// GORM pool config
package postgres

import (
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Connect() {
	conn, err := gorm.Open(postgres.Open("host=localhost user=postgres dbname=vesktop port=5432 sslmode=disable"), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	DB = conn

	println("Connected to Postgres database")
}
