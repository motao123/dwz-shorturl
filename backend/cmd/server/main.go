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
	"dwz-admin/internal/model"
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
	configPath := "configs/config.yaml"
	if envPath := os.Getenv("CONFIG_PATH"); envPath != "" {
		configPath = envPath
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

	// Initialize repositories
	userRepo := repository.NewUserRepo(db)
	roleRepo := repository.NewRoleRepo(db)
	shortUrlRepo := repository.NewShortUrlRepo(db)
	auditRepo := repository.NewAuditRepo(db)
	configRepo := repository.NewConfigRepo(db)
	apiKeyRepo := repository.NewApiKeyRepo(db)

	// Initialize services
	authSvc := service.NewAuthService(userRepo, roleRepo)
	shortUrlSvc := service.NewShortUrlService(shortUrlRepo, rdb)
	userSvc := service.NewUserService(userRepo)
	roleSvc := service.NewRoleService(roleRepo)
	statsSvc := service.NewStatsService(shortUrlRepo, db)
	configSvc := service.NewConfigService(configRepo)
	auditSvc := service.NewAuditService(auditRepo)
	apiKeySvc := service.NewApiKeyService(apiKeyRepo)

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
		Redirect: handler.NewRedirectHandler(shortUrlSvc, rdb, db, zapLogger),
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		zapLogger.Fatal("server forced to shutdown", zap.Error(err))
	}

	zapLogger.Info("server exited")
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

	// Auto-migrate tables (safe for development; production should use migrations)
	if err := db.AutoMigrate(
		&model.User{},
		&model.Role{},
		&model.Permission{},
		&model.RolePermission{},
		&model.UserRole{},
		&model.ShortUrl{},
		&model.UrlCategory{},
		&model.AuditLog{},
		&model.SystemConfig{},
		&model.ApiKey{},
	); err != nil {
		zapLogger.Warn("auto-migrate failed (may already exist)", zap.Error(err))
	}

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
