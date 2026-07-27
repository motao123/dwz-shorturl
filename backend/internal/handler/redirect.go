package handler

import (
	"net/http"

	"dwz-admin/internal/model"
	"dwz-admin/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type RedirectHandler struct {
	svc    service.ShortUrlService
	rdb    *redis.Client
	db     *gorm.DB
	logger *zap.Logger
}

func NewRedirectHandler(svc service.ShortUrlService, rdb *redis.Client, db *gorm.DB, logger *zap.Logger) *RedirectHandler {
	return &RedirectHandler{svc: svc, rdb: rdb, db: db, logger: logger}
}

func (h *RedirectHandler) Redirect(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.Redirect(http.StatusFound, "/")
		return
	}

	record, err := h.svc.ResolveByUID(code)
	if err != nil {
		c.String(http.StatusNotFound, "not found")
		return
	}

	// Async click log
	go h.logClick(record, c.ClientIP(), c.Request.UserAgent(), c.GetHeader("Referer"))

	c.Redirect(http.StatusFound, record.LongURL)
}

func (h *RedirectHandler) logClick(record *model.ShortUrl, ip, ua, referer string) {
	// If we got the record from cache, we might not have the ID.
	// Look it up if needed.
	if record.ID == 0 {
		var full model.ShortUrl
		if err := h.db.Where("uid = ?", record.UID).First(&full).Error; err != nil {
			return
		}
		record.ID = full.ID
	}

	clickLog := &model.ClickLog{
		ShortUrlID: record.ID,
		IP:         ip,
		UserAgent:  ua,
		Referer:    referer,
	}

	if err := h.db.Create(clickLog).Error; err != nil {
		h.logger.Error("failed to write click log",
			zap.Uint64("short_url_id", record.ID),
			zap.Error(err),
		)
	}

	// Increment click counter
	if err := h.db.Model(&model.ShortUrl{}).Where("id = ?", record.ID).
		UpdateColumn("clicks", gorm.Expr("clicks + 1")).Error; err != nil {
		h.logger.Error("failed to increment clicks",
			zap.Uint64("short_url_id", record.ID),
			zap.Error(err),
		)
	}
}
