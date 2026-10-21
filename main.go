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
	"eigenshare/internal/share"
	"eigenshare/internal/user"
	"eigenshare/internal/web"
)

func main() {
	cfg := config.Load()

	db := database.NewDatabase(cfg.DataPath)
	db.Migrate()

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
		log.Printf("failed to seed admin: %v\n", err)
	}

	contr := web.NewController(userSvc, shareSvc, cfg.Secret, cfg.SecureCookies)
	pageLimiter := web.NewRateLimiter(cfg.RateLimit, cfg.BurstLimit, 3*time.Minute, cfg.TrustProxy)
	apiLimiter := web.NewRateLimiter(cfg.APIRateLimit, cfg.APIBurstLimit, 3*time.Minute, cfg.TrustProxy)

	doneChan := make(chan struct{}, 1)
	pageLimiter.StartCleanup(doneChan)
	apiLimiter.StartCleanup(doneChan)

	mux := http.NewServeMux()
	mux.Handle("/api/", apiLimiter.Limit(contr.APIRoutes()))
	mux.Handle("/", pageLimiter.Limit(contr.PageRoutes()))

	server := http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: web.Logger(mux),
	}

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		fmt.Println("shutting down...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		close(doneChan)

		server.Shutdown(shutdownCtx)
	}()

	log.Printf("server listening on port %v\n", cfg.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Printf("server error: %v\n", err)
	}
}
