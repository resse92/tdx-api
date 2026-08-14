package boardcache

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type memoryRepo struct {
	mu      sync.Mutex
	boards  any
	members map[string]any
	writes  int
}

func (r *memoryRepo) Boards(_ context.Context, limit uint32) (any, bool, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return limitValue(r.boards, limit), r.boards != nil, "", nil
}
func (r *memoryRepo) Members(_ context.Context, key string, limit uint32) (any, bool, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v := r.members[key]
	return limitValue(v, limit), v != nil, "", nil
}
func (r *memoryRepo) ReplaceBoards(_ context.Context, v any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.boards = v
	r.writes++
	return nil
}
func (r *memoryRepo) ReplaceMembers(_ context.Context, k string, v any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.members == nil {
		r.members = map[string]any{}
	}
	r.members[k] = v
	r.writes++
	return nil
}
func (r *memoryRepo) Publish(_ context.Context, b any, m map[string]any) error {
	r.boards = b
	r.members = m
	return nil
}
func (r *memoryRepo) Close() error { return nil }
func limitValue(v any, l uint32) any {
	list, ok := v.([]any)
	if ok && uint32(len(list)) > l {
		return list[:l]
	}
	return v
}

type fakeFetch struct {
	mu      sync.Mutex
	boards  any
	members map[string]any
	err     error
	calls   int
	block   chan struct{}
}

func (f *fakeFetch) Boards(context.Context) (any, error) {
	f.mu.Lock()
	f.calls++
	b, e := f.boards, f.err
	block := f.block
	f.mu.Unlock()
	if block != nil {
		<-block
	}
	return b, e
}
func (f *fakeFetch) Members(_ context.Context, k string) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.members[k], f.err
}

func TestServiceUsesCacheAndLimits(t *testing.T) {
	repo := &memoryRepo{boards: []any{1, 2}}
	fetch := &fakeFetch{}
	s := New(repo, fetch, time.Second, nil)
	v, err := s.Boards(context.Background(), 1)
	if err != nil || len(v.([]any)) != 1 || fetch.calls != 0 {
		t.Fatalf("v=%v err=%v calls=%d", v, err, fetch.calls)
	}
}
func TestServiceMissCoalescesAndDoesNotWriteFailures(t *testing.T) {
	repo := &memoryRepo{}
	fetch := &fakeFetch{boards: []any{"a"}, block: make(chan struct{})}
	s := New(repo, fetch, time.Second, nil)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = s.Boards(context.Background(), 10) }()
	}
	time.Sleep(20 * time.Millisecond)
	close(fetch.block)
	wg.Wait()
	if fetch.calls != 1 || repo.writes != 1 {
		t.Fatalf("calls=%d writes=%d", fetch.calls, repo.writes)
	}
	repo = &memoryRepo{}
	fetch = &fakeFetch{err: errors.New("upstream")}
	s = New(repo, fetch, time.Second, nil)
	if _, err := s.Boards(context.Background(), 10); err == nil || repo.writes != 0 {
		t.Fatalf("err=%v writes=%d", err, repo.writes)
	}
}
func TestNextTrigger(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	for _, tt := range []struct{ now, want string }{{"2026-08-14 08:59", "2026-08-14 09:00"}, {"2026-08-14 09:00", "2026-08-14 15:00"}, {"2026-08-14 20:00", "2026-08-15 09:00"}} {
		now, _ := time.ParseInLocation("2006-01-02 15:04", tt.now, loc)
		want, _ := time.ParseInLocation("2006-01-02 15:04", tt.want, loc)
		if got := nextTrigger(now); !got.Equal(want) {
			t.Fatalf("%s: %s", tt.now, got)
		}
	}
}
