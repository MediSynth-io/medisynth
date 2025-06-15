package routes

import (
	"github.com/MediSynth-io/medisynth/internal/handlers"
	"github.com/MediSynth-io/medisynth/internal/middleware"
	"github.com/gin-gonic/gin"
)

// SetupAdminRoutes configures all admin routes
func SetupAdminRoutes(router *gin.Engine, h *handlers.Handler) {
	admin := router.Group("/admin")
	admin.Use(middleware.RequireAdmin())

	// Dashboard
	admin.GET("/dashboard", h.AdminDashboard)

	// User Management
	admin.GET("/users", h.AdminUsers)
	admin.POST("/users/:id/state", h.UpdateUserState)
	admin.POST("/users/:id/force-password-reset", h.ForcePasswordReset)
}
