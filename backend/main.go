package main

import (
	"context"
	"eigenshare/backend/handler"
	"eigenshare/backend/middleware"
	"eigenshare/backend/repository"
	"eigenshare/backend/service"
	"eigenshare/backend/storage"
	"eigenshare/backend/upload"
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

	DATA_PATH     = "data"
	DATA_TMP_PATH = DATA_PATH + "/tmp"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	router := http.NewServeMux()

	databaseStorage := storage.NewDatabaseStorage()
	fileStorage := storage.NewFileStorage(DATA_PATH, DATA_TMP_PATH, CHUNK_SIZE)

	shareRepo := repository.NewShareRepository(databaseStorage)
	uploadTracker := upload.NewUploadTracker(CHUNK_SIZE)
	shareWorker := service.NewUploadWorker(1*time.Minute, 5*time.Minute, shareRepo, uploadTracker, fileStorage)
	shareService := service.NewShareService(shareRepo, fileStorage, uploadTracker, shareWorker)
	shareController := handler.NewShareController(shareService)
	shareController.RegisterHandlers(router)

	go shareWorker.Run(ctx)

	server := http.Server{
		Addr:    ":" + strconv.Itoa(PORT),
		Handler: middleware.Logging(router),
	}

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		fmt.Println("Shutting down...")
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		server.Shutdown(shutdownCtx)
	}()

	fmt.Printf("Server listening on port %v\n", PORT)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Printf("Server error: %v\n", err)
	}
}
