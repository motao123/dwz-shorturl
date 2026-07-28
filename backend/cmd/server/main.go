package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dwz-admin/internal/config"
	"dwz-admin/internal/handler"
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

	// Initialize logger
	zapLogger := initLogger(cfg.Log.Level, cfg.Log.File)
	defer zapLogger.Sync()

	// Initialize database
	db := initDB(cfg, zapLogger)

	// Initialize Redis
	rdb := initRedis(cfg)

	// Initialize click event queue (buffered channel + batch worker)
	clickQueue := handler.NewClickQueue(db, zapLogger)
	zapLogger.Info("click queue initialized", zap.Int("capacity", 2048))

	// Initialize cron scheduler
	cronSvc := service.NewCronService(db, zapLogger)

	// Initialize repositories
	userRepo := repository.NewUserRepo(db)
	roleRepo := repository.NewRoleRepo(db)
	shortUrlRepo := repository.NewShortUrlRepo(db)
	auditRepo := repository.NewAuditRepo(db)
	configRepo := repository.NewConfigRepo(db)
	apiKeyRepo := repository.NewApiKeyRepo(db)
	domainRepo := repository.NewDomainRepo(db)

	// Initialize services
	authSvc := service.NewAuthService(userRepo, roleRepo)
	shortUrlSvc := service.NewShortUrlService(shortUrlRepo, rdb, db, domainRepo)
	userSvc := service.NewUserService(userRepo)
	roleSvc := service.NewRoleService(roleRepo)
	statsSvc := service.NewStatsService(shortUrlRepo, db)
	configSvc := service.NewConfigService(configRepo)
	auditSvc := service.NewAuditService(auditRepo)
	apiKeySvc := service.NewApiKeyService(apiKeyRepo)
	domainSvc := service.NewDomainService(domainRepo)

	// Initialize handlers
	handlers := &router.Handlers{
		Auth:     handler.NewAuthHandler(authSvc),
		ShortUrl: handler.NewShortUrlHandler(shortUrlSvc),
		User:     handler.NewUserHandler(userSvc),
		Role:     handler.NewRoleHandler(roleSvc),
		Stats:    handler.NewStatsHandler(statsSvc),
		Config:   handler.NewConfigHandler(configSvc),
		Audit:    handler.NewAuditHandler(auditSvc),
		ApiKey:   handler.NewApiKeyHandler(apiKeySvc),
		Redirect: handler.NewRedirectHandler(shortUrlSvc, rdb, db, zapLogger, clickQueue),
		Domain:   handler.NewDomainHandler(domainSvc),
	}

	// Permission loader function for RBAC middleware
	permFunc := service.NewPermissionFunc(authSvc)

	// Setup Gin engine
	gin.SetMode(cfg.Server.Mode)
	engine := gin.New()

	// Register routes
	router.Setup(engine, handlers, permFunc, zapLogger)

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

	zapLogger.Info("server exited gracefully")
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
	// backend/migrations/ instead (schema.sql, migrate_wjoy_log.sql).
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
