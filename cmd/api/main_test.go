package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/resse/tdx-api/internal/config"
	"github.com/resse/tdx-api/internal/tdx"
)

type fakeCaller struct{}

func (fakeCaller) Call(context.Context, string, tdx.Params) (any, error) { return nil, nil }
func (fakeCaller) Ready() bool                                           { return true }
func (fakeCaller) Close() error                                          { return nil }

func TestAssemblePropagatesTDXStartupError(t *testing.T) {
	want := errors.New("连接失败")
	server, clients, err := assemble(config.Config{}, func(config.Config) (tdx.Caller, error) { return nil, want })
	if !errors.Is(err, want) || server != nil || clients != nil {
		t.Fatalf("server=%v clients=%v err=%v", server, clients, err)
	}
}

func TestAssembleCreatesServerAfterTDXStartup(t *testing.T) {
	cfg := config.Config{HTTPAddr: ":8080", GinMode: "test", CORSOrigins: []string{"*"}, CORSMethods: []string{"GET"}, PoolSize: 1, UpstreamTimeout: time.Second, MaxItems: 1}
	server, clients, err := assemble(cfg, func(config.Config) (tdx.Caller, error) { return fakeCaller{}, nil })
	if err != nil || server == nil || clients == nil || server.Addr != cfg.HTTPAddr {
		t.Fatalf("server=%v clients=%v err=%v", server, clients, err)
	}
}
