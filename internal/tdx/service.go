package tdx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/bensema/gotdx"
	"github.com/resse/tdx-api/internal/config"
)

type Params struct {
	Market      uint8    `json:"-" form:"market"`
	MarketSet   bool     `json:"-" form:"-"`
	Code        string   `json:"-" form:"code"`
	Symbols     []Symbol `json:"symbols" form:"-"`
	Offset      uint32   `json:"-" form:"offset"`
	Limit       uint32   `json:"-" form:"limit"`
	LimitSet    bool     `json:"-" form:"-"`
	Date        uint32   `json:"-" form:"date"`
	Period      string   `json:"-" form:"period"`
	Adjust      string   `json:"-" form:"adjust"`
	BoardType   uint16   `json:"-" form:"-"`
	BoardSymbol string   `json:"-" form:"board_symbol"`
	Days        uint16   `json:"-" form:"days"`
}

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
	main, mac *gotdx.Client
	mu        sync.Mutex
}
type Service struct {
	pool    chan *clients
	all     []*clients
	retries int
	once    sync.Once
}

func New(c config.Config) *Service {
	s := &Service{pool: make(chan *clients, c.PoolSize), retries: c.RetryLimit}
	for range c.PoolSize {
		opts := options(c)
		b := &clients{main: gotdx.New(opts...), mac: gotdx.NewMAC(opts...)}
		s.all = append(s.all, b)
		s.pool <- b
	}
	return s
}

func options(c config.Config) []gotdx.Option {
	opts := []gotdx.Option{gotdx.WithTimeoutSec(int(c.UpstreamTimeout.Seconds())), gotdx.WithAutoSelectFastest(true)}
	add := func(hosts []string, primary func(string) gotdx.Option, pool func(...string) gotdx.Option) {
		if len(hosts) > 0 {
			opts = append(opts, primary(hosts[0]))
			if len(hosts) > 1 {
				opts = append(opts, pool(hosts[1:]...))
			}
		}
	}
	add(c.MainHosts, gotdx.WithTCPAddress, gotdx.WithTCPAddressPool)
	add(c.MACHosts, gotdx.WithMacTCPAddress, gotdx.WithMacTCPAddressPool)
	return opts
}

func (s *Service) Ready() bool { return s != nil && len(s.all) > 0 }
func (s *Service) Close() error {
	var first error
	s.once.Do(func() {
		for _, b := range s.all {
			for _, c := range []*gotdx.Client{b.main, b.mac} {
				if err := c.Disconnect(); err != nil && first == nil {
					first = err
				}
			}
		}
	})
	return first
}

func (s *Service) with(ctx context.Context, fn func(*clients) (any, error)) (any, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case b := <-s.pool:
		defer func() { s.pool <- b }()
		b.mu.Lock()
		defer b.mu.Unlock()
		return retry(ctx, s.retries, isRetryable, func() (any, error) {
			out, err := fn(b)
			if err != nil {
				_ = b.main.Disconnect()
				_ = b.mac.Disconnect()
			}
			return out, err
		})
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
	return s.with(ctx, func(c *clients) (any, error) { return call(c, op, p) })
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
		return m.StockKLine(periodValue(p.Period), p.Market, p.Code, uint16(p.Offset), uint16(p.Limit), 1, adjustValue(p.Adjust))
	case "stock.index.bars":
		return m.GetIndexBars(periodValue(p.Period), p.Market, p.Code, uint16(p.Offset), uint16(p.Limit))
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
		return mac.MACSymbolBars(p.Market, p.Code, periodValue(p.Period), 1, p.Offset, p.Limit, adjustValue(p.Adjust))
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
