package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	_ "modernc.org/sqlite"
)

const (
	FILES_ENDPOINT string = "/files"
	PORT           int    = 8080
)

type File struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

var db *sql.DB

func main() {
	var err error
	db, err = sql.Open("sqlite", "./data.db")
	if err != nil {
		log.Fatal(err.Error())
	}
	defer db.Close()

	db.SetConnMaxLifetime(0)
	db.SetMaxIdleConns(3)
	db.SetMaxOpenConns(3)

	pingErr := db.Ping()
	if pingErr != nil {
		log.Fatal(pingErr)
	}

	fmt.Println("Connected to database.")

	schema, err := os.ReadFile("schema.sql")
	if err != nil {
		log.Fatal("No schema.sql found.")
	}

	_, err = db.Exec(string(schema))
	if err != nil {
		log.Fatal(err.Error())
	}

	fmt.Println("Database schema.sql executed.")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /files", filesHandler)
	mux.HandleFunc("POST /files", uploadHandler)
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	x, err := r.MultipartReader()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprint(w, x.ReadForm())
}

func filesHandler(w http.ResponseWriter, r *http.Request) {
	files, err := getFiles()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	res, err := json.Marshal(files)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprint(w, string(res))
}

func uploadFile() {
}

func getFiles() ([]File, error) {
	var files []File

	rows, err := db.Query("SELECT id, name FROM files")
	if err != nil {
		return nil, fmt.Errorf("Failed to query database for all files.")
	}

	for rows.Next() {
		var f File
		if err := rows.Scan(&f.ID, &f.Name); err != nil {
			return nil, fmt.Errorf("Failed to read rows.")
		}
		files = append(files, f)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%v", err)
	}

	return files, nil
}
