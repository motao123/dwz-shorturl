package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"dwz-admin/internal/config"
	"dwz-admin/internal/handler"
	"dwz-admin/internal/middleware"
	"dwz-admin/internal/pkg"
	"dwz-admin/internal/repository"
	"dwz-admin/internal/router"
	"dwz-admin/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	// Load configuration
	configPath := "configs/config.yaml" // production config (gitignored)
	if envPath := os.Getenv("CONFIG_PATH"); envPath != "" {
		configPath = envPath
	}
	// Fallback to example config if production config doesn't exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configPath = "configs/config.example.yaml"
	}
	if err := config.Init(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}
	cfg := config.Get()

	// P0-3: refuse to boot with a known-default JWT secret. Anyone who knows the
	// placeholder value can forge a super_admin token, so a weak secret is worse
	// than no secret at all: fail fast with an explicit remediation message.
	if isInsecureJWTSecret(cfg.JWT.Secret) {
		fmt.Fprintf(os.Stderr, "FATAL: jwt.secret is still set to an insecure default value.\n"+
			"Generate a strong random secret (e.g. `openssl rand -hex 32`) and set it in\n"+
			"configs/config.yaml under jwt.secret (or via DWZ_JWT_SECRET) before starting.\n")
		os.Exit(1)
	}

	// Initialize logger
	zapLogger := initLogger(cfg.Log.Level, cfg.Log.File)
	defer zapLogger.Sync()

	// public.base_url is no longer defaulted to any domain (#20). Without it the
	// API returns relative short-link addresses and expiry emails omit the member
	// center link — correct, but almost never what an operator intended, so say so.
	if strings.TrimSpace(cfg.Public.BaseURL) == "" {
		zapLogger.Warn("public.base_url is empty: short links will be returned as relative addresses",
			zap.String("remediation", "set public.base_url in configs/config.yaml to your own site origin"))
	}

	// Initialize database
	db := initDB(cfg, zapLogger)

	// Initialize public frontend DB (members table lives here, separate schema)
	publicDB := initPublicDB(cfg, zapLogger)

	// Initialize Redis
	rdb := initRedis(cfg)

	// P1-4: register the JWT logout blacklist (revoked jti → rejected by Auth middleware).
	middleware.SetTokenBlacklistCheck(func(jti string) bool {
		if jti == "" {
			return false
		}
		n, err := rdb.Exists(rdb.Context(), "jwt:blacklist:"+jti).Result()
		return err == nil && n > 0
	})

	// Initialize click event queue (buffered channel + batch worker)
	clickQueue := handler.NewClickQueue(db, zapLogger)
	zapLogger.Info("click queue initialized", zap.Int("capacity", 2048))

	// GeoIP country resolution (optional): resolve click source country from IP
	// when an ip2region v1 database is configured.
	if cfg.GeoIP.DBPath != "" {
		geo, err := pkg.NewIp2Region(cfg.GeoIP.DBPath)
		if err != nil {
			zapLogger.Warn("geoip database failed to load; country resolution disabled",
				zap.String("path", cfg.GeoIP.DBPath), zap.Error(err))
		} else {
			clickQueue.SetGeoCountry(func(ip string) string {
				region, err := geo.Search(ip)
				if err != nil {
					return ""
				}
				return pkg.CountryCodeFromRegion(region)
			})
			zapLogger.Info("geoip country resolution enabled", zap.String("path", cfg.GeoIP.DBPath))
		}
	}

	// Initialize cron scheduler
	emailSvc := service.NewEmailService(cfg.SMTP, zapLogger)
	cronSvc := service.NewCronService(db, publicDB, zapLogger, emailSvc)

	// Initialize repositories
	userRepo := repository.NewUserRepo(db)
	roleRepo := repository.NewRoleRepo(db)
	shortUrlRepo := repository.NewShortUrlRepo(db)
	auditRepo := repository.NewAuditRepo(db)
	configRepo := repository.NewConfigRepo(db)
	apiKeyRepo := repository.NewApiKeyRepo(db)
	domainRepo := repository.NewDomainRepo(db)
	memberRepo := repository.NewMemberRepo(publicDB)
	violationRepo := repository.NewViolationRepo(publicDB)
	wjoyLogRepo := repository.NewWjoyLogRepo(publicDB)
	webhookRepo := repository.NewWebhookRepo(db)

	// Runtime governance switches (#4): one resolver, consulted by the create path,
	// and invalidated whenever the admin UI saves a config so a toggle takes effect
	// at once instead of after the cache TTL.
	runtimeCfg := service.NewRuntimeConfig(configRepo, zapLogger)

	// Initialize services
	authSvc := service.NewAuthService(userRepo, roleRepo).WithTokenBlacklist(func(jti string) bool {
		if jti == "" {
			return false
		}
		n, err := rdb.Exists(rdb.Context(), "jwt:blacklist:"+jti).Result()
		return err == nil && n > 0
	})
	shortUrlSvc := service.NewShortUrlService(shortUrlRepo, rdb, db, wjoyLogRepo, domainRepo, violationRepo).
		WithSettings(runtimeCfg)
	userSvc := service.NewUserService(userRepo)
	roleSvc := service.NewRoleService(roleRepo)
	statsSvc := service.NewStatsService(shortUrlRepo, db)
	configSvc := service.NewConfigService(configRepo).WithRuntimeConfig(runtimeCfg)
	auditSvc := service.NewAuditService(auditRepo)
	// #40: the API-key service needs the super_admin resolver so its ownership
	// guard can let a super_admin manage any key while still refusing everybody
	// else. Without it the guard stays strict (owner-only), which is safe.
	apiKeySvc := service.NewApiKeyService(apiKeyRepo).
		WithSuperAdminChecker(userSvc.IsSuperAdmin)
	domainSvc := service.NewDomainService(domainRepo)
	memberSvc := service.NewMemberService(memberRepo).WithPurge(repository.NewMemberPurgeRepo(db, publicDB), zapLogger)
	violationSvc := service.NewViolationService(violationRepo)
	monitorSvc := service.NewMonitorService(db, rdb, clickQueue, cronSvc, zapLogger)
	webhookSvc := service.NewWebhookService(webhookRepo, zapLogger)
	memberApiSvc := service.NewMemberApiService(shortUrlRepo, wjoyLogRepo, memberRepo, db, emailSvc, shortUrlSvc)

	// Dispatch link.clicked webhooks when clicks are recorded.
	clickQueue.SetOnClick(func(uid string) {
		webhookSvc.Dispatch("link.clicked", map[string]interface{}{
			"uid":       uid,
			"short_url": service.PublicShortURL(uid),
		})
	})

	// click_logs partition maintenance alerts. The cron task used to only write
	// a log line, so a failing ALTER (DDL lock, full disk, revoked privilege)
	// could exhaust the month partitions unnoticed and silently degrade
	// click_logs into one huge p_future table. Alerts now reach the same webhook
	// channel as other operational events, and the state is exposed on
	// /admin/api/monitor/partitions.
	cronSvc.SetPartitionAlertHook(func(res *service.PartitionReconcileResult, st *service.PartitionStatus) {
		payload := map[string]interface{}{
			"table":                st.Table,
			"level":                st.AlertLevel,
			"alerts":               st.Alerts,
			"coverage_until":       st.LatestMonth,
			"target_month":         st.RequiredMonth,
			"months_behind":        st.BehindMonths,
			"pending_months":       st.PendingMonths,
			"future_rows":          st.FutureRows,
			"future_ratio":         st.FutureRatio,
			"consecutive_failures": st.ConsecutiveFailures,
		}
		if res != nil {
			payload["created"] = res.Created
			payload["failed_month"] = res.FailedMonth
		}
		event := "system.partition_alert"
		if st.AlertLevel == "ok" {
			event = "system.partition_ok"
		}
		webhookSvc.Dispatch(event, payload)
	})

	// Rate limiter (Redis-backed, shared across handlers)
	rateLimiter := pkg.NewRateLimiter(rdb)

	// Account-level lockout for admin login: the per-IP limiter in the router
	// only bounds a single source address, so the username counter is what
	// actually caps a distributed brute-force run against a known account.
	authSvc.WithLoginLimiter(rateLimiter)

	// Initialize handlers
	handlers := &router.Handlers{
		Auth:      handler.NewAuthHandler(authSvc, rdb),
		ShortUrl:  handler.NewShortUrlHandler(shortUrlSvc, rateLimiter, cfg.RateLimit, auditSvc, webhookSvc),
		User:      handler.NewUserHandler(userSvc, auditSvc),
		Role:      handler.NewRoleHandler(roleSvc, auditSvc),
		Stats:     handler.NewStatsHandler(statsSvc),
		Config:    handler.NewConfigHandler(configSvc),
		Audit:     handler.NewAuditHandler(auditSvc),
		ApiKey:    handler.NewApiKeyHandler(apiKeySvc),
		Redirect:  handler.NewRedirectHandler(shortUrlSvc, rdb, db, zapLogger, clickQueue),
		Domain:    handler.NewDomainHandler(domainSvc, auditSvc),
		Member:    handler.NewMemberHandler(memberSvc, auditSvc),
		Violation: handler.NewViolationHandler(violationSvc, auditSvc),
		Monitor:   handler.NewMonitorHandler(monitorSvc),
		Webhook:   handler.NewWebhookHandler(webhookSvc, auditSvc),
		MemberApi: handler.NewMemberApiHandler(memberApiSvc),
		Metrics:   handler.NewMetricsHandler(db, publicDB, rdb, cfg.Redis.Addr != "", clickQueue),
	}

	// Permission loader function for RBAC middleware
	permFunc := service.NewPermissionFunc(authSvc)

	// Setup Gin engine
	gin.SetMode(cfg.Server.Mode)
	engine := gin.New()

	// P0-4: only honor X-Forwarded-For from trusted reverse proxies (nginx).
	// With an empty list, ClientIP() falls back to the direct peer address, so
	// all traffic behind a local nginx would collapse to 127.0.0.1 and per-IP
	// rate limiting would stop working. Production must list the proxy IPs here
	// (e.g. ["127.0.0.1"]) — otherwise attackers can spoof X-Forwarded-For to
	// bypass the login brute-force limiter.
	if err := engine.SetTrustedProxies(cfg.Public.TrustedProxies); err != nil {
		zapLogger.Fatal("invalid trusted_proxies configuration", zap.Error(err))
	}

	// Register routes
	router.Setup(engine, handlers, permFunc, zapLogger, cfg, apiKeyRepo, rateLimiter, memberRepo, runtimeCfg)

	// Create HTTP server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: engine,
	}

	// Start cron scheduler
	cronSvc.Start()
	defer func() {
		cronSvc.Stop()
	}()

	// Start server in goroutine
	go func() {
		zapLogger.Info("server starting", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zapLogger.Fatal("server failed", zap.Error(err))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	zapLogger.Info("shutting down server...")

	// 1. Stop HTTP server (no new requests)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		zapLogger.Fatal("server forced to shutdown", zap.Error(err))
	}

	// 2. Stop click queue (flush remaining events)
	zapLogger.Info("flushing click queue...")
	clickQueue.Stop()
	zapLogger.Info("click queue stopped")

	// 3. Close Redis
	if err := rdb.Close(); err != nil {
		zapLogger.Warn("failed to close redis", zap.Error(err))
	}

	// 4. Close DB
	if sqlDB, err := db.DB(); err == nil {
		if err := sqlDB.Close(); err != nil {
			zapLogger.Warn("failed to close database", zap.Error(err))
		}
	}
	if publicDB != nil {
		if sqlDB, err := publicDB.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				zapLogger.Warn("failed to close public database", zap.Error(err))
			}
		}
	}

	zapLogger.Info("server exited gracefully")
}

// insecureJWTSecrets are placeholder values shipped in example configs and the
// docker-compose defaults. A deployment still using any of these is vulnerable
// to token forgery and must not start.
var insecureJWTSecrets = []string{
	"change-me-to-random-32-bytes!!",
	"change-me-in-production-32bytes!",
}

func isInsecureJWTSecret(secret string) bool {
	for _, s := range insecureJWTSecrets {
		if secret == s {
			return true
		}
	}
	return false
}

func initLogger(level, file string) *zap.Logger {
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	var cores []zapcore.Core

	// Console encoder
	consoleEncoder := zapcore.NewConsoleEncoder(zap.NewProductionEncoderConfig())
	cores = append(cores, zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), zapLevel))

	// File encoder (if configured)
	if file != "" {
		f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			fileEncoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
			cores = append(cores, zapcore.NewCore(fileEncoder, zapcore.AddSync(f), zapLevel))
		}
	}

	core := zapcore.NewTee(cores...)
	return zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
}

func initDB(cfg *config.Config, zapLogger *zap.Logger) *gorm.DB {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.DBName,
		cfg.Database.Charset,
	)

	gormLogger := logger.Default.LogMode(logger.Warn)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		zapLogger.Fatal("failed to connect database", zap.Error(err))
	}

	sqlDB, err := db.DB()
	if err != nil {
		zapLogger.Fatal("failed to get sql.DB", zap.Error(err))
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// NOTE: AutoMigrate is disabled. Use the SQL migration files in
	// backend/migrations/ via `go run ./cmd/migrate`
	// (schema.sql baseline + add_* backfills + php/ side migrations).
	// This prevents GORM from altering columns/indexes that were carefully
	// defined in the hand-written DDL.

	zapLogger.Info("database connected",
		zap.String("host", cfg.Database.Host),
		zap.Int("port", cfg.Database.Port),
		zap.String("dbname", cfg.Database.DBName),
	)

	return db
}

func initRedis(cfg *config.Config) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: redis connection failed: %v\n", err)
	}

	return rdb
}

// initPublicDB opens the public frontend database (where the `members` table
// lives). Unlike the admin DB, it is optional: if not configured, the member
// management feature reports empty results instead of failing to boot.
func initPublicDB(cfg *config.Config, zapLogger *zap.Logger) *gorm.DB {
	if cfg.PublicDB.User == "" || cfg.PublicDB.DBName == "" {
		zapLogger.Warn("public_db not configured; member management disabled")
		return nil
	}
	return initDBSql(cfg.PublicDB, zapLogger)
}

func initDBSql(cfg config.DatabaseConfig, zapLogger *zap.Logger) *gorm.DB {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DBName, cfg.Charset,
	)
	gormLogger := logger.Default.LogMode(logger.Warn)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: gormLogger})
	if err != nil {
		zapLogger.Fatal("failed to connect database", zap.Error(err))
	}
	sqlDB, err := db.DB()
	if err != nil {
		zapLogger.Fatal("failed to get sql.DB", zap.Error(err))
	}
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetMaxOpenConns(30)
	sqlDB.SetConnMaxLifetime(time.Hour)
	zapLogger.Info("database connected",
		zap.String("host", cfg.Host), zap.Int("port", cfg.Port), zap.String("dbname", cfg.DBName))
	return db
}
