package main

import (
	"context"
	"errors"
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()

	if err := os.MkdirAll(cfg.DataPath, 0o755); err != nil {
		log.Fatalf("failed to create data directory %s: %v", cfg.DataPath, err)
	}

	db := database.NewDatabase(cfg.DataPath)
	db.Migrate()

	userRepo := user.NewUserRepository(db)
	userSvc := user.NewUserService(userRepo)
	shareRepo := share.NewShareRepository(db)
	fileStore := share.NewFileStorage(cfg.DataPath)
	shareSvc := share.NewShareService(shareRepo, fileStore, userSvc, cfg.Demo)

	shareCleaner := share.NewShareCleaner(shareRepo, fileStore)
	go shareCleaner.Run(ctx, 1*time.Minute)

	if cfg.Demo {
		if err := userSvc.SeedUser("guest", "guest", false); err != nil {
			log.Printf("failed to seed guest user: %v\n", err)
		}
	} else {
		if err := userSvc.SeedUser("admin", "admin", true); err != nil {
			log.Printf("failed to seed admin user: %v\n", err)
		}
	}

	contr := web.NewController(userSvc, shareSvc, cfg.Secret, cfg.SecureCookies, cfg.ChunkSizeMB, cfg.Demo)
	pageLimiter := web.NewRateLimiter(cfg.RateLimit, cfg.BurstLimit, 3*time.Minute, cfg.TrustProxy)
	apiLimiter := web.NewRateLimiter(cfg.APIRateLimit, cfg.APIBurstLimit, 3*time.Minute, cfg.TrustProxy)

	pageLimiter.StartCleanup(ctx.Done())
	apiLimiter.StartCleanup(ctx.Done())

	mux := http.NewServeMux()
	mux.Handle("/api/", apiLimiter.Limit(contr.APIRoutes()))
	mux.Handle("/", pageLimiter.Limit(contr.PageRoutes()))

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: web.Logger(mux),
	}

	go func() {
		log.Printf("server listening on port %v\n", cfg.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v\n", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("server forced to shutdown: %v\n", err)
	}
}
