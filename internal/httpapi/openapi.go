package httpapi

//go:generate go run ../../cmd/openapi -out ../../docs/openapi.json

import (
	"encoding/json"
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
)

func (s *Server) openapi(c *gin.Context) { c.JSON(http.StatusOK, OpenAPIDocument()) }

func OpenAPIDocument() map[string]any {
	paths := map[string]any{}
	for _, route := range stableRoutes {
		path := openAPIPath("/api/v1" + route.Path)
		methods, ok := paths[path].(map[string]any)
		if !ok {
			methods = map[string]any{}
			paths[path] = methods
		}
		operation := map[string]any{
			"summary":     route.Summary,
			"operationId": route.Operation,
			"responses": map[string]any{
				"200": map[string]any{"description": "请求成功", "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/SuccessEnvelope"}}}},
				"400": map[string]any{"description": "参数错误", "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/ErrorEnvelope"}}}},
				"502": map[string]any{"description": "上游不可用"},
				"504": map[string]any{"description": "上游超时"},
			},
		}
		if route.Method == http.MethodPost {
			operation["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/BatchSymbolsRequest"}}}}
		} else {
			operation["parameters"] = openAPIParameters(route)
		}
		methods[methodName(route.Method)] = operation
	}
	return map[string]any{
		"openapi": "3.0.3",
		"info":    map[string]any{"title": "沪深股票行情 API", "version": "1.0.0", "description": "证券代码使用 000001.SZ 或 600519.SH 格式；全市场接口使用 market=0（深圳）或 market=1（上海）。"},
		"paths":   paths,
		"components": map[string]any{"schemas": map[string]any{
			"BatchSymbolsRequest": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"symbols"}, "properties": map[string]any{"symbols": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"$ref": "#/components/schemas/Symbol"}}}},
			"Symbol":              map[string]any{"type": "object", "additionalProperties": false, "required": []string{"code"}, "properties": map[string]any{"code": codeSchema()}},
			"SuccessEnvelope":     map[string]any{"type": "object", "required": []string{"data"}, "properties": map[string]any{"data": map[string]any{"description": "仅包含沪深市场数据"}, "meta": map[string]any{"type": "object"}}},
			"ErrorEnvelope":       map[string]any{"type": "object", "required": []string{"error"}, "properties": map[string]any{"error": map[string]any{"type": "object", "required": []string{"code", "message"}, "properties": map[string]any{"code": map[string]any{"type": "string"}, "message": map[string]any{"type": "string"}, "request_id": map[string]any{"type": "string"}, "details": map[string]any{}}}}},
		}},
	}
}

func requestProperties() map[string]any {
	return map[string]any{
		"market": marketSchema(), "code": codeSchema(),
		"offset": integerSchema(0, 4294967295), "limit": integerSchema(1, 1000),
		"start_date": dateRangeSchema(), "end_date": dateRangeSchema(),
		"date":   map[string]any{"type": "integer", "minimum": 19900101, "maximum": 21001231, "description": "YYYYMMDD；部分实时接口可省略"},
		"period": map[string]any{"type": "string", "enum": []string{"1m", "5m", "15m", "30m", "1h", "daily", "weekly", "monthly", "quarterly", "yearly"}, "default": "daily"},
		"adjust": map[string]any{"type": "string", "enum": []string{"none", "forward", "backward"}, "default": "none"},
		"days":   integerSchema(1, 30), "board_symbol": boardSymbolSchema(),
	}
}

func marketSchema() map[string]any {
	return map[string]any{"type": "integer", "enum": []int{0, 1}, "description": "0=深圳，1=上海"}
}
func codeSchema() map[string]any {
	return map[string]any{"type": "string", "pattern": "^[0-9]{6}\\.(SH|SZ)$", "examples": []string{"000001.SZ", "600519.SH"}}
}
func boardSymbolSchema() map[string]any {
	return map[string]any{"type": "string", "pattern": "^[A-Za-z0-9._-]{1,32}$"}
}

func dateRangeSchema() map[string]any {
	return map[string]any{"type": "string", "pattern": "^(?:[0-9]{8}|[0-9]{14})$", "description": "日线及以上使用 YYYYMMDD，日线以下使用 YYYYMMDDhhmmss；包含起止时间，范围不得超过允许跨度"}
}

func integerSchema(min, max uint64) map[string]any {
	return map[string]any{"type": "integer", "minimum": min, "maximum": max}
}
func arraySchema(items map[string]any) map[string]any {
	return map[string]any{"type": "array", "items": items}
}
func openAPIPath(path string) string { return path }
func openAPIParameters(route route) []any {
	properties := requestProperties()
	out := make([]any, 0, 20)
	for _, name := range operationParameters(route.Operation) {
		out = append(out, map[string]any{"name": name, "in": "query", "required": requiredQueryParameter(route.Operation, name), "schema": properties[name]})
	}
	return out
}
func requiredQueryParameter(operation, name string) bool {
	required := map[string][]string{"stock.count": {"market"}, "stock.list": {"market"}, "stock.quote": {"code"}, "stock.kline": {"code", "start_date", "end_date"}, "stock.index.bars": {"code", "start_date", "end_date"}, "stock.tick": {"code"}, "stock.tick.history": {"code", "date"}, "stock.sampling": {"code"}, "stock.index.info": {"code"}, "stock.index.momentum": {"code"}, "stock.auction": {"code"}, "stock.unusual": {"market"}, "stock.volume-profile": {"code"}, "stock.transactions": {"code"}, "stock.orders.history": {"code", "date"}, "stock.transactions.history": {"code", "date"}, "company.finance": {"code"}, "company.xdxr": {"code"}, "company.f10": {"code"}, "mac.board.members": {"board_symbol"}, "mac.board.quotes": {"board_symbol"}, "mac.quote": {"code"}, "mac.transactions": {"code"}, "mac.auction": {"code"}, "mac.ticks": {"code", "days"}, "mac.symbol.info": {"code"}, "mac.capital-flow": {"code"}, "mac.monitor": {"market"}, "mac.symbol.boards": {"code"}, "mac.symbol.bars": {"code", "start_date", "end_date"}}
	return slices.Contains(required[operation], name)
}

func operationParameters(operation string) []string {
	params := map[string][]string{
		"stock.count": {"market"}, "stock.list": {"market", "offset", "limit"}, "stock.quote": {"code"}, "stock.kline": {"code", "period", "adjust", "start_date", "end_date"}, "stock.index.bars": {"code", "period", "adjust", "start_date", "end_date"}, "stock.tick": {"code", "limit"}, "stock.tick.history": {"code", "date"}, "stock.sampling": {"code"}, "stock.index.info": {"code"}, "stock.index.momentum": {"code"}, "stock.auction": {"code", "limit"}, "stock.unusual": {"market", "limit"}, "stock.volume-profile": {"code"}, "stock.transactions": {"code", "limit"}, "stock.orders.history": {"code", "date"}, "stock.transactions.history": {"code", "date", "limit"}, "company.finance": {"code"}, "company.xdxr": {"code"}, "company.f10": {"code"}, "mac.boards": {"limit"}, "mac.board.members": {"board_symbol", "limit"}, "mac.board.quotes": {"board_symbol", "limit"}, "mac.quote": {"code", "date"}, "mac.transactions": {"code", "date", "limit"}, "mac.auction": {"code", "limit"}, "mac.ticks": {"code", "date", "days"}, "mac.symbol.info": {"code"}, "mac.capital-flow": {"code"}, "mac.monitor": {"market", "limit"}, "mac.symbol.boards": {"code"}, "mac.symbol.bars": {"code", "period", "adjust", "start_date", "end_date"},
	}
	return params[operation]
}
func methodName(method string) string {
	if method == http.MethodGet {
		return "get"
	}
	if method == http.MethodPost {
		return "post"
	}
	return method
}
func docs(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><title>沪深股票 API 文档</title><link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css"></head><body><div id="swagger-ui"></div><script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script><script>SwaggerUIBundle({url:'/openapi.json',dom_id:'#swagger-ui',deepLinking:true})</script></body></html>`))
}
func StableRouteCount() int { return len(stableRoutes) }
func StableOperations() []string {
	out := make([]string, len(stableRoutes))
	for i, route := range stableRoutes {
		out[i] = route.Operation
	}
	return out
}
func OpenAPIJSON() ([]byte, error) { return json.MarshalIndent(OpenAPIDocument(), "", "  ") }
