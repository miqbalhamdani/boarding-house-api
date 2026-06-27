package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iqbal-hamdani/go-backend/internal/config"
	"github.com/iqbal-hamdani/go-backend/internal/database"
	"github.com/iqbal-hamdani/go-backend/internal/handler"
	"github.com/iqbal-hamdani/go-backend/internal/repository"
	"github.com/iqbal-hamdani/go-backend/internal/server"
	"github.com/iqbal-hamdani/go-backend/internal/service"
	"github.com/iqbal-hamdani/go-backend/pkg/auth"
	"github.com/iqbal-hamdani/go-backend/pkg/logger"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := logger.New(cfg.Env)
	slog.SetDefault(log)

	// Root context cancelled on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Database connection pool.
	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	log.Info("connected to database")

	// Wire dependencies: repository -> service -> handler.
	userRepo := repository.NewUserRepository(pool)
	userSvc := service.NewUserService(userRepo)
	userHandler := handler.NewUserHandler(userSvc)
	healthHandler := handler.NewHealthHandler(pool)

	// Auth module.
	tokenManager := auth.NewManager(cfg.JWTSecret, cfg.JWTAccessTTLMinutes, cfg.JWTRefreshTTLHours)
	ownerRepo := repository.NewOwnerRepository(pool)
	tenantAuthRepo := repository.NewTenantAuthRepository(pool)
	authSvc := service.NewAuthService(ownerRepo, tenantAuthRepo, tokenManager)
	authHandler := handler.NewAuthHandler(authSvc, tokenManager)

	// Room management module.
	roomRepo := repository.NewRoomRepository(pool)
	roomSvc := service.NewRoomService(roomRepo)
	roomHandler := handler.NewRoomHandler(roomSvc, tokenManager)

	router := server.NewRouter(cfg, healthHandler, userHandler, authHandler, roomHandler)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Start server in the background.
	go func() {
		log.Info("server listening", "port", cfg.Port, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "err", err)
			stop()
		}
	}()

	// Block until a shutdown signal arrives.
	<-ctx.Done()
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
