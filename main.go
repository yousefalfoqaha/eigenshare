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

	"eigenshare/internal/app"
	"eigenshare/internal/auth"
	"eigenshare/internal/infra"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
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
	shareService := share.NewShareService(shareRepo, fileStorage, userSvc)
	authService := auth.NewAuthService(userSvc, sessionRepo)
	maintenanceCtx, stopMaintenance := context.WithCancel(context.Background())
	defer stopMaintenance()
	cleaner := share.NewCleaner(shareRepo, fileStorage)
	go cleaner.Run(maintenanceCtx, 1*time.Minute)

	if err := userSvc.SeedAdmin(); err != nil {
		log.Printf("Failed to seed admin: %v\n", err)
	}

	app := app.New(authService, userSvc, shareService)

	server := http.Server{
		Addr:    ":" + strconv.Itoa(PORT),
		Handler: app.Routes(),
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
