package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
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
	Size int    `json:"size"`
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

	log.Fatal(http.ListenAndServe(":8080", mux))
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	reader, err := r.MultipartReader()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var upload File

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		file, err := os.Create("data/" + part.FileName())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer file.Close()

		size, err := io.Copy(file, part)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// check for unique naming -> add filname (n), n being number of same-name files (regex of %filename% (*))

		result, err := db.Exec("INSERT INTO file (name, size) VALUES (?, ?)", part.FileName(), size)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		id, err := result.LastInsertId()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		upload = File{
			ID:   int(id),
			Name: part.FileName(),
			Size: int(size),
		}

		part.Close()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(upload)
}

func filesHandler(w http.ResponseWriter, r *http.Request) {
	files, err := getFiles()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if files == nil {
		files = []File{}
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(files)
}

func uploadFile() {
}

func getFiles() ([]File, error) {
	var files []File

	rows, err := db.Query("SELECT id, name, size FROM file")
	if err != nil {
		return nil, fmt.Errorf("Failed to query database for all files.")
	}

	for rows.Next() {
		var f File
		if err := rows.Scan(&f.ID, &f.Name, &f.Size); err != nil {
			return nil, fmt.Errorf("Failed to read rows.")
		}
		files = append(files, f)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%v", err)
	}

	return files, nil
}
