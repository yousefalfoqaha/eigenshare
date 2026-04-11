package main

import (
	"eigenshare/backend/share"
	"eigenshare/backend/storage"
	"log"
	"net/http"
	"strconv"

	_ "modernc.org/sqlite"
)

const PORT int = 8080

func main() {
	storage := storage.NewStorage()

	router := http.NewServeMux()

	shareRepo := share.NewRepository(storage)
	shareCont := share.NewController(shareRepo)
	shareCont.RegisterHandlers(router)

	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(PORT), router))
}
