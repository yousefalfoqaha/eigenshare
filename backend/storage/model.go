package storage

import "database/sql"

type DatabaseStorage struct {
	DB *sql.DB
}
