package main

import (
	"context"
	"eigenshare/backend/handler"
	"eigenshare/backend/middleware"
	"eigenshare/backend/repository"
	"eigenshare/backend/service"
	"eigenshare/backend/storage"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

const (
	PORT int = 8080

	CHUNK_SIZE = int64(1024 * 1024 * 5)

	DATA_PATH = "data"
)

func main() {
	router := http.NewServeMux()

	databaseStorage := storage.NewDatabaseStorage()
	fileStorage := storage.NewFileStorage(DATA_PATH, CHUNK_SIZE)

	shareRepo := repository.NewShareRepository(databaseStorage)

	shareService := service.NewShareService(shareRepo, fileStorage)

	shareController := handler.NewShareController(shareService)
	shareController.RegisterHandlers(router)

	server := http.Server{
		Addr:    ":" + strconv.Itoa(PORT),
		Handler: middleware.Logging(router),
	}

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		fmt.Println("Shutting down...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		server.Shutdown(shutdownCtx)
	}()

	fmt.Printf("Server listening on port %v\n", PORT)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Printf("Server error: %v\n", err)
	}
}
