package database

import (
	"cmp"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type migration struct {
	Version   int64
	Name      string
	AppliedAt time.Time
}

func (db *Database) Migrate() {
	_, err := db.DB.Exec(`
		CREATE TABLE IF NOT EXISTS migration (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at DATETIME NOT NULL
		)
	`)
	if err != nil {
		panic(err)
	}

	rows, err := db.DB.Query("SELECT version, name, applied_at FROM migration")
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	applied := make(map[int64]migration)
	for rows.Next() {
		var m migration
		if err := rows.Scan(&m.Version, &m.Name, &m.AppliedAt); err != nil {
			panic(err)
		}
		m.AppliedAt = m.AppliedAt.UTC()
		applied[m.Version] = m
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}

	files, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		panic(err)
	}

	var migrations []migration
	for _, p := range files {
		trimmed := strings.TrimSuffix(strings.TrimPrefix(path.Base(p), "V"), ".sql")
		verStr, name, ok := strings.Cut(trimmed, "_")
		v, err := strconv.ParseInt(verStr, 10, 64)
		if !ok || err != nil {
			panic("invalid migration filename: " + p)
		}
		migrations = append(migrations, migration{
			Version:   v,
			Name:      name,
			AppliedAt: time.Now().UTC(),
		})
	}

	slices.SortFunc(migrations, func(a, b migration) int {
		return cmp.Compare(a.Version, b.Version)
	})

	tx, err := db.DB.Begin()
	if err != nil {
		panic(err)
	}
	defer tx.Rollback()

	hasUnapplied := false
	for i, m := range migrations {
		if i > 0 && m.Version == migrations[i-1].Version {
			panic(fmt.Sprintf("duplicate migration version: %d", m.Version))
		}
		if _, ok := applied[m.Version]; ok {
			if hasUnapplied {
				panic(fmt.Sprintf("migration gap detected at version: %d", m.Version))
			}
			continue
		}

		hasUnapplied = true
		filePath := fmt.Sprintf("migrations/V%03d_%s.sql", m.Version, m.Name)
		sqlBytes, err := migrationsFS.ReadFile(filePath)
		if err != nil {
			panic(err)
		}
		if _, err := tx.Exec(string(sqlBytes)); err != nil {
			panic(fmt.Sprintf("failed %s: %v", filePath, err))
		}
		if _, err := tx.Exec("INSERT INTO migration (version, name, applied_at) VALUES (?, ?, ?)", m.Version, m.Name, m.AppliedAt); err != nil {
			panic(err)
		}

		log.Printf("applied migration V%03d_%s.sql", m.Version, m.Name)
	}

	if err := tx.Commit(); err != nil {
		panic(err)
	}
}
