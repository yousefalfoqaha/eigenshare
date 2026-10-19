package database

import (
	"database/sql"
	_ "embed"
	"log"

	_ "modernc.org/sqlite"
)

type Database struct {
	DB *sql.DB
}

func NewDatabase(dbPath string) *Database {
	sqlDB, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_time_format=sqlite")
	if err != nil {
		log.Fatal(err.Error())
	}

	sqlDB.SetConnMaxLifetime(0)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetMaxOpenConns(10)

	if err := sqlDB.Ping(); err != nil {
		log.Fatal(err)
	}

	return &Database{DB: sqlDB}
}
