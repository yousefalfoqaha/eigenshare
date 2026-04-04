package main

import (
	"eigenshare/backend/db"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
)

const (
	FILES_ENDPOINT string = "/files/"
	PORT           int    = 8080
)

func handler(w http.ResponseWriter, r *http.Request) {
	title := strings.TrimPrefix(r.URL.Path, FILES_ENDPOINT)

	file, err := db.Load(title)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "<h1>%s not found</h1>", title)
		return
	}

	fmt.Fprintf(w, "<h1>%s</h1><p>%s</p>", file.Title, file.Body)
}

func main() {
	http.HandleFunc(FILES_ENDPOINT, handler)
	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(PORT), nil))
}
