package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"eigenshare/backend/share"
	"eigenshare/backend/storage"

	"github.com/google/uuid"

	_ "modernc.org/sqlite"
)

const PORT int = 8080

func main() {
	storage := storage.NewStorage()
	router := http.NewServeMux()

	shareRepo := share.NewRepository(storage)
	shareCont := share.NewController(shareRepo)
	shareCont.RegisterHandlers(router)

	log.Fatal(http.ListenAndServe(":"+string(PORT), router))
}

func removeFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	os.Remove("data/" + id)
	_, err := db.Exec("DELETE FROM file WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
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

		id := uuid.New()
		file, err := os.Create("data/" + id.String())
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

		_, err = db.Exec("INSERT INTO file (id, name, size) VALUES (?, ?, ?)", id.String(), part.FileName(), size)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		upload = File{
			ID:   id.String(),
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
