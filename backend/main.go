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
	PORT int = 8080

	CHUNK_SIZE = int64(1024 * 1024 * 5)

	DATA_PATH     = "data"
	DATA_TMP_PATH = DATA_PATH + "/tmp"
)

func main() {
	router := http.NewServeMux()

	databaseStorage := storage.NewDatabaseStorage()
	fileStorage := storage.NewFileStorage(DATA_PATH, DATA_TMP_PATH, CHUNK_SIZE)

	shareRepo := share.NewRepository(databaseStorage)
	uploadManager := share.NewUploadManager(CHUNK_SIZE)
	shareServ := share.NewService(shareRepo, fileStorage, uploadManager)
	shareCont := share.NewController(shareServ)
	shareCont.RegisterHandlers(router)

	server := http.Server{
		Addr:    ":" + strconv.Itoa(PORT),
		Handler: middleware.Logging(router),
	}

	fmt.Printf("Server listening on port %v\n", PORT)
	server.ListenAndServe()
}
