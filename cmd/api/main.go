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
	server, clients, err := assemble(cfg, func(c config.Config) (tdx.Caller, error) { return tdx.New(c) })
	if err != nil {
		slog.Error("TDX 连接初始化失败", "error", err)
		os.Exit(1)
	}
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

func assemble(cfg config.Config, create func(config.Config) (tdx.Caller, error)) (*http.Server, tdx.Caller, error) {
	clients, err := create(cfg)
	if err != nil {
		return nil, nil, err
	}
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: httpapi.NewRouter(cfg, clients), ReadHeaderTimeout: cfg.UpstreamTimeout}
	return server, clients, nil
}
