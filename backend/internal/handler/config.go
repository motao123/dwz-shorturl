package handler

import (
	"net/http"
	"strings"

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

	// P1-6: never return secret-ish values (passwords, API tokens, SMTP creds)
	// to the frontend. The keys stay visible so admins know the setting exists.
	for i := range configs {
		if isSensitiveConfigKey(configs[i].ConfigKey) && configs[i].ConfigValue != "" {
			configs[i].ConfigValue = "******"
		}
	}

	pkg.Success(c, configs)
}

func isSensitiveConfigKey(key string) bool {
	k := strings.ToLower(key)
	for _, part := range []string{"password", "secret", "token", "apikey", "api_key", "appkey", "app_key", "private"} {
		if strings.Contains(k, part) {
			return true
		}
	}
	return false
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
