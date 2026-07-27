package middleware

import (
	"net/http"
	"runtime/debug"

	"dwz-admin/internal/pkg"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Recovery returns a middleware that recovers from panics, logs the stack trace,
// and returns a 500 JSON response.
func Recovery(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("panic recovered",
					zap.Any("error", err),
					zap.String("path", c.Request.URL.Path),
					zap.String("method", c.Request.Method),
					zap.ByteString("stack", debug.Stack()),
				)
				pkg.Fail(c, http.StatusInternalServerError, pkg.CodeInternalError, "internal server error")
				c.Abort()
			}
		}()
		c.Next()
	}
}
