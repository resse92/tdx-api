package tdx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bensema/gotdx"
	"github.com/resse/tdx-api/internal/config"
)

type Params struct {
	Market            uint8    `json:"-" form:"market"`
	MarketSet         bool     `json:"-" form:"-"`
	Code              string   `json:"-" form:"code"`
	Symbols           []Symbol `json:"symbols" form:"-"`
	Offset            uint32   `json:"-" form:"offset"`
	Limit             uint32   `json:"-" form:"limit"`
	LimitSet          bool     `json:"-" form:"-"`
	StartDate         uint64   `json:"-" form:"-"`
	EndDate           uint64   `json:"-" form:"-"`
	StartDateSetValue bool     `json:"-" form:"-"`
	EndDateSetValue   bool     `json:"-" form:"-"`
	StartDateText     string   `json:"-" form:"start_date"`
	EndDateText       string   `json:"-" form:"end_date"`
	Date              uint32   `json:"-" form:"date"`
	Period            string   `json:"-" form:"period"`
	Adjust            string   `json:"-" form:"adjust"`
	BoardType         uint16   `json:"-" form:"-"`
	BoardSymbol       string   `json:"-" form:"board_symbol"`
	Days              uint16   `json:"-" form:"days"`
}

func (p Params) StartDateSet() bool { return p.StartDateSetValue }
func (p Params) EndDateSet() bool   { return p.EndDateSetValue }

type Symbol struct {
	Market uint8  `json:"-"`
	Code   string `json:"code"`
}

func (p Params) pairs() ([]uint8, []string) {
	markets := make([]uint8, len(p.Symbols))
	codes := make([]string, len(p.Symbols))
	for i, s := range p.Symbols {
		markets[i] = s.Market
		codes[i] = s.Code
	}
	return markets, codes
}

type Caller interface {
	Call(context.Context, string, Params) (any, error)
	Ready() bool
	Close() error
}

type clients struct {
	main, mac                     *gotdx.Client
	connectMain, connectMAC       func() error
	disconnectMain, disconnectMAC func() error
	invoke                        func(string, Params) (any, error)
	mainAddress, macAddress       func() string
	logger                        *slog.Logger
	group                         int
	mainReady, macReady           atomic.Bool
	mu                            sync.Mutex
}
type Service struct {
	pool      chan *clients
	all       []*clients
	retries   int
	lifecycle sync.RWMutex
	closed    bool
	once      sync.Once
}

type clientFactory func([]gotdx.Option) *clients

func New(c config.Config) (*Service, error) {
	return newService(c, slog.Default(), newClients)
}

func newService(c config.Config, logger *slog.Logger, factory clientFactory) (*Service, error) {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Service{pool: make(chan *clients, c.PoolSize), retries: c.RetryLimit}
	opts := options(c)
	for i := range c.PoolSize {
		b := factory(opts)
		b.logger = logger
		b.group = i + 1
		s.all = append(s.all, b)
		if err := b.ensureMain("startup"); err != nil {
			_ = s.closeClients()
			return nil, fmt.Errorf("初始化第 %d 个主市场连接: %w", i+1, err)
		}
		if err := b.ensureMAC("startup"); err != nil {
			_ = s.closeClients()
			return nil, fmt.Errorf("初始化第 %d 个 MAC 连接: %w", i+1, err)
		}
		s.pool <- b
	}
	return s, nil
}

func newClients(opts []gotdx.Option) *clients {
	main, mac := gotdx.New(opts...), gotdx.NewMAC(opts...)
	b := &clients{main: main, mac: mac}
	b.connectMain = func() error { _, err := main.Connect(); return err }
	b.connectMAC = mac.ConnectMAC
	b.disconnectMain = main.Disconnect
	b.disconnectMAC = mac.Disconnect
	b.mainAddress = main.CurrentAddress
	b.macAddress = mac.CurrentAddress
	b.invoke = func(op string, p Params) (any, error) { return call(b, op, p) }
	return b
}

func options(c config.Config) []gotdx.Option {
	opts := []gotdx.Option{gotdx.WithTimeoutSec(int(c.UpstreamTimeout.Seconds())), gotdx.WithAutoSelectFastest(true)}
	add := func(hosts []string, primary func(string) gotdx.Option, pool func(...string) gotdx.Option) {
		if len(hosts) > 0 {
			opts = append(opts, primary(hosts[0]))
			opts = append(opts, pool(hosts[1:]...))
		}
	}
	add(c.MainHosts, gotdx.WithTCPAddress, gotdx.WithTCPAddressPool)
	add(c.MACHosts, gotdx.WithMacTCPAddress, gotdx.WithMacTCPAddressPool)
	return opts
}

func (s *Service) Ready() bool {
	if s == nil {
		return false
	}
	s.lifecycle.RLock()
	defer s.lifecycle.RUnlock()
	if s.closed || len(s.all) == 0 {
		return false
	}
	for _, b := range s.all {
		if !b.mainReady.Load() || !b.macReady.Load() {
			return false
		}
	}
	return true
}

func (s *Service) Close() error {
	if s == nil {
		return nil
	}
	var first error
	s.once.Do(func() {
		s.lifecycle.Lock()
		defer s.lifecycle.Unlock()
		s.closed = true
		first = s.closeClients()
	})
	return first
}

func (s *Service) closeClients() error {
	var first error
	for _, b := range s.all {
		if err := b.disconnectMainClient("shutdown", nil); err != nil && first == nil {
			first = err
		}
		if err := b.disconnectMACClient("shutdown", nil); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (s *Service) with(ctx context.Context, fn func(*clients) (any, error)) (any, error) {
	s.lifecycle.RLock()
	defer s.lifecycle.RUnlock()
	if s.closed {
		return nil, errors.New("TDX 服务已关闭")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case b := <-s.pool:
		defer func() { s.pool <- b }()
		b.mu.Lock()
		defer b.mu.Unlock()
		return retry(ctx, s.retries, isRetryable, func() (any, error) { return fn(b) })
	}
}

func retry(ctx context.Context, retries int, retryable func(error) bool, fn func() (any, error)) (any, error) {
	var out any
	var err error
	for attempt := 0; attempt <= retries; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		out, err = fn()
		if err == nil {
			return out, nil
		}
		if !retryable(err) {
			return out, err
		}
	}
	return nil, err
}

func isRetryable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "connect") || strings.Contains(message, "connection") || strings.Contains(message, "broken pipe") || strings.Contains(message, "closed")
}

func (s *Service) Call(ctx context.Context, op string, p Params) (any, error) {
	mac := strings.HasPrefix(op, "mac.")
	return s.with(ctx, func(c *clients) (any, error) {
		if mac {
			if err := c.ensureMAC("request"); err != nil {
				return nil, err
			}
		} else if err := c.ensureMain("request"); err != nil {
			return nil, err
		}
		out, err := c.invoke(op, p)
		if err != nil && isRetryable(err) {
			if mac {
				_ = c.disconnectMACClient("protocol_error", err)
			} else {
				_ = c.disconnectMainClient("protocol_error", err)
			}
		}
		return out, err
	})
}

func (c *clients) ensureMain(trigger string) error {
	if c.mainReady.Load() {
		return nil
	}
	c.logConnectAttempt("main", trigger)
	if err := c.connectMain(); err != nil {
		c.logConnectFailure("main", trigger, err)
		return err
	}
	c.mainReady.Store(true)
	c.logConnectSuccess("main", trigger, c.address(c.mainAddress))
	return nil
}

func (c *clients) ensureMAC(trigger string) error {
	if c.macReady.Load() {
		return nil
	}
	c.logConnectAttempt("mac", trigger)
	if err := c.connectMAC(); err != nil {
		c.logConnectFailure("mac", trigger, err)
		return err
	}
	c.macReady.Store(true)
	c.logConnectSuccess("mac", trigger, c.address(c.macAddress))
	return nil
}

func (c *clients) disconnectMainClient(reason string, cause error) error {
	if c.mainReady.Swap(false) {
		c.logDisconnect("main", reason, cause)
	}
	return c.disconnectMain()
}

func (c *clients) disconnectMACClient(reason string, cause error) error {
	if c.macReady.Swap(false) {
		c.logDisconnect("mac", reason, cause)
	}
	return c.disconnectMAC()
}

func (c *clients) logConnectAttempt(protocol, trigger string) {
	c.logger.Info("TDX 连接中", "protocol", protocol, "client_group", c.group, "trigger", trigger)
}

func (c *clients) logConnectSuccess(protocol, trigger, address string) {
	c.logger.Info("TDX 连接成功", "protocol", protocol, "client_group", c.group, "trigger", trigger, "address", address)
}

func (c *clients) logConnectFailure(protocol, trigger string, err error) {
	c.logger.Error("TDX 连接失败", "protocol", protocol, "client_group", c.group, "trigger", trigger, "error", err)
}

func (c *clients) logDisconnect(protocol, reason string, cause error) {
	args := []any{"protocol", protocol, "client_group", c.group, "reason", reason}
	if cause != nil {
		args = append(args, "error", cause)
	}
	c.logger.Info("TDX 连接断开", args...)
}

func (c *clients) address(get func() string) string {
	if get == nil {
		return ""
	}
	return get()
}
func call(c *clients, op string, p Params) (any, error) {
	m := c.main
	switch op {
	case "stock.count":
		return m.StockCount(p.Market)
	case "stock.list":
		return m.StockList(p.Market, p.Offset, p.Limit)
	case "stock.quotes":
		markets, codes := p.pairs()
		return m.StockQuotesDetail(markets, codes)
	case "stock.quote":
		return m.StockQuotes([]uint8{p.Market}, []string{p.Code})
	case "stock.kline":
		offset, limit, err := PlanBars(p.StartDate, p.EndDate, p.Period, time.Now())
		if err != nil {
			return nil, err
		}
		return m.StockKLine(periodValue(p.Period), p.Market, p.Code, uint16(offset), uint16(limit), 1, adjustValue(p.Adjust))
	case "stock.index.bars":
		offset, limit, err := PlanBars(p.StartDate, p.EndDate, p.Period, time.Now())
		if err != nil {
			return nil, err
		}
		return m.GetIndexBars(periodValue(p.Period), p.Market, p.Code, uint16(offset), uint16(limit))
	case "stock.tick":
		return m.StockTickChart(p.Market, p.Code, 0, uint16(p.Limit))
	case "stock.tick.history":
		return m.StockHistoryTickChart(p.Date, p.Market, p.Code)
	case "stock.sampling":
		return m.StockChartSampling(p.Market, p.Code)
	case "stock.index.info":
		return m.StockIndexInfo(p.Market, p.Code)
	case "stock.index.momentum":
		return m.StockIndexMomentum(p.Market, p.Code)
	case "stock.auction":
		return m.StockAuction(p.Market, p.Code, 0, p.Limit)
	case "stock.unusual":
		return m.StockUnusual(p.Market, 0, p.Limit)
	case "stock.volume-profile":
		return m.StockVolumeProfile(p.Market, p.Code)
	case "stock.transactions":
		return m.StockTransaction(p.Market, p.Code, 0, uint16(p.Limit))
	case "stock.orders.history":
		return m.StockHistoryOrders(p.Date, p.Market, p.Code)
	case "stock.transactions.history":
		return m.StockHistoryTransaction(p.Date, p.Market, p.Code, 0, uint16(p.Limit))
	case "stock.exchange-announcement":
		return m.GetExchangeAnnouncement()
	case "stock.announcement":
		return m.GetAnnouncement()
	case "company.finance":
		return m.GetFinanceInfo(p.Market, p.Code)
	case "company.xdxr":
		return m.GetXDXRInfo(p.Market, p.Code)
	case "company.f10":
		return m.StockF10(p.Market, p.Code)
	}
	mac := c.mac
	switch op {
	case "mac.boards":
		return mac.MACBoardList(p.BoardType, p.Limit)
	case "mac.board.members":
		return mac.MACBoardMembers(p.BoardSymbol, p.Limit)
	case "mac.board.quotes":
		return mac.MACBoardMembersQuotes(p.BoardSymbol, p.Limit)
	case "mac.quote":
		return mac.MACQuotesWithDate(p.Market, p.Code, p.Date)
	case "mac.transactions":
		return mac.MACTransactionsWithDate(p.Market, p.Code, 0, p.Limit, p.Date)
	case "mac.auction":
		return mac.MACAuction(p.Market, p.Code, 0, p.Limit)
	case "mac.ticks":
		return mac.MACTickCharts(p.Market, p.Code, p.Date, p.Days)
	case "mac.symbol.info":
		return mac.MACSymbolInfo(p.Market, p.Code)
	case "mac.capital-flow":
		return mac.MACCapitalFlow(p.Market, p.Code)
	case "mac.monitor":
		return mac.MACMarketMonitor(p.Market, 0, p.Limit)
	case "mac.symbol.boards":
		return mac.MACSymbolBelongBoard(p.Code, p.Market)
	case "mac.symbol.bars":
		offset, limit, err := PlanBars(p.StartDate, p.EndDate, p.Period, time.Now())
		if err != nil {
			return nil, err
		}
		return mac.MACSymbolBars(p.Market, p.Code, periodValue(p.Period), 1, offset, limit, adjustValue(p.Adjust))
	default:
		return nil, fmt.Errorf("未知操作: %s", op)
	}
}

func periodValue(value string) uint16 {
	switch value {
	case "5m":
		return 0
	case "15m":
		return 1
	case "30m":
		return 2
	case "1h":
		return 3
	case "weekly":
		return 5
	case "monthly":
		return 6
	case "1m":
		return 8
	case "quarterly":
		return 10
	case "yearly":
		return 11
	default:
		return 4
	}
}
func adjustValue(value string) uint16 {
	if value == "forward" {
		return 1
	}
	if value == "backward" {
		return 2
	}
	return 0
}
