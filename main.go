package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"eigenshare/internal/auth"
	"eigenshare/internal/infra"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
	"eigenshare/internal/web"
)

const (
	PORT      int = 8080
	DATA_PATH     = "data"
)

func main() {
	db := infra.NewDatabase("./data.db")

	userRepo := user.NewUserRepository(db)
	userSvc := user.NewUserService(userRepo)
	sessionRepo := auth.NewSessionRepository(db)
	shareRepo := share.NewShareRepository(db)
	fileStorage := share.NewFileStorage(DATA_PATH)
	shareService := share.NewShareService(shareRepo, fileStorage)
	authService := auth.NewAuthService(userSvc, sessionRepo)
	maintenanceCtx, stopMaintenance := context.WithCancel(context.Background())
	defer stopMaintenance()
	share.NewCleanupJob(maintenanceCtx, shareService, 1*time.Minute)

	if err := userSvc.SeedAdmin(); err != nil {
		log.Printf("Failed to seed admin: %v\n", err)
	}

	webController := web.NewController(authService, userSvc, shareService)

	protectedMux := http.NewServeMux()
	webController.RegisterProtectedHandlers(protectedMux)

	router := http.NewServeMux()
	router.Handle("/", web.RequireAuth(authService, protectedMux))
	webController.RegisterHandlers(router)

	server := http.Server{
		Addr:    ":" + strconv.Itoa(PORT),
		Handler: web.Logging(router),
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

	log.Printf("Server listening on port %v\n", PORT)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Printf("Server error: %v\n", err)
	}
}
