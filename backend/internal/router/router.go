package router

import (
	"time"

	"dwz-admin/internal/config"
	"dwz-admin/internal/handler"
	"dwz-admin/internal/middleware"
	"dwz-admin/internal/pkg"
	"dwz-admin/internal/repository"

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
	Domain   *handler.DomainHandler
	Member   *handler.MemberHandler
	Violation *handler.ViolationHandler
	Monitor  *handler.MonitorHandler
	Webhook  *handler.WebhookHandler
	MemberApi *handler.MemberApiHandler
}

func Setup(engine *gin.Engine, h *Handlers, permFunc func(uint64) ([]string, error), logger *zap.Logger, cfg *config.Config, apiKeyRepo repository.ApiKeyRepo, rateLimiter *pkg.RateLimiter, memberRepo repository.MemberRepo) {
	// Global middleware
	engine.Use(middleware.CORS(cfg))
	engine.Use(middleware.Logger(logger))
	engine.Use(middleware.Recovery(logger))

	// Public redirect route. POST is needed for password-protected links: the
	// unlock interstitial posts the password back to the same short URL.
	engine.GET("/r/:code", h.Redirect.Redirect)
	engine.POST("/r/:code", h.Redirect.Redirect)

	// Public API (API-key authenticated)
	public := engine.Group("/public/api")
	{
		public.POST("/short-urls", middleware.RequireApiKey(apiKeyRepo, rateLimiter), h.ShortUrl.CreatePublic)
		public.POST("/short-urls/batch", middleware.RequireApiKey(apiKeyRepo, rateLimiter), h.ShortUrl.BatchCreatePublic)
	}

	// Member API (public registered users, authenticated by member JWT)
	member := engine.Group("/member/api")

	// Public member auth routes (no JWT required): password reset flow
	member.POST("/auth/forgot-password", h.MemberApi.RequestPasswordReset)
	member.POST("/auth/reset-password", h.MemberApi.ResetPassword)
	member.POST("/auth/send-verification", h.MemberApi.SendVerification)
	member.POST("/auth/verify-email", h.MemberApi.VerifyEmail)

	member.Use(middleware.MemberAuth(memberRepo.FindByID))
	{
		member.GET("/me", h.MemberApi.Me)
		member.GET("/summary", h.MemberApi.Summary)
		member.GET("/links", h.MemberApi.ListLinks)
		member.GET("/links/export", h.MemberApi.ExportLinks)
		member.POST("/links", h.MemberApi.CreateLink)
		member.POST("/links/batch", h.MemberApi.BatchCreateLinks)
		member.POST("/links/fetch-title", h.MemberApi.FetchTitle)
		member.POST("/links/import", h.MemberApi.ImportLinks)
		member.POST("/links/renew-expiring", h.MemberApi.RenewExpiring)
		member.PUT("/links/:id", h.MemberApi.UpdateLink)
		member.DELETE("/links/:id", h.MemberApi.DeleteLink)
		member.PUT("/links/:id/expiry", h.MemberApi.UpdateLinkExpiry)
		member.GET("/links/:uid/stats", h.MemberApi.GetLinkStats)
	}

	// Admin API group
	admin := engine.Group("/admin/api")
	{
		// Public auth routes (rate-limited to slow brute-force)
		admin.POST("/auth/login", middleware.RateLimitByIP(rateLimiter, "admin-login", 10, time.Minute), h.Auth.Login)
		// Refresh is public: the refresh token in the body is the credential, and
		// the frontend calls it without a (possibly expired) Bearer header. The
		// handler itself verifies the token type and signature.
		admin.POST("/auth/refresh", middleware.RateLimitByIP(rateLimiter, "admin-refresh", 30, time.Minute), h.Auth.Refresh)

		// Public domain routes (no auth required) - registered before /:id to avoid route conflict
		admin.GET("/domains/active", h.Domain.Active)

		// Authenticated routes
		auth := admin.Group("")
		auth.Use(middleware.Auth())
		auth.Use(middleware.LoadPermissions(permFunc))
		{
			// Auth
			auth.POST("/auth/logout", h.Auth.Logout)
			auth.GET("/auth/me", h.Auth.GetMe)

			// Short URLs
			shortUrls := auth.Group("/short-urls")
			{
				shortUrls.GET("", middleware.RequirePermission("short_urls", "read"), h.ShortUrl.List)
				shortUrls.GET("/export", middleware.RequirePermission("short_urls", "export"), h.ShortUrl.Export)
				shortUrls.GET("/:id", middleware.RequirePermission("short_urls", "read"), h.ShortUrl.GetByID)
				shortUrls.GET("/:id/check", middleware.RequirePermission("short_urls", "read"), h.ShortUrl.CheckLink)
				shortUrls.POST("", middleware.RequirePermission("short_urls", "create"), h.ShortUrl.Create)
				shortUrls.POST("/batch-create", middleware.RequirePermission("short_urls", "create"), h.ShortUrl.BatchCreate)
				shortUrls.POST("/import", middleware.RequirePermission("short_urls", "create"), h.ShortUrl.Import)
				shortUrls.POST("/batch-delete", middleware.RequirePermission("short_urls", "delete"), h.ShortUrl.BatchDelete)
				shortUrls.POST("/batch-update", middleware.RequirePermission("short_urls", "update"), h.ShortUrl.BatchUpdate)
				shortUrls.PUT("/:id", middleware.RequirePermission("short_urls", "update"), h.ShortUrl.Update)
				shortUrls.DELETE("/:id", middleware.RequirePermission("short_urls", "delete"), h.ShortUrl.Delete)
				shortUrls.POST("/:id/restore", middleware.RequirePermission("short_urls", "update"), h.ShortUrl.Restore)
			}

			// Domains
			domains := auth.Group("/domains")
			{
				domains.GET("", middleware.RequirePermission("domains", "read"), h.Domain.List)
				domains.GET("/:id", middleware.RequirePermission("domains", "read"), h.Domain.GetByID)
				domains.POST("", middleware.RequirePermission("domains", "create"), h.Domain.Create)
				domains.PUT("/:id", middleware.RequirePermission("domains", "update"), h.Domain.Update)
				domains.DELETE("/:id", middleware.RequirePermission("domains", "delete"), h.Domain.Delete)
				domains.POST("/:id/check", middleware.RequirePermission("domains", "update"), h.Domain.Check)
				domains.PUT("/batch-status", middleware.RequirePermission("domains", "update"), h.Domain.BatchStatus)
			}

			// Stats
			stats := auth.Group("/stats")
			{
				stats.GET("/overview", middleware.RequirePermission("stats", "read"), h.Stats.Overview)
				stats.GET("/trend", middleware.RequirePermission("stats", "read"), h.Stats.Trend)
				stats.GET("/top", middleware.RequirePermission("stats", "read"), h.Stats.TopN)
				stats.GET("/recent", middleware.RequirePermission("stats", "read"), h.Stats.Recent)
				stats.GET("/link/:id", middleware.RequirePermission("stats", "read"), h.Stats.LinkStats)
				stats.GET("/countries", middleware.RequirePermission("stats", "read"), h.Stats.Countries)
				stats.GET("/referrer-types", middleware.RequirePermission("stats", "read"), h.Stats.ReferrerTypes)
			}

			// System monitoring
			auth.GET("/monitor", middleware.RequirePermission("stats", "read"), h.Monitor.Status)
			auth.POST("/monitor/run-task", middleware.RequirePermission("stats", "update"), h.Monitor.RunTask)

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

			// Public members (registered users on the PHP frontend).
			// Reuses the `users` permission resource to avoid extra RBAC seeding.
			members := auth.Group("/members")
			{
				members.GET("", middleware.RequirePermission("users", "read"), h.Member.List)
				members.PUT("/:id/status", middleware.RequirePermission("users", "update"), h.Member.UpdateStatus)
				members.PUT("/:id/password", middleware.RequirePermission("users", "update"), h.Member.ResetPassword)
				members.DELETE("/:id", middleware.RequirePermission("users", "delete"), h.Member.Delete)
			}

			// Violation reviews (blocked URLs awaiting moderation).
			// Uses the audit resource: read to view, update/delete to act.
			violations := auth.Group("/violations")
			{
				violations.GET("", middleware.RequirePermission("audit", "read"), h.Violation.List)
				violations.PUT("/:id/review", middleware.RequirePermission("audit", "update"), h.Violation.MarkReviewed)
				violations.DELETE("/:id", middleware.RequirePermission("audit", "delete"), h.Violation.Delete)
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

			// Webhooks
			webhooks := auth.Group("/webhooks")
			{
				webhooks.GET("", middleware.RequirePermission("api_keys", "read"), h.Webhook.List)
				webhooks.POST("", middleware.RequirePermission("api_keys", "create"), h.Webhook.Create)
				webhooks.DELETE("/:id", middleware.RequirePermission("api_keys", "revoke"), h.Webhook.Delete)
				webhooks.POST("/:id/ping", middleware.RequirePermission("api_keys", "create"), h.Webhook.TestPing)
				webhooks.GET("/deliveries", middleware.RequirePermission("api_keys", "read"), h.Webhook.ListDeliveries)
				webhooks.POST("/deliveries/:id/retry", middleware.RequirePermission("api_keys", "create"), h.Webhook.RetryDelivery)
			}
		}
	}

	// Health check (detailed)
	engine.GET("/health", h.Redirect.Health)
}
