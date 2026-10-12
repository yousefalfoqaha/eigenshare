package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"eigenshare/internal/config"
	"eigenshare/internal/database"
	"eigenshare/internal/security"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
	"eigenshare/internal/web"
)

func main() {
	cfg := config.Load()

	db := database.NewDatabase("./data.db")
	userRepo := user.NewUserRepository(db)
	userSvc := user.NewUserService(userRepo)
	shareRepo := share.NewShareRepository(db)
	fileStore := share.NewFileStorage(cfg.DataPath)
	shareSvc := share.NewShareService(shareRepo, fileStore, userSvc)

	cleanerCtx, stopCleaner := context.WithCancel(context.Background())
	defer stopCleaner()
	shareCleaner := share.NewShareCleaner(shareRepo, fileStore)
	go shareCleaner.Run(cleanerCtx, 1*time.Minute)

	if err := userSvc.SeedAdmin(); err != nil {
		log.Printf("Failed to seed admin: %v\n", err)
	}

	secret, err := security.RandomBytes(32)
	if err != nil {
		log.Fatalf("failed to generate unlock secret: %v", err)
	}

	contr := web.NewController(userSvc, shareSvc, secret, cfg.SecureCookies)

	mux := http.NewServeMux()
	mux.Handle("/", contr.HTMLRoutes())
	mux.Handle("/api/", contr.APIRoutes())

	server := http.Server{
		Addr: fmt.Sprintf(":%d", cfg.Port),
		Handler: web.Logger(
			contr.WithSession(mux),
		),
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

	log.Printf("server listening on port %v\n", cfg.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Printf("Server error: %v\n", err)
	}
}
