package handler

import (
	"net/http"

	"dwz-admin/internal/model"
	"dwz-admin/internal/pkg"
	"dwz-admin/internal/service"

	"github.com/gin-gonic/gin"
)

type ConfigHandler struct {
	svc service.ConfigService
}

func NewConfigHandler(svc service.ConfigService) *ConfigHandler {
	return &ConfigHandler{svc: svc}
}

type BatchUpdateConfigRequest struct {
	Configs []ConfigItem `json:"configs" binding:"required,min=1"`
}

type ConfigItem struct {
	ConfigKey   string `json:"config_key" binding:"required"`
	ConfigValue string `json:"config_value" binding:"required"`
	ValueType   string `json:"value_type"`
}

func (h *ConfigHandler) GetAll(c *gin.Context) {
	configs, err := h.svc.GetAll()
	if err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "failed to get configs")
		return
	}

	pkg.Success(c, configs)
}

func (h *ConfigHandler) BatchUpdate(c *gin.Context) {
	var req BatchUpdateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, http.StatusBadRequest, pkg.CodeValidation, "configs array is required")
		return
	}

	userID := c.GetUint64("user_id")

	configs := make([]model.SystemConfig, 0, len(req.Configs))
	for _, item := range req.Configs {
		vt := item.ValueType
		if vt == "" {
			vt = "string"
		}
		configs = append(configs, model.SystemConfig{
			ConfigKey:   item.ConfigKey,
			ConfigValue: item.ConfigValue,
			ValueType:   vt,
		})
	}

	if err := h.svc.BatchUpdate(configs, userID); err != nil {
		pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "failed to update configs")
		return
	}

	pkg.Success(c, nil)
}
