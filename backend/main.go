package main

import (
	"eigenshare/backend/middleware"
	"eigenshare/backend/share"
	"eigenshare/backend/storage"
	"fmt"
	"net/http"
	"strconv"

	_ "modernc.org/sqlite"
)

const (
	PORT       int = 8080
	CHUNK_SIZE     = 1024 * 1024 * 5
)

func main() {
	router := http.NewServeMux()

	databaseStorage := storage.NewDatabaseStorage()
	localStorage := storage.NewLocalStorage("data/tmp", int64(CHUNK_SIZE))

	shareRepo := share.NewRepository(databaseStorage)
	uploadManager := share.NewUploadManager()
	shareServ := share.NewService(shareRepo, localStorage, uploadManager)
	shareCont := share.NewController(shareServ)
	shareCont.RegisterHandlers(router)

	server := http.Server{
		Addr:    ":" + strconv.Itoa(PORT),
		Handler: middleware.Logging(router),
	}

	fmt.Printf("Server listening on port %v\n", PORT)
	server.ListenAndServe()
}
