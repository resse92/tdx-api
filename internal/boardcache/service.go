package boardcache

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type Service struct {
	repo      Repository
	fetch     Fetcher
	timeout   time.Duration
	logger    *slog.Logger
	mu        sync.Mutex
	loads     map[string]*load
	refreshMu sync.Mutex
	stop      chan struct{}
	done      chan struct{}
	cancel    context.CancelFunc
	stopOnce  sync.Once
}
type load struct {
	done  chan struct{}
	value any
	err   error
}

func New(repo Repository, fetch Fetcher, timeout time.Duration, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: repo, fetch: fetch, timeout: timeout, logger: logger, loads: make(map[string]*load)}
}
func (s *Service) Boards(ctx context.Context, limit uint32) (any, error) {
	return s.cached(ctx, "boards", func(c context.Context) (any, bool, error) {
		v, loaded, _, err := s.repo.Boards(c, limit)
		return v, loaded, err
	}, func(c context.Context) (any, error) { return s.fetch.Boards(c) }, func(c context.Context, v any) error { return s.repo.ReplaceBoards(c, v) })
}
func (s *Service) Members(ctx context.Context, board string, limit uint32) (any, error) {
	return s.cached(ctx, "members:"+board, func(c context.Context) (any, bool, error) {
		v, loaded, _, err := s.repo.Members(c, board, limit)
		return v, loaded, err
	}, func(c context.Context) (any, error) { return s.fetch.Members(c, board) }, func(c context.Context, v any) error { return s.repo.ReplaceMembers(c, board, v) })
}
func (s *Service) cached(ctx context.Context, key string, read func(context.Context) (any, bool, error), fetch func(context.Context) (any, error), write func(context.Context, any) error) (any, error) {
	v, loaded, err := read(ctx)
	if err != nil {
		return nil, err
	}
	if loaded {
		return v, nil
	}
	s.mu.Lock()
	if current := s.loads[key]; current != nil {
		s.mu.Unlock()
		select {
		case <-current.done:
			return current.value, current.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	l := &load{done: make(chan struct{})}
	s.loads[key] = l
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.loads, key); close(l.done); s.mu.Unlock() }()
	shared, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	v, err = fetch(shared)
	if err == nil {
		err = write(shared, v)
	}
	l.value, l.err = v, err
	return v, err
}
func (s *Service) Refresh(ctx context.Context) error {
	if !s.refreshMu.TryLock() {
		s.logger.Info("板块刷新跳过", "reason", "overlap")
		return nil
	}
	defer s.refreshMu.Unlock()
	started := time.Now()
	s.logger.Info("板块刷新开始")
	c, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	boards, err := s.fetch.Boards(c)
	if err != nil {
		s.logger.Error("板块刷新失败", "stage", "boards", "error", err, "duration", time.Since(started))
		return err
	}
	list, ok := boards.([]any)
	if !ok {
		return fmt.Errorf("板块列表响应格式错误")
	}
	members := make(map[string]any, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		code, _ := m["code"].(string)
		if code == "" {
			code, _ = m["Code"].(string)
		}
		if code == "" {
			continue
		}
		value, e := s.fetch.Members(c, code)
		if e != nil {
			s.logger.Error("板块刷新失败", "stage", "members", "board_symbol", code, "error", e)
			return e
		}
		members[code] = value
	}
	if err := s.repo.Publish(c, boards, members); err != nil {
		s.logger.Error("板块刷新失败", "stage", "publish", "error", err)
		return err
	}
	s.logger.Info("板块刷新成功", "duration", time.Since(started))
	return nil
}
func (s *Service) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	go s.loop(ctx)
}
func (s *Service) loop(ctx context.Context) {
	defer close(s.done)
	loc := time.FixedZone("Asia/Shanghai", 8*60*60)

	for {
		now := time.Now().In(loc)
		next := nextTrigger(now)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-timer.C:
			_ = s.Refresh(ctx)
		case <-s.stop:
			timer.Stop()
			return
		case <-ctx.Done():
			timer.Stop()
			return
		}
	}
}
func nextTrigger(now time.Time) time.Time {
	loc := now.Location()
	for _, hour := range []int{9, 15, 20} {
		candidate := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, loc)
		if candidate.After(now) {
			return candidate
		}
	}
	tomorrow := now.AddDate(0, 0, 1)
	return time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 9, 0, 0, 0, loc)
}
func (s *Service) Stop(ctx context.Context) error {
	if s.stop != nil {
		s.stopOnce.Do(func() {
			close(s.stop)
			if s.cancel != nil {
				s.cancel()
			}
		})
		select {
		case <-s.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
func (s *Service) Close() error { return s.repo.Close() }
