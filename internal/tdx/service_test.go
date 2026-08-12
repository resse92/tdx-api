package tdx

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/resse/tdx-api/internal/config"
)

func TestServiceLifecycle(t *testing.T) {
	s := New(config.Config{PoolSize: 2, UpstreamTimeout: time.Second})
	if !s.Ready() {
		t.Fatal("服务应就绪")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
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
	s := New(config.Config{PoolSize: 1, UpstreamTimeout: time.Second})
	defer s.Close()
	var mu sync.Mutex
	active, maxActive := 0, 0
	fn := func() {
		b := <-s.pool
		b.mu.Lock()
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
		b.mu.Unlock()
		s.pool <- b
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); fn() }()
	}
	wg.Wait()
	if maxActive != 1 {
		t.Fatalf("并发数=%d", maxActive)
	}
}
