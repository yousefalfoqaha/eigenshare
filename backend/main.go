package main

import (
	"log"
	"net/http"
	"strconv"

	"eigenshare/backend/share"
	"eigenshare/backend/storage"

	_ "modernc.org/sqlite"
)

const PORT int = 8080

func main() {
	storage := storage.NewStorage()

	router := http.NewServeMux()

	shareRepo := share.NewRepository(storage)
	shareServ := share.NewService(shareRepo)
	shareCont := share.NewController(shareServ)
	shareCont.RegisterHandlers(router)

	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(PORT), router))
}
