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
	"github.com/iqbal-hamdani/go-backend/internal/gateway"
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
	billRepo := repository.NewBillRepository(pool)
	roomSvc := service.NewRoomService(roomRepo, billRepo)
	roomHandler := handler.NewRoomHandler(roomSvc, tokenManager)

	// Tenant management module.
	tenantRepo := repository.NewTenantRepository(pool)
	tenantSvc := service.NewTenantService(tenantRepo, billRepo)
	tenantHandler := handler.NewTenantHandler(tenantSvc, tokenManager)

	// Tenant onboarding module.
	onboardingRepo := repository.NewOnboardingRepository(pool)
	onboardingSvc := service.NewOnboardingService(onboardingRepo)
	onboardingHandler := handler.NewOnboardingHandler(onboardingSvc, tokenManager)

	// Monthly billing module.
	billSvc := service.NewBillService(billRepo)
	billHandler := handler.NewBillHandler(billSvc, tokenManager)

	// Payment tracking module.
	paymentRepo := repository.NewPaymentRepository(pool)
	paymentSvc := service.NewPaymentService(paymentRepo)
	paymentHandler := handler.NewPaymentHandler(paymentSvc, tokenManager)

	// Dashboard module.
	dashboardRepo := repository.NewDashboardRepository(pool)
	dashboardSvc := service.NewDashboardService(dashboardRepo)
	dashboardHandler := handler.NewDashboardHandler(dashboardSvc, tokenManager)

	// Tenant portal module. The MVP uses a self-contained sandbox gateway
	// provider so the Pay Now flow works without external credentials.
	gatewayProvider := gateway.NewSandboxProvider(
		cfg.PaymentGatewayProvider,
		cfg.PaymentGatewayCheckoutBaseURL,
		time.Duration(cfg.PaymentGatewayCheckoutTTLHours)*time.Hour,
	)
	tenantPortalRepo := repository.NewTenantPortalRepository(pool)
	gatewayRepo := repository.NewGatewayRepository(pool)
	tenantPortalSvc := service.NewTenantPortalService(tenantPortalRepo, gatewayRepo, gatewayProvider, cfg.PaymentGatewayReturnURL)
	tenantPortalHandler := handler.NewTenantPortalHandler(tenantPortalSvc, tokenManager)

	router := server.NewRouter(cfg, healthHandler, userHandler, authHandler, roomHandler, tenantHandler, onboardingHandler, billHandler, paymentHandler, dashboardHandler, tenantPortalHandler)

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
