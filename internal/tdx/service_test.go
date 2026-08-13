package tdx

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bensema/gotdx"
	"github.com/resse/tdx-api/internal/config"
)

type fakeBundle struct {
	b                               *clients
	mainConnects, macConnects       atomic.Int32
	mainDisconnects, macDisconnects atomic.Int32
	mainErr, macErr, invokeErr      error
	result                          any
}

func newFakeBundle() *fakeBundle {
	f := &fakeBundle{result: "ok"}
	f.b = &clients{}
	f.b.connectMain = func() error { f.mainConnects.Add(1); return f.mainErr }
	f.b.connectMAC = func() error { f.macConnects.Add(1); return f.macErr }
	f.b.disconnectMain = func() error { f.mainDisconnects.Add(1); return nil }
	f.b.disconnectMAC = func() error { f.macDisconnects.Add(1); return nil }
	f.b.mainAddress = func() string { return "main.example:7709" }
	f.b.macAddress = func() string { return "mac.example:7709" }
	f.b.invoke = func(string, Params) (any, error) { return f.result, f.invokeErr }
	return f
}

func testService(t *testing.T, retries int, bundles ...*fakeBundle) *Service {
	return testServiceWithLogger(t, slog.New(slog.NewTextHandler(io.Discard, nil)), retries, bundles...)
}

func testServiceWithLogger(t *testing.T, logger *slog.Logger, retries int, bundles ...*fakeBundle) *Service {
	t.Helper()
	i := 0
	s, err := newService(config.Config{PoolSize: len(bundles), RetryLimit: retries, UpstreamTimeout: time.Second}, logger, func([]gotdx.Option) *clients {
		b := bundles[i].b
		i++
		return b
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestServiceConnectsAtStartupAndClosesOnce(t *testing.T) {
	f1, f2 := newFakeBundle(), newFakeBundle()
	s := testService(t, 0, f1, f2)
	if !s.Ready() || f1.mainConnects.Load() != 1 || f1.macConnects.Load() != 1 || f2.mainConnects.Load() != 1 || f2.macConnects.Load() != 1 {
		t.Fatalf("启动状态错误: ready=%v f1=%d/%d f2=%d/%d", s.Ready(), f1.mainConnects.Load(), f1.macConnects.Load(), f2.mainConnects.Load(), f2.macConnects.Load())
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if s.Ready() || f1.mainDisconnects.Load() != 1 || f1.macDisconnects.Load() != 1 {
		t.Fatalf("关闭状态错误: ready=%v disconnect=%d/%d", s.Ready(), f1.mainDisconnects.Load(), f1.macDisconnects.Load())
	}
}

func TestStartupFailureCleansUpClients(t *testing.T) {
	tests := []struct {
		name string
		set  func(*fakeBundle)
	}{
		{"main", func(f *fakeBundle) { f.mainErr = errors.New("main down") }},
		{"mac", func(f *fakeBundle) { f.macErr = errors.New("mac down") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeBundle()
			tt.set(f)
			_, err := newService(config.Config{PoolSize: 1, UpstreamTimeout: time.Second}, slog.New(slog.NewTextHandler(io.Discard, nil)), func([]gotdx.Option) *clients { return f.b })
			if err == nil || f.mainDisconnects.Load() != 1 || f.macDisconnects.Load() != 1 {
				t.Fatalf("err=%v disconnect=%d/%d", err, f.mainDisconnects.Load(), f.macDisconnects.Load())
			}
		})
	}
}

func TestCallReusesStartupConnections(t *testing.T) {
	f := newFakeBundle()
	s := testService(t, 0, f)
	defer s.Close()
	for _, op := range []string{"stock.announcement", "company.finance", "mac.symbol.info"} {
		if _, err := s.Call(context.Background(), op, Params{}); err != nil {
			t.Fatal(err)
		}
	}
	if f.mainConnects.Load() != 1 || f.macConnects.Load() != 1 {
		t.Fatalf("健康连接被重复建立: %d/%d", f.mainConnects.Load(), f.macConnects.Load())
	}
}

func TestRetryReconnectsOnlyFailedProtocol(t *testing.T) {
	f := newFakeBundle()
	calls := 0
	f.b.invoke = func(string, Params) (any, error) {
		calls++
		if calls == 1 {
			return nil, io.EOF
		}
		return "ok", nil
	}
	s := testService(t, 1, f)
	defer s.Close()
	value, err := s.Call(context.Background(), "stock.announcement", Params{})
	if err != nil || value != "ok" || f.mainConnects.Load() != 2 || f.mainDisconnects.Load() != 1 || f.macConnects.Load() != 1 || f.macDisconnects.Load() != 0 {
		t.Fatalf("value=%v err=%v main=%d/%d mac=%d/%d", value, err, f.mainConnects.Load(), f.mainDisconnects.Load(), f.macConnects.Load(), f.macDisconnects.Load())
	}
}

func TestReadinessTracksProtocolState(t *testing.T) {
	f := newFakeBundle()
	s := testService(t, 0, f)
	defer s.Close()
	f.b.mainReady.Store(false)
	if s.Ready() {
		t.Fatal("主市场断开时不应就绪")
	}
	f.b.mainReady.Store(true)
	f.b.macReady.Store(false)
	if s.Ready() {
		t.Fatal("MAC 断开时不应就绪")
	}
}

func TestCloseWaitsForCallAndRejectsNewCalls(t *testing.T) {
	f := newFakeBundle()
	started, release := make(chan struct{}), make(chan struct{})
	f.b.invoke = func(string, Params) (any, error) {
		close(started)
		<-release
		return "ok", nil
	}
	s := testService(t, 0, f)
	callDone := make(chan error, 1)
	go func() { _, err := s.Call(context.Background(), "stock.count", Params{}); callDone <- err }()
	<-started
	closeDone := make(chan error, 1)
	go func() { closeDone <- s.Close() }()
	select {
	case <-closeDone:
		t.Fatal("关闭不应早于在途调用")
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	if err := <-callDone; err != nil {
		t.Fatal(err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	if _, err := s.Call(context.Background(), "stock.count", Params{}); err == nil {
		t.Fatal("关闭后调用应失败")
	}
}

func TestRetryBounded(t *testing.T) {
	calls := 0
	want := errors.New("失败")
	_, err := retry(context.Background(), 2, func(error) bool { return true }, func() (any, error) { calls++; return nil, want })
	if !errors.Is(err, want) || calls != 3 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestRetryStopsAfterSuccess(t *testing.T) {
	calls := 0
	value, err := retry(context.Background(), 3, func(error) bool { return true }, func() (any, error) {
		calls++
		if calls < 2 {
			return nil, errors.New("暂时失败")
		}
		return "ok", nil
	})
	if err != nil || value != "ok" || calls != 2 {
		t.Fatalf("value=%v calls=%d err=%v", value, calls, err)
	}
}

func TestRetryDoesNotHideNonRetryableError(t *testing.T) {
	calls := 0
	want := errors.New("参数错误")
	_, err := retry(context.Background(), 3, func(error) bool { return false }, func() (any, error) { calls++; return nil, want })
	if !errors.Is(err, want) || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestPoolSerializesSingleClient(t *testing.T) {
	f := newFakeBundle()
	s := testService(t, 0, f)
	defer s.Close()
	var mu sync.Mutex
	active, maxActive := 0, 0
	f.b.invoke = func(string, Params) (any, error) {
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()
		time.Sleep(time.Millisecond)
		mu.Lock()
		active--
		mu.Unlock()
		return nil, nil
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = s.Call(context.Background(), "stock.count", Params{}) }()
	}
	wg.Wait()
	if maxActive != 1 {
		t.Fatalf("并发数=%d", maxActive)
	}
}

func TestConnectionLifecycleLogs(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	f := newFakeBundle()
	calls := 0
	f.b.invoke = func(string, Params) (any, error) {
		calls++
		if calls == 1 {
			return nil, io.EOF
		}
		return "ok", nil
	}
	s := testServiceWithLogger(t, logger, 1, f)
	if _, err := s.Call(context.Background(), "stock.count", Params{}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	output := logs.String()
	for _, want := range []string{
		`"msg":"TDX 连接中"`,
		`"msg":"TDX 连接成功"`,
		`"msg":"TDX 连接断开"`,
		`"protocol":"main"`,
		`"protocol":"mac"`,
		`"client_group":1`,
		`"trigger":"startup"`,
		`"trigger":"request"`,
		`"reason":"protocol_error"`,
		`"reason":"shutdown"`,
		`"address":"main.example:7709"`,
		`"error":"EOF"`,
	} {
		if !bytes.Contains(logs.Bytes(), []byte(want)) {
			t.Fatalf("日志缺少 %s:\n%s", want, output)
		}
	}
	if bytes.Contains(logs.Bytes(), []byte(`"Market"`)) || bytes.Contains(logs.Bytes(), []byte(`"Symbols"`)) {
		t.Fatalf("日志不应包含业务参数:\n%s", output)
	}
}

func TestConnectionFailureLogIncludesContext(t *testing.T) {
	var logs bytes.Buffer
	f := newFakeBundle()
	f.mainErr = errors.New("dial failed")
	_, err := newService(config.Config{PoolSize: 1, UpstreamTimeout: time.Second}, slog.New(slog.NewJSONHandler(&logs, nil)), func([]gotdx.Option) *clients { return f.b })
	if err == nil {
		t.Fatal("应返回启动连接错误")
	}
	for _, want := range []string{`"msg":"TDX 连接失败"`, `"protocol":"main"`, `"client_group":1`, `"trigger":"startup"`, `"error":"dial failed"`} {
		if !bytes.Contains(logs.Bytes(), []byte(want)) {
			t.Fatalf("日志缺少 %s:\n%s", want, logs.String())
		}
	}
}
