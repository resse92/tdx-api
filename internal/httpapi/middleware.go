package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/resse/tdx-api/internal/config"
)

const requestIDKey = "request_id"

func requestID(c *gin.Context) string { v, _ := c.Get(requestIDKey); s, _ := v.(string); return s }

func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			var b [16]byte
			if _, err := rand.Read(b[:]); err == nil {
				id = hex.EncodeToString(b[:])
			} else {
				id = time.Now().UTC().Format("20060102150405.000000000")
			}
		}
		c.Set(requestIDKey, id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}
func loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		slog.Info("HTTP 请求", "method", c.Request.Method, "path", c.Request.URL.Path, "status", c.Writer.Status(), "duration", time.Since(started), "request_id", requestID(c))
	}
}
func recoveryMiddleware() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, _ any) {
		writeError(c, &APIError{Kind: KindInternal, Message: "服务内部错误"})
	})
}
func corsMiddleware(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && (slices.Contains(cfg.CORSOrigins, "*") || slices.Contains(cfg.CORSOrigins, origin)) {
			if slices.Contains(cfg.CORSOrigins, "*") {
				c.Header("Access-Control-Allow-Origin", "*")
			} else {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Vary", "Origin")
			}
			c.Header("Access-Control-Allow-Methods", strings.Join(cfg.CORSMethods, ","))
			c.Header("Access-Control-Allow-Headers", strings.Join(cfg.CORSHeaders, ","))
			if cfg.CORSAllowCreds {
				c.Header("Access-Control-Allow-Credentials", "true")
			}
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
