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

const PORT int = 8080

func main() {
	router := http.NewServeMux()

	storage := storage.NewStorage()

	shareRepo := share.NewRepository(storage)
	shareServ := share.NewService(shareRepo)
	shareCont := share.NewController(shareServ)
	shareCont.RegisterHandlers(router)

	server := http.Server{
		Addr:    ":" + strconv.Itoa(PORT),
		Handler: middleware.Logging(router),
	}

	fmt.Printf("Server listening on port %v\n", PORT)
	server.ListenAndServe()
}
