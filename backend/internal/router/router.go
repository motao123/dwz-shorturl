package router

import (
	"dwz-admin/internal/handler"
	"dwz-admin/internal/middleware"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type Handlers struct {
	Auth     *handler.AuthHandler
	ShortUrl *handler.ShortUrlHandler
	User     *handler.UserHandler
	Role     *handler.RoleHandler
	Stats    *handler.StatsHandler
	Config   *handler.ConfigHandler
	Audit    *handler.AuditHandler
	ApiKey   *handler.ApiKeyHandler
	Redirect *handler.RedirectHandler
}

func Setup(engine *gin.Engine, h *Handlers, permFunc func(uint64) ([]string, error), logger *zap.Logger) {
	// Global middleware
	engine.Use(middleware.CORS())
	engine.Use(middleware.Logger(logger))
	engine.Use(middleware.Recovery(logger))

	// Public redirect route
	engine.GET("/r/:code", h.Redirect.Redirect)

	// Admin API group
	admin := engine.Group("/admin/api")
	{
		// Public auth routes
		admin.POST("/auth/login", h.Auth.Login)

		// Authenticated routes
		auth := admin.Group("")
		auth.Use(middleware.Auth())
		auth.Use(middleware.LoadPermissions(permFunc))
		{
			// Auth
			auth.POST("/auth/refresh", h.Auth.Refresh)
			auth.POST("/auth/logout", h.Auth.Logout)
			auth.GET("/auth/me", h.Auth.GetMe)

			// Short URLs
			shortUrls := auth.Group("/short-urls")
			{
				shortUrls.GET("", middleware.RequirePermission("short_urls", "read"), h.ShortUrl.List)
				shortUrls.GET("/export", middleware.RequirePermission("short_urls", "export"), h.ShortUrl.Export)
				shortUrls.GET("/:id", middleware.RequirePermission("short_urls", "read"), h.ShortUrl.GetByID)
				shortUrls.POST("", middleware.RequirePermission("short_urls", "create"), h.ShortUrl.Create)
				shortUrls.POST("/batch-create", middleware.RequirePermission("short_urls", "create"), h.ShortUrl.BatchCreate)
				shortUrls.POST("/batch-delete", middleware.RequirePermission("short_urls", "delete"), h.ShortUrl.BatchDelete)
				shortUrls.PUT("/:id", middleware.RequirePermission("short_urls", "update"), h.ShortUrl.Update)
				shortUrls.DELETE("/:id", middleware.RequirePermission("short_urls", "delete"), h.ShortUrl.Delete)
			}

			// Stats
			stats := auth.Group("/stats")
			{
				stats.GET("/overview", middleware.RequirePermission("stats", "read"), h.Stats.Overview)
				stats.GET("/trend", middleware.RequirePermission("stats", "read"), h.Stats.Trend)
				stats.GET("/top", middleware.RequirePermission("stats", "read"), h.Stats.TopN)
				stats.GET("/recent", middleware.RequirePermission("stats", "read"), h.Stats.Recent)
			}

			// Users
			users := auth.Group("/users")
			{
				users.GET("", middleware.RequirePermission("users", "read"), h.User.List)
				users.GET("/:id", middleware.RequirePermission("users", "read"), h.User.GetByID)
				users.POST("", middleware.RequirePermission("users", "create"), h.User.Create)
				users.PUT("/:id", middleware.RequirePermission("users", "update"), h.User.Update)
				users.PUT("/:id/password", middleware.RequirePermission("users", "update"), h.User.ResetPassword)
				users.PUT("/:id/roles", middleware.RequirePermission("users", "assign_roles"), h.User.AssignRoles)
				users.DELETE("/:id", middleware.RequirePermission("users", "delete"), h.User.Delete)
			}

			// Roles
			roles := auth.Group("/roles")
			{
				roles.GET("", middleware.RequirePermission("roles", "read"), h.Role.List)
				roles.POST("", middleware.RequirePermission("roles", "create"), h.Role.Create)
				roles.PUT("/:id", middleware.RequirePermission("roles", "update"), h.Role.Update)
				roles.PUT("/:id/permissions", middleware.RequirePermission("roles", "update"), h.Role.SetPermissions)
				roles.GET("/:id/permissions", middleware.RequirePermission("roles", "read"), h.Role.GetPermissions)
				roles.DELETE("/:id", middleware.RequirePermission("roles", "delete"), h.Role.Delete)
			}

			// Permissions
			auth.GET("/permissions", middleware.RequirePermission("roles", "read"), h.Role.GetAllPermissions)

			// System Configs
			configs := auth.Group("/configs")
			{
				configs.GET("", middleware.RequirePermission("configs", "read"), h.Config.GetAll)
				configs.PUT("", middleware.RequirePermission("configs", "update"), h.Config.BatchUpdate)
			}

			// Audit Logs
			auditLogs := auth.Group("/audit-logs")
			{
				auditLogs.GET("", middleware.RequirePermission("audit", "read"), h.Audit.List)
				auditLogs.GET("/:id", middleware.RequirePermission("audit", "read"), h.Audit.GetByID)
			}

			// API Keys
			apiKeys := auth.Group("/api-keys")
			{
				apiKeys.GET("", middleware.RequirePermission("api_keys", "read"), h.ApiKey.List)
				apiKeys.POST("", middleware.RequirePermission("api_keys", "create"), h.ApiKey.Create)
				apiKeys.DELETE("/:id", middleware.RequirePermission("api_keys", "revoke"), h.ApiKey.Revoke)
				apiKeys.GET("/:id/stats", middleware.RequirePermission("api_keys", "read"), h.ApiKey.GetStats)
			}
		}
	}

	// Health check (detailed)
	engine.GET("/health", h.Redirect.Health)
}
