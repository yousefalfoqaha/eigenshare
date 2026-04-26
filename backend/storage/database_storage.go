package storage

import (
	"database/sql"
	"fmt"
	"log"
	"os"
)

func NewDatabaseStorage() *DatabaseStorage {
	db, err := sql.Open("sqlite", "./data.db")
	if err != nil {
		log.Fatal(err.Error())
	}

	db.SetConnMaxLifetime(0)
	db.SetMaxIdleConns(5)
	db.SetMaxOpenConns(10)

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;"); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Connected to database")

	storage := &DatabaseStorage{DB: db}
	initSchema(storage)

	return storage
}

func initSchema(s *DatabaseStorage) {
	schema, err := os.ReadFile("schema.sql")
	if err != nil {
		log.Fatal("No schema.sql found.")
	}

	_, err = s.DB.Exec(string(schema))
	if err != nil {
		log.Fatal(err.Error())
	}

	fmt.Println("Initialized database schema")
}
