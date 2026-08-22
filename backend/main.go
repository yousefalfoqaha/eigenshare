package main

import (
	"context"
	"eigenshare/backend/auth"
	"eigenshare/backend/infra"
	"eigenshare/backend/middleware"
	"eigenshare/backend/share"
	"eigenshare/backend/user"
	"eigenshare/backend/web"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

const (
	PORT      int = 8080
	DATA_PATH     = "data"
)

func main() {
	db := infra.NewDatabase("./data.db")

	userRepo := user.NewUserRepository(db)
	sessionRepo := user.NewSessionRepository(db)
	shareRepo := share.NewShareRepository(db)

	fileStorage := share.NewFileStorage(DATA_PATH)
	shareService := share.NewShareService(shareRepo, fileStorage)
	authService := auth.NewAuthService(userRepo, sessionRepo)

	if err := authService.SeedAdmin(); err != nil {
		fmt.Printf("Failed to seed admin: %v\n", err)
	}

	webController := web.NewController(authService, shareService)

	protectedMux := http.NewServeMux()
	webController.RegisterProtectedHandlers(protectedMux)

	router := http.NewServeMux()
	router.Handle("/", auth.RequireAuth(authService, protectedMux))
	webController.RegisterHandlers(router)

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
		go share.NewExpiryJob(shutdownCtx, shareService, 1*time.Minute)

		server.Shutdown(shutdownCtx)
	}()

	fmt.Printf("Server listening on port %v\n", PORT)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Printf("Server error: %v\n", err)
	}
}
