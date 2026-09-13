package middleware

import (
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"
)

func Logger(level string) gin.HandlerFunc {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		logger.Info("http",
			"method", param.Method,
			"path", param.Path,
			"status", param.StatusCode,
			"latency_ms", param.Latency.Milliseconds(),
			"client_ip", param.ClientIP,
			"errors", param.ErrorMessage,
		)
		return ""
	})
}