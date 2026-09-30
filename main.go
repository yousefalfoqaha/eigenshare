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

	"eigenshare/internal/api"
	"eigenshare/internal/database"
	"eigenshare/internal/middleware"
	"eigenshare/internal/page"
	"eigenshare/internal/security"
	"eigenshare/internal/session"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

const (
	PORT                      int    = 8080
	DATA_PATH                 string = "data"
	SECURE_COOKIE             bool   = false
	SESSION_DURATION_SEC      int    = 60 * 24 * 7
	SHARE_UNLOCK_DURATION_SEC int    = 60 * 24 * 7
)

func main() {
	db := database.NewDatabase("./data.db")
	userRepo := user.NewUserRepository(db)
	userSvc := user.NewUserService(userRepo)
	sessionRepo := session.NewSessionRepository(db)
	shareRepo := share.NewShareRepository(db)
	fileStore := share.NewFileStorage(DATA_PATH)
	shareSvc := share.NewShareService(shareRepo, fileStore, userSvc)
	sessionSvc := session.NewSessionService(userSvc, sessionRepo)

	cleanerCtx, stopCleaner := context.WithCancel(context.Background())
	defer stopCleaner()
	cleaner := share.NewCleaner(shareRepo, fileStore)
	go cleaner.Run(cleanerCtx, 1*time.Minute)

	if err := userSvc.SeedAdmin(); err != nil {
		log.Printf("Failed to seed admin: %v\n", err)
	}

	unlockSecret, err := security.RandomBytes(32)
	if err != nil {
		log.Fatalf("failed to generate unlock secret: %v", err)
	}

	sessionCookie := security.NewCookieManager("session_id",
		security.WithMaxAge(SESSION_DURATION_SEC),
		security.WithSecure(SECURE_COOKIE),
	)
	shareUnlockCookie := security.NewCookieManager("share_unlock",
		security.WithPath("/s/"),
		security.WithMaxAge(SHARE_UNLOCK_DURATION_SEC),
		security.WithSecure(SECURE_COOKIE),
	)

	apiContr := api.NewController(sessionSvc, userSvc, shareSvc)
	pageContr := page.NewController(sessionSvc, userSvc, shareSvc, sessionCookie, shareUnlockCookie, unlockSecret)

	mux := http.NewServeMux()
	mux.Handle("/", pageContr.Routes())
	mux.Handle("/api/", http.StripPrefix("/api", apiContr.Routes()))

	server := http.Server{
		Addr:    ":" + strconv.Itoa(PORT),
		Handler: middleware.Logger(middleware.WithSession(sessionSvc, sessionCookie)(mux)),
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

	log.Printf("server listening on port %v\n", PORT)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Printf("Server error: %v\n", err)
	}
}
