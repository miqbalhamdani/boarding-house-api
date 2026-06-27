package server

import (
	"github.com/gin-gonic/gin"

	"github.com/iqbal-hamdani/go-backend/internal/config"
	"github.com/iqbal-hamdani/go-backend/internal/handler"
	"github.com/iqbal-hamdani/go-backend/internal/server/middleware"
)

// NewRouter builds the Gin engine with middleware and routes registered.
func NewRouter(cfg *config.Config, health *handler.HealthHandler, user *handler.UserHandler, authH *handler.AuthHandler, roomH *handler.RoomHandler, tenantH *handler.TenantHandler, onboardingH *handler.OnboardingHandler) *gin.Engine {
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(middleware.RequestLogger(), gin.Recovery())

	// Health/probe endpoints (unversioned).
	r.GET("/healthz", health.Live)
	r.GET("/readyz", health.Ready)

	// Versioned API.
	v1 := r.Group("/api/v1")
	authH.Register(v1)
	user.Register(v1)
	roomH.Register(v1)
	tenantH.Register(v1)
	onboardingH.Register(v1)

	return r
}
