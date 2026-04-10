package storage

import (
	"database/sql"
	"fmt"
	"log"
	"os"
)

type Storage struct {
	DB *sql.DB
}

func NewStorage() *Storage {
	var db *sql.DB

	db, err := sql.Open("sqlite", "./data.db")
	if err != nil {
		log.Fatal(err.Error())
	}
	defer db.Close()

	db.SetConnMaxLifetime(0)
	db.SetMaxIdleConns(5)
	db.SetMaxOpenConns(10)

	pingErr := db.Ping()
	if pingErr != nil {
		log.Fatal(pingErr)
	}

	fmt.Println("Connected to database.")

	storage := &Storage{DB: db}

	initSchema(storage)

	return storage
}

func initSchema(s *Storage) {
	schema, err := os.ReadFile("schema.sql")
	if err != nil {
		log.Fatal("No schema.sql found.")
	}

	_, err = s.DB.Exec(string(schema))
	if err != nil {
		log.Fatal(err.Error())
	}

	fmt.Println("Database schema.sql executed.")
}
