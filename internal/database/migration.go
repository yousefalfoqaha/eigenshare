package database

import (
	"embed"
	// "io/fs"
	// "log"
)

//go:embed migrations
var templateFS embed.FS

const migrationsTableSQL = `
	CREATE TABLE IF NOT EXISTS migrations (
		version INT NOT NULL,
		applied_at DATETIME NOT NULL
	);
`

func (db *Database) Migrate() {
	// if _, err := db.DB.Exec(migrationsTableSQL); err != nil {
	// 	log.Fatal(err.Error())
	// }
	//
	// pageFiles, err := fs.Glob(templateFS, "templates/pages/*.html")
	// if err != nil {
	// 	log.Fatalf("failed to glob embedded templates: %v", err)
	// }
}
