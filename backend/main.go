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
	router := http.NewServeMux()

	storage := storage.NewStorage()

	shareRepo := share.NewRepository(storage)
	shareServ := share.NewService(shareRepo)
	shareCont := share.NewController(shareServ)
	shareCont.RegisterHandlers(router)

	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(PORT), router))
}
