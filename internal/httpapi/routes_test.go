package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/resse/tdx-api/internal/config"
	"github.com/resse/tdx-api/internal/tdx"
)

type fakeCaller struct {
	ready bool
	err   error
	calls int
	last  tdx.Params
}

func (f *fakeCaller) Call(_ context.Context, _ string, p tdx.Params) (any, error) {
	f.calls++
	f.last = p
	if f.err != nil {
		return nil, f.err
	}
	return map[string]any{"ok": true}, nil
}
func (f *fakeCaller) Ready() bool  { return f.ready }
func (f *fakeCaller) Close() error { return nil }
func testConfig() config.Config {
	return config.Config{HTTPAddr: ":8080", GinMode: gin.TestMode, CORSOrigins: []string{"*"}, CORSMethods: []string{"GET", "POST", "OPTIONS"}, CORSHeaders: []string{"Origin", "Content-Type"}, PoolSize: 1, UpstreamTimeout: 1000000000, MaxItems: 1000, ShutdownTimeout: 1000000000}
}

func TestHealthAndNotFound(t *testing.T) {
	f := &fakeCaller{ready: true}
	r := NewRouter(testConfig(), f)
	for _, tt := range []struct {
		path   string
		status int
	}{{"/health/live", 200}, {"/health/ready", 200}, {"/missing", 404}} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		r.ServeHTTP(w, req)
		if w.Code != tt.status {
			t.Fatalf("%s: %d %s", tt.path, w.Code, w.Body.String())
		}
		if w.Header().Get("X-Request-ID") == "" {
			t.Fatal("缺少请求 ID")
		}
	}
}

func TestReadinessUnavailable(t *testing.T) {
	r := NewRouter(testConfig(), &fakeCaller{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if w.Code != http.StatusBadGateway {
		t.Fatalf("状态码: %d", w.Code)
	}
}

func TestValidationPreventsUpstreamCall(t *testing.T) {
	f := &fakeCaller{ready: true}
	r := NewRouter(testConfig(), f)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/stocks?market=1&limit=0", nil))
	if w.Code != http.StatusBadRequest || f.calls != 0 {
		t.Fatalf("status=%d calls=%d", w.Code, f.calls)
	}
}

func TestCORS(t *testing.T) {
	r := NewRouter(testConfig(), &fakeCaller{ready: true})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/stocks", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	r.ServeHTTP(w, req)
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("CORS 错误: %d %v", w.Code, w.Header())
	}
}

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		err    error
		status int
	}{{context.DeadlineExceeded, 504}, {&APIError{Kind: KindValidation, Message: "bad"}, 400}, {errors.New("connect failed"), 502}}
	for _, tt := range tests {
		f := &fakeCaller{ready: true, err: tt.err}
		r := NewRouter(testConfig(), f)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/stocks/count?market=1", nil))
		if w.Code != tt.status {
			t.Fatalf("%v: %d %s", tt.err, w.Code, w.Body.String())
		}
	}
}

func TestOpenAPICoversStableRoutes(t *testing.T) {
	r := NewRouter(testConfig(), &fakeCaller{ready: true})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	operations := 0
	for _, methods := range doc.Paths {
		operations += len(methods)
	}
	if operations != StableRouteCount() {
		t.Fatalf("OpenAPI=%d routes=%d", operations, StableRouteCount())
	}
}

func TestOpenAPIUsesCodeQueryParameter(t *testing.T) {
	doc := OpenAPIDocument()
	paths := doc["paths"].(map[string]any)
	if _, ok := paths["/api/v1/stocks/auction"]; !ok {
		t.Fatal("OpenAPI 应使用不含 code 的固定路径")
	}
	for path := range paths {
		if strings.Contains(path, "{code}") || strings.Contains(path, ":code") {
			t.Fatalf("OpenAPI 路径不得包含 code: %s", path)
		}
	}
	operation := paths["/api/v1/stocks/auction"].(map[string]any)["get"].(map[string]any)
	parameters := operation["parameters"].([]any)
	required := map[string]bool{}
	for _, raw := range parameters {
		parameter := raw.(map[string]any)
		if parameter["required"] == true {
			required[parameter["name"].(string)] = true
		}
	}
	for _, name := range []string{"code"} {
		if !required[name] {
			t.Fatalf("竞价接口参数 %s 应标记为必填", name)
		}
	}
	if required["market"] {
		t.Fatal("按证券查询不应要求 market")
	}
}

func TestGeneratedOpenAPIIsCurrent(t *testing.T) {
	generated, err := os.ReadFile("../../docs/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	want, err := OpenAPIJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(generated) != string(append(want, '\n')) {
		t.Fatal("docs/openapi.json 未更新，请运行 go generate ./internal/httpapi")
	}
}

func TestStableOperationsAreUniqueAndExcluded(t *testing.T) {
	seen := map[string]bool{}
	for _, op := range StableOperations() {
		if seen[op] {
			t.Fatalf("重复操作: %s", op)
		}
		seen[op] = true
		for _, word := range []string{"goods", "icfqs", "experiment", "todo", "client26", "quotes2", "kline2", "withtrans"} {
			if strings.Contains(strings.ToLower(op), word) {
				t.Fatalf("稳定路由包含排除能力: %s", op)
			}
		}
	}
}

func TestExtendedMarketRouteNotFound(t *testing.T) {
	f := &fakeCaller{ready: true}
	r := NewRouter(testConfig(), f)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/extended/stocks/AU/quote?category=30", nil))
	if w.Code != http.StatusNotFound || f.calls != 0 {
		t.Fatalf("status=%d calls=%d", w.Code, f.calls)
	}
}

func TestOnlyShanghaiAndShenzhenCodeSuffixesAllowed(t *testing.T) {
	f := &fakeCaller{ready: true}
	r := NewRouter(testConfig(), f)
	for _, code := range []string{"600519.BJ", "00700.HK", "AAPL.US", "600519.sh", "600519"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/stocks/quote?code="+code, nil))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("code=%s status=%d", code, w.Code)
		}
	}
	if f.calls != 0 {
		t.Fatalf("非沪深请求不应调用上游，calls=%d", f.calls)
	}
}

func TestOpenAPIExcludesExtendedMarkets(t *testing.T) {
	for path := range OpenAPIDocument()["paths"].(map[string]any) {
		if strings.Contains(path, "/extended") {
			t.Fatalf("OpenAPI 包含扩展市场路由: %s", path)
		}
	}
}

func TestResponseFiltersNonMainlandMarkets(t *testing.T) {
	type item struct {
		Market uint8
		Code   string
	}
	got := filterMainlandMarkets([]item{{0, "000001"}, {1, "600519"}, {2, "920001"}, {3, "00700"}}).([]item)
	if len(got) != 2 || got[0].Market != 0 || got[1].Market != 1 {
		t.Fatalf("过滤结果错误: %+v", got)
	}
}

func TestCodeIsQueryParameterOnly(t *testing.T) {
	f := &fakeCaller{ready: true}
	r := NewRouter(testConfig(), f)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/stocks/auction?code=600519.SH&limit=10", nil))
	if w.Code != http.StatusOK || f.calls != 1 {
		t.Fatalf("query 参数请求 status=%d calls=%d", w.Code, f.calls)
	}
	if f.last.Market != 1 || f.last.Code != "600519" {
		t.Fatalf("代码未正确转换: %+v", f.last)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/stocks/600519/auction?market=1&limit=10", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("旧路径应返回 404，status=%d", w.Code)
	}
}

func TestBoardSymbolIsQueryParameterOnly(t *testing.T) {
	f := &fakeCaller{ready: true}
	r := NewRouter(testConfig(), f)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/mac/boards/members?board_symbol=BK0420&limit=10", nil))
	if w.Code != http.StatusOK || f.calls != 1 || f.last.BoardSymbol != "BK0420" {
		t.Fatalf("query 参数请求 status=%d calls=%d params=%+v", w.Code, f.calls, f.last)
	}

	for _, path := range []string{
		"/api/v1/mac/boards/members?limit=10",
		"/api/v1/mac/boards/members?board_symbol=%20&limit=10",
	} {
		w = httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d", path, w.Code)
		}
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/mac/boards/BK0420/members?limit=10", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("旧路径应返回 404，status=%d", w.Code)
	}
}

func TestOpenAPIUsesBoardSymbolQueryParameter(t *testing.T) {
	paths := OpenAPIDocument()["paths"].(map[string]any)
	if _, ok := paths["/api/v1/mac/boards/members"]; !ok {
		t.Fatal("OpenAPI 缺少固定板块成分路径")
	}
	for path := range paths {
		if strings.Contains(path, "board_symbol") {
			t.Fatalf("OpenAPI 路径不得包含 board_symbol: %s", path)
		}
	}
	operation := paths["/api/v1/mac/boards/members"].(map[string]any)["get"].(map[string]any)
	found := false
	for _, raw := range operation["parameters"].([]any) {
		parameter := raw.(map[string]any)
		if parameter["name"] == "board_symbol" {
			found = parameter["in"] == "query" && parameter["required"] == true
		}
	}
	if !found {
		t.Fatal("board_symbol 应是必填 query 参数")
	}
}

func TestRawFileRoutesAreNotExposed(t *testing.T) {
	f := &fakeCaller{ready: true}
	r := NewRouter(testConfig(), f)
	for _, path := range []string{"/api/v1/files/meta?filename=block.dat", "/api/v1/files/download?filename=block.dat", "/api/v1/mac/files/meta?filename=block.dat", "/api/v1/stocks/company/content?code=600519&market=1&filename=x&length=10"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d", path, w.Code)
		}
	}
	if f.calls != 0 {
		t.Fatalf("原始文件路由不应调用上游，calls=%d", f.calls)
	}
}

func TestProtocolParametersAreRejected(t *testing.T) {
	f := &fakeCaller{ready: true}
	r := NewRouter(testConfig(), f)
	for _, path := range []string{
		"/api/v1/stocks/auction?code=600519.SH&start=10",
		"/api/v1/stocks/bars?code=600519.SH&category=4",
		"/api/v1/mac/monitor?market=1&sort_type=6",
		"/api/v1/mac/symbols/bars?code=600519.SH&times=2",
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d", path, w.Code)
		}
	}
	if f.calls != 0 {
		t.Fatalf("不支持参数不应调用上游，calls=%d", f.calls)
	}
}

func TestBatchRequestRejectsUnknownFields(t *testing.T) {
	f := &fakeCaller{ready: true}
	r := NewRouter(testConfig(), f)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stocks/quotes", strings.NewReader(`{"symbols":[{"code":"600519.SH"}],"bitmap":[1]}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || f.calls != 0 {
		t.Fatalf("status=%d calls=%d", w.Code, f.calls)
	}
}

func TestBatchRequestRejectsLegacyMarketField(t *testing.T) {
	f := &fakeCaller{ready: true}
	r := NewRouter(testConfig(), f)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stocks/quotes", strings.NewReader(`{"symbols":[{"market":1,"code":"600519.SH"}]}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || f.calls != 0 {
		t.Fatalf("status=%d calls=%d", w.Code, f.calls)
	}
}

func TestCodeSuffixDerivesMarket(t *testing.T) {
	f := &fakeCaller{ready: true}
	r := NewRouter(testConfig(), f)
	for _, tt := range []struct {
		code   string
		market uint8
		plain  string
	}{{"000001.SZ", 0, "000001"}, {"600519.SH", 1, "600519"}} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/stocks/quote?code="+tt.code, nil))
		if w.Code != http.StatusOK || f.last.Market != tt.market || f.last.Code != tt.plain {
			t.Fatalf("code=%s status=%d params=%+v", tt.code, w.Code, f.last)
		}
	}
}

func TestSymbolRequestRejectsMarket(t *testing.T) {
	f := &fakeCaller{ready: true}
	r := NewRouter(testConfig(), f)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/stocks/quote?code=600519.SH&market=1", nil))
	if w.Code != http.StatusBadRequest || f.calls != 0 {
		t.Fatalf("status=%d calls=%d", w.Code, f.calls)
	}
}

func TestBatchCodesDeriveMarkets(t *testing.T) {
	f := &fakeCaller{ready: true}
	r := NewRouter(testConfig(), f)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stocks/quotes", strings.NewReader(`{"symbols":[{"code":"000001.SZ"},{"code":"600519.SH"}]}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || len(f.last.Symbols) != 2 || f.last.Symbols[0].Market != 0 || f.last.Symbols[0].Code != "000001" || f.last.Symbols[1].Market != 1 || f.last.Symbols[1].Code != "600519" {
		t.Fatalf("status=%d params=%+v", w.Code, f.last)
	}
}

func TestOpenAPIDoesNotExposeProtocolParameters(t *testing.T) {
	data, err := OpenAPIJSON()
	if err != nil {
		t.Fatal(err)
	}
	document := string(data)
	for _, name := range []string{`"start"`, `"count"`, `"category"`, `"times"`, `"sort_type"`, `"sort_order"`, `"filter"`, `"filters"`, `"bitmap"`, `"board_type"`} {
		if strings.Contains(document, name) {
			t.Fatalf("OpenAPI 暴露协议参数: %s", name)
		}
	}
	for _, operation := range []string{"stock.server", "stock.heartbeat", "mac.server", "mac.kline-offset", "mac.board.dynamic"} {
		if strings.Contains(document, operation) {
			t.Fatalf("OpenAPI 暴露运维接口: %s", operation)
		}
	}
}
