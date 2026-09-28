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
	"eigenshare/internal/auth"
	"eigenshare/internal/infra"
	"eigenshare/internal/middleware"
	"eigenshare/internal/pages"
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

	apiSvc := api.New(authService, userSvc, shareService)
	pageSvc := pages.New(authService, userSvc, shareService)

	server := http.Server{
		Addr:    ":" + strconv.Itoa(PORT),
		Handler: middleware.Logging(routes(apiSvc, pageSvc)),
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

func routes(apiSvc *api.Controller, pageSvc *pages.Controller) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /static/", pageSvc.StaticHandler())

	// Public page routes.
	pub := http.NewServeMux()
	pub.Handle("GET /login", pageSvc.Wrap(pageSvc.LoginPage))
	pub.Handle("POST /login", pageSvc.Wrap(pageSvc.LoginSubmit))
	pub.Handle("POST /logout", pageSvc.Wrap(pageSvc.Logout))
	pub.Handle("GET /s/{handle}", pageSvc.Wrap(pageSvc.SharePage))
	pub.Handle("GET /s/{handle}/download", pageSvc.Wrap(pageSvc.DownloadAll))
	pub.Handle("GET /s/{handle}/files/{fileID}/download", pageSvc.Wrap(pageSvc.DownloadFile))
	mux.Handle("/login", pageSvc.OptionalAuth(pub))
	mux.Handle("/logout", pageSvc.OptionalAuth(pub))
	mux.Handle("/s/", pageSvc.OptionalAuth(pub))

	// Protected page routes.
	prot := http.NewServeMux()
	prot.Handle("GET /", pageSvc.Wrap(pageSvc.Index))
	prot.Handle("GET /shares", pageSvc.Wrap(pageSvc.SharesPage))
	prot.Handle("GET /shares/new", pageSvc.Wrap(pageSvc.NewSharePage))
	prot.Handle("GET /shares/{id}/edit", pageSvc.Wrap(pageSvc.EditSharePage))
	prot.Handle("POST /shares/delete", pageSvc.Wrap(pageSvc.DeleteShares))
	prot.Handle("POST /shares/{id}/trash", pageSvc.Wrap(pageSvc.TrashShare))
	prot.Handle("POST /shares/{id}/restore", pageSvc.Wrap(pageSvc.RestoreShare))
	prot.Handle("POST /shares/{id}/visibility", pageSvc.Wrap(pageSvc.SetVisibility))
	prot.Handle("POST /shares/{id}/owners", pageSvc.Wrap(pageSvc.AddOwner))
	prot.Handle("POST /shares/{id}/owners/{ownerID}/delete", pageSvc.Wrap(pageSvc.RemoveOwner))
	prot.Handle("POST /shares/{id}/files/{fileID}/delete", pageSvc.Wrap(pageSvc.DeleteFile))
	prot.Handle("GET /trash", pageSvc.Wrap(pageSvc.TrashPage))
	prot.Handle("GET /users", pageSvc.Wrap(pageSvc.UsersPage))
	prot.Handle("GET /users/new", pageSvc.Admin(pageSvc.Wrap(pageSvc.NewUserPage)))
	prot.Handle("POST /users", pageSvc.Admin(pageSvc.Wrap(pageSvc.CreateUser)))
	prot.Handle("GET /users/{id}/edit", pageSvc.Admin(pageSvc.Wrap(pageSvc.EditUserPage)))
	prot.Handle("POST /users/{id}", pageSvc.Admin(pageSvc.Wrap(pageSvc.UpdateUser)))
	prot.Handle("POST /users/{id}/delete", pageSvc.Admin(pageSvc.Wrap(pageSvc.DeleteUser)))
	mux.Handle("/", pageSvc.RequireLogin(prot))

	// API routes.
	apiMux := http.NewServeMux()
	apiMux.Handle("POST /shares", apiSvc.Wrap(apiSvc.CreateShare))
	apiMux.Handle("POST /shares/{id}/files", apiSvc.Wrap(apiSvc.AddFilesToShare))
	apiMux.Handle("HEAD /shares/{id}/files/{fileID}/stream", apiSvc.Wrap(apiSvc.GetUploadOffset))
	apiMux.Handle("PUT /shares/{id}/files/{fileID}/stream", apiSvc.Wrap(apiSvc.StreamFile))
	apiMux.Handle("DELETE /shares/{id}/files/{fileID}", apiSvc.Wrap(apiSvc.DeleteShareFile))
	mux.Handle("/api/", http.StripPrefix("/api", apiSvc.RequireLogin(apiMux)))

	return mux
}
