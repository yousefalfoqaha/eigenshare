package infra

import (
	"database/sql"
	_ "embed"
	"log"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

type Database struct {
	DB *sql.DB
}

func NewDatabase(dbPath string) *Database {
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatal(err.Error())
	}

	sqlDB.SetConnMaxLifetime(0)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetMaxOpenConns(10)

	if err := sqlDB.Ping(); err != nil {
		log.Fatal(err)
	}

	if _, err := sqlDB.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;"); err != nil {
		log.Fatal(err)
	}

	db := &Database{DB: sqlDB}
	initSchema(db)

	return db
}

func initSchema(s *Database) {
	if _, err := s.DB.Exec(schemaSQL); err != nil {
		log.Fatal(err.Error())
	}

	log.Println("Initialized database schema")
}
