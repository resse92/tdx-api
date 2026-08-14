package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/resse/tdx-api/internal/boardcache"
	"github.com/resse/tdx-api/internal/config"
	"github.com/resse/tdx-api/internal/tdx"
)

type route struct {
	Method, Path, Operation, Summary string
	Validate                         func(tdx.Params, config.Config) error
}

var stableRoutes = []route{
	{http.MethodGet, "/stocks/count", "stock.count", "查询主市场证券数量", requireMarket},
	{http.MethodGet, "/stocks", "stock.list", "查询主市场证券列表", validateList},
	{http.MethodPost, "/stocks/quotes", "stock.quotes", "批量查询主市场行情", validatePairs},
	{http.MethodGet, "/stocks/quote", "stock.quote", "查询单个主市场行情", requireSymbol},
	{http.MethodGet, "/stocks/bars", "stock.kline", "查询股票 K 线", validateBars},
	{http.MethodGet, "/stocks/index-bars", "stock.index.bars", "查询指数 K 线", validateBars},
	{http.MethodGet, "/stocks/ticks", "stock.tick", "查询当日分时", validateSymbolLimit16},
	{http.MethodGet, "/stocks/ticks/history", "stock.tick.history", "查询历史分时", validateDatedSymbol},
	{http.MethodGet, "/stocks/sampling", "stock.sampling", "查询分时采样", requireSymbol},
	{http.MethodGet, "/stocks/index-info", "stock.index.info", "查询指数信息", requireSymbol},
	{http.MethodGet, "/stocks/index-momentum", "stock.index.momentum", "查询指数动量", requireSymbol},
	{http.MethodGet, "/stocks/auction", "stock.auction", "查询集合竞价", validateSymbolLimit},
	{http.MethodGet, "/stocks/unusual", "stock.unusual", "查询市场异动", validateList},
	{http.MethodGet, "/stocks/volume-profile", "stock.volume-profile", "查询筹码分布", requireSymbol},
	{http.MethodGet, "/stocks/transactions", "stock.transactions", "查询当日成交", validateSymbolLimit16},
	{http.MethodGet, "/stocks/orders/history", "stock.orders.history", "查询历史委托", validateDatedSymbol},
	{http.MethodGet, "/stocks/transactions/history", "stock.transactions.history", "查询历史成交", validateDatedSymbolLimit16},
	{http.MethodGet, "/stocks/exchange-announcement", "stock.exchange-announcement", "查询交易所公告", nil},
	{http.MethodGet, "/stocks/announcement", "stock.announcement", "查询公告", nil},
	{http.MethodGet, "/stocks/finance", "company.finance", "查询财务信息", requireSymbol},
	{http.MethodGet, "/stocks/xdxr", "company.xdxr", "查询除权除息", requireSymbol},
	{http.MethodGet, "/stocks/f10", "company.f10", "查询组合 F10", requireSymbol},
	{http.MethodGet, "/mac/boards", "mac.boards", "查询缓存优先的 MAC 板块列表", validateLimit},
	{http.MethodGet, "/mac/boards/members", "mac.board.members", "查询缓存优先的 MAC 板块成分", validateBoardLimit},
	{http.MethodGet, "/mac/boards/quotes", "mac.board.quotes", "查询 MAC 板块成分行情", validateBoardLimit},
	{http.MethodGet, "/mac/symbols/quote", "mac.quote", "查询 MAC 股票快照", requireSymbol},
	{http.MethodGet, "/mac/symbols/transactions", "mac.transactions", "查询 MAC 成交", validateSymbolLimit},
	{http.MethodGet, "/mac/symbols/auction", "mac.auction", "查询 MAC 集合竞价", validateSymbolLimit},
	{http.MethodGet, "/mac/symbols/ticks", "mac.ticks", "查询 MAC 多日分时", validateMACTicks},
	{http.MethodGet, "/mac/symbols/info", "mac.symbol.info", "查询 MAC 股票摘要", requireSymbol},
	{http.MethodGet, "/mac/symbols/capital-flow", "mac.capital-flow", "查询 MAC 资金流向", requireSymbol},
	{http.MethodGet, "/mac/monitor", "mac.monitor", "查询 MAC 市场监控", validateList},
	{http.MethodGet, "/mac/symbols/boards", "mac.symbol.boards", "查询 MAC 证券所属板块", requireSymbol},
	{http.MethodGet, "/mac/symbols/bars", "mac.symbol.bars", "查询 MAC 统一 K 线", validateBars},
}

type Server struct {
	cfg    config.Config
	caller tdx.Caller
	cache  *boardcache.Service
}

func NewRouter(cfg config.Config, caller tdx.Caller, caches ...*boardcache.Service) *gin.Engine {
	gin.SetMode(cfg.GinMode)
	r := gin.New()
	r.Use(requestIDMiddleware(), loggingMiddleware(), recoveryMiddleware(), corsMiddleware(cfg))
	var cache *boardcache.Service
	if len(caches) > 0 {
		cache = caches[0]
	}
	s := &Server{cfg: cfg, caller: caller, cache: cache}
	r.GET("/health/live", func(c *gin.Context) { writeData(c, http.StatusOK, gin.H{"status": "alive"}) })
	r.GET("/health/ready", func(c *gin.Context) {
		if caller == nil || !caller.Ready() {
			writeError(c, &APIError{Kind: KindUnavailable, Message: "TDX 客户端未就绪"})
			return
		}
		writeData(c, http.StatusOK, gin.H{"status": "ready"})
	})
	r.GET("/openapi.json", s.openapi)
	r.GET("/docs", docs)
	v1 := r.Group("/api/v1")
	for _, rt := range stableRoutes {
		rt := rt
		v1.Handle(rt.Method, rt.Path, s.handle(rt))
	}
	r.NoRoute(func(c *gin.Context) { writeError(c, &APIError{Kind: KindNotFound, Message: "接口不存在"}) })
	return r
}

func (s *Server) handle(rt route) gin.HandlerFunc {
	return func(c *gin.Context) {
		var p tdx.Params
		if err := validateQueryParameters(c, rt); err != nil {
			writeError(c, err)
			return
		}
		if err := bindParams(c, &p); err != nil {
			writeError(c, &APIError{Kind: KindValidation, Message: "请求参数格式错误", Details: err.Error()})
			return
		}
		_, p.MarketSet = c.GetQuery("market")
		_, p.LimitSet = c.GetQuery("limit")
		if err := normalizeCodes(&p); err != nil {
			writeError(c, err)
			return
		}
		applyDefaults(rt.Operation, &p)
		if rt.Validate != nil {
			if err := rt.Validate(p, s.cfg); err != nil {
				writeError(c, err)
				return
			}
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), s.cfg.UpstreamTimeout)
		defer cancel()
		var data any
		var err error
		if s.cache != nil && rt.Operation == "mac.boards" {
			data, err = s.cache.Boards(ctx, p.Limit)
		} else if s.cache != nil && rt.Operation == "mac.board.members" {
			data, err = s.cache.Members(ctx, p.BoardSymbol, p.Limit)
		} else {
			data, err = s.caller.Call(ctx, rt.Operation, p)
		}
		if err != nil {
			writeError(c, err)
			return
		}
		writeData(c, http.StatusOK, filterMainlandMarkets(data))
	}
}

func validation(message string) *APIError { return &APIError{Kind: KindValidation, Message: message} }

var (
	codeRE        = regexp.MustCompile(`^([0-9]{6})\.(SH|SZ)$`)
	plainCodeRE   = regexp.MustCompile(`^[0-9]{6}$`)
	boardSymbolRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,32}$`)
)

func requireMarket(p tdx.Params, _ config.Config) error {
	if !p.MarketSet {
		return validation("market 为必填参数")
	}
	if p.Market > 1 {
		return validation("market 只支持 0（深圳）或 1（上海）")
	}
	return nil
}
func requireSymbol(p tdx.Params, _ config.Config) error {
	if !p.MarketSet || !plainCodeRE.MatchString(p.Code) {
		return validation("code 必须使用 000001.SZ 或 600519.SH 格式")
	}
	return nil
}
func validateLimit(p tdx.Params, c config.Config) error {
	if p.Limit == 0 || p.Limit > c.MaxItems {
		return validation("limit 超出允许范围")
	}
	return nil
}
func validateList(p tdx.Params, c config.Config) error {
	if err := requireMarket(p, c); err != nil {
		return err
	}
	return validateLimit(p, c)
}
func validateSymbolLimit(p tdx.Params, c config.Config) error {
	if err := requireSymbol(p, c); err != nil {
		return err
	}
	return validateLimit(p, c)
}
func validateLimit16(p tdx.Params, c config.Config) error {
	if err := validateLimit(p, c); err != nil {
		return err
	}
	if p.Limit > 65535 {
		return validation("limit 超出范围")
	}
	return nil
}
func validateSymbolLimit16(p tdx.Params, c config.Config) error {
	if err := requireSymbol(p, c); err != nil {
		return err
	}
	return validateLimit16(p, c)
}
func validDate(v uint32) bool {
	return v >= 19900101 && v <= 21001231 && v%100 >= 1 && v%100 <= 31 && (v/100)%100 >= 1 && (v/100)%100 <= 12
}
func validateDatedSymbol(p tdx.Params, c config.Config) error {
	if err := requireSymbol(p, c); err != nil {
		return err
	}
	if !validDate(p.Date) {
		return validation("date 必须是有效的 YYYYMMDD")
	}
	return nil
}
func validateDatedSymbolLimit16(p tdx.Params, c config.Config) error {
	if err := validateDatedSymbol(p, c); err != nil {
		return err
	}
	return validateLimit16(p, c)
}
func validatePairs(p tdx.Params, c config.Config) error {
	if len(p.Symbols) == 0 || uint32(len(p.Symbols)) > c.MaxItems {
		return validation("symbols 必须非空且不超过上限")
	}
	for _, symbol := range p.Symbols {
		if symbol.Market > 1 || !plainCodeRE.MatchString(symbol.Code) {
			return validation("symbols 中的 code 必须使用 000001.SZ 或 600519.SH 格式")
		}
	}
	return nil
}
func validateBoardLimit(p tdx.Params, c config.Config) error {
	if !boardSymbolRE.MatchString(p.BoardSymbol) {
		return validation("board_symbol 为必填参数且格式必须有效")
	}
	return validateLimit(p, c)
}

func validateBars(p tdx.Params, c config.Config) error {
	if err := requireSymbol(p, c); err != nil {
		return err
	}
	if err := validateLimit16(p, c); err != nil {
		return err
	}
	validPeriods := map[string]bool{"1m": true, "5m": true, "15m": true, "30m": true, "1h": true, "daily": true, "weekly": true, "monthly": true, "quarterly": true, "yearly": true}
	if !validPeriods[p.Period] {
		return validation("period 格式错误")
	}
	if p.Adjust != "none" && p.Adjust != "forward" && p.Adjust != "backward" {
		return validation("adjust 格式错误")
	}
	return nil
}
func applyDefaults(operation string, p *tdx.Params) {
	if !p.LimitSet {
		defaults := map[string]uint32{"stock.list": 200, "stock.kline": 200, "stock.index.bars": 200, "stock.tick": 1000, "stock.auction": 500, "stock.unusual": 200, "stock.transactions": 600, "stock.transactions.history": 600, "mac.boards": 500, "mac.board.members": 500, "mac.board.quotes": 500, "mac.transactions": 500, "mac.auction": 500, "mac.monitor": 200, "mac.symbol.bars": 200}
		p.Limit = defaults[operation]
	}
	if p.Period == "" {
		p.Period = "daily"
	}
	if p.Adjust == "" {
		p.Adjust = "none"
	}
}

func validateQueryParameters(c *gin.Context, route route) error {
	allowed := map[string]bool{}
	for _, name := range operationParameters(route.Operation) {
		allowed[name] = true
	}
	for name := range c.Request.URL.Query() {
		if !allowed[name] {
			return validation("不支持的参数: " + name)
		}
	}
	return nil
}

func bindParams(c *gin.Context, params *tdx.Params) error {
	if c.Request.Method != http.MethodPost {
		return c.ShouldBindQuery(params)
	}
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(params); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("请求体只能包含一个 JSON 对象")
	}
	return nil
}

func normalizeCodes(params *tdx.Params) error {
	if params.Code != "" {
		market, code, err := parseCode(params.Code)
		if err != nil {
			return err
		}
		params.Market = market
		params.MarketSet = true
		params.Code = code
	}
	for i := range params.Symbols {
		market, code, err := parseCode(params.Symbols[i].Code)
		if err != nil {
			return validation("symbols 中的 code 必须使用 000001.SZ 或 600519.SH 格式")
		}
		params.Symbols[i].Market = market
		params.Symbols[i].Code = code
	}
	return nil
}

func parseCode(value string) (uint8, string, error) {
	match := codeRE.FindStringSubmatch(value)
	if match == nil {
		return 0, "", validation("code 必须使用 000001.SZ 或 600519.SH 格式")
	}
	if match[2] == "SH" {
		return 1, match[1], nil
	}
	return 0, match[1], nil
}
func validateMACTicks(p tdx.Params, c config.Config) error {
	if err := requireSymbol(p, c); err != nil {
		return err
	}
	if p.Date != 0 && !validDate(p.Date) {
		return validation("date 必须为 0 或有效的 YYYYMMDD")
	}
	if p.Days == 0 || p.Days > 30 {
		return validation("days 必须在 1 到 30 之间")
	}
	return nil
}

func filterMainlandMarkets(data any) any {
	return filterMarketValue(reflect.ValueOf(data)).Interface()
}

func filterMarketValue(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return value
		}
		copy := reflect.New(value.Elem().Type())
		copy.Elem().Set(value.Elem())
		copy.Elem().Set(filterMarketValue(copy.Elem()))
		return copy
	}
	if value.Kind() == reflect.Slice {
		out := reflect.MakeSlice(value.Type(), 0, value.Len())
		for i := 0; i < value.Len(); i++ {
			item := value.Index(i)
			candidate := item
			if candidate.Kind() == reflect.Pointer && !candidate.IsNil() {
				candidate = candidate.Elem()
			}
			if candidate.Kind() == reflect.Struct {
				market := candidate.FieldByName("Market")
				if market.IsValid() && market.CanUint() && market.Uint() > 1 {
					continue
				}
			}
			out = reflect.Append(out, filterMarketValue(item))
		}
		return out
	}
	if value.Kind() == reflect.Struct {
		out := reflect.New(value.Type()).Elem()
		out.Set(value)
		for i := 0; i < value.NumField(); i++ {
			if out.Field(i).CanSet() && (value.Field(i).Kind() == reflect.Slice || value.Field(i).Kind() == reflect.Pointer) {
				out.Field(i).Set(filterMarketValue(value.Field(i)))
			}
		}
		return out
	}
	return value
}
