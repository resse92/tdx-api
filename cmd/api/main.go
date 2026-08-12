package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/resse/tdx-api/internal/config"
	"github.com/resse/tdx-api/internal/httpapi"
	"github.com/resse/tdx-api/internal/tdx"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("配置加载失败", "error", err)
		os.Exit(1)
	}
	clients := tdx.New(cfg)
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: httpapi.NewRouter(cfg, clients), ReadHeaderTimeout: cfg.UpstreamTimeout}
	errCh := make(chan error, 1)
	go func() { slog.Info("TDX API 已启动", "address", cfg.HTTPAddr); errCh <- server.ListenAndServe() }()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	select {
	case <-ctx.Done():
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP 服务异常退出", "error", err)
			os.Exit(1)
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP 服务优雅关闭失败", "error", err)
	}
	if err := clients.Close(); err != nil {
		slog.Error("TDX 连接关闭失败", "error", err)
	}
}
