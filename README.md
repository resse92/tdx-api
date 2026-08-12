# tdx-api

基于 Gin 和 [`github.com/bensema/gotdx`](https://github.com/bensema/gotdx) 的版本化沪深股票行情 HTTP API。服务只提供上海与深圳市场数据、F10 和 MAC 板块及股票业务能力，默认允许跨域访问。

## 快速启动

要求 Go 1.26 或更高版本：

```bash
go run ./cmd/api
curl http://127.0.0.1:8080/health/ready
```

API 前缀为 `/api/v1`，OpenAPI 文档位于 `/openapi.json`，简易文档入口位于 `/docs`。

## 调用示例

```bash
# 上海市场证券列表
curl 'http://127.0.0.1:8080/api/v1/stocks?market=1&offset=0&limit=20'

# 批量详细行情
curl -X POST 'http://127.0.0.1:8080/api/v1/stocks/quotes' \
  -H 'Content-Type: application/json' \
  -d '{"symbols":[{"code":"000001.SZ"},{"code":"600519.SH"}]}'

# 日 K 线，不复权
curl 'http://127.0.0.1:8080/api/v1/stocks/bars?code=600519.SH&period=daily&adjust=none&limit=20'

# F10
curl 'http://127.0.0.1:8080/api/v1/stocks/f10?code=600519.SH'

# MAC 股票摘要
curl 'http://127.0.0.1:8080/api/v1/mac/symbols/info?code=600519.SH'

# MAC 板块成分
curl 'http://127.0.0.1:8080/api/v1/mac/boards/members?board_symbol=BK0420&limit=20'
```

## 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `HTTP_ADDR` | `:8080` | HTTP 监听地址 |
| `GIN_MODE` | `release` | `debug`、`release` 或 `test` |
| `CORS_ALLOWED_ORIGINS` | `*` | 逗号分隔的允许来源 |
| `CORS_ALLOWED_METHODS` | `GET,POST,OPTIONS` | 允许方法 |
| `CORS_ALLOWED_HEADERS` | `Origin,Content-Type,Accept,X-Request-ID` | 允许请求头 |
| `CORS_ALLOW_CREDENTIALS` | `false` | 是否允许凭据；不能与 `*` 来源组合 |
| `TDX_MAIN_HOSTS` | gotdx 内置主机 | 主市场主机池 |
| `TDX_MAC_HOSTS` | gotdx 内置主机 | MAC 主市场主机池 |
| `TDX_POOL_SIZE` | `2` | 客户端组数量，范围 1-32 |
| `TDX_TIMEOUT` | `6s` | 上游及请求超时 |
| `TDX_RETRY_LIMIT` | `1` | 失败后的附加尝试次数，范围 0-5 |
| `MAX_ITEMS` | `1000` | 列表与批量请求上限 |
| `SHUTDOWN_TIMEOUT` | `15s` | 优雅关闭期限 |

## 公共参数

接口只接受 Swagger 明确列出的参数，传入未知参数会返回 HTTP 400。

| 参数 | 用途 |
| --- | --- |
| `market` | 仅无 `code` 的全市场接口使用，`0` 深圳、`1` 上海 |
| `code` | 单只证券代码，必须包含交易所后缀，如 `000001.SZ`、`600519.SH` |
| `symbols` | 批量行情 JSON 数组，每项仅包含带交易所后缀的 `code` |
| `board_symbol` | MAC 板块代码，通过查询参数提供，不出现在路径中 |
| `offset` | 可选，列表或 K 线业务分页偏移，默认 `0` |
| `limit` | 可选，返回条数；不同接口有安全默认值，最大受 `MAX_ITEMS` 限制 |
| `date` | 历史查询日期，格式 `YYYYMMDD`；实时 MAC 接口可省略 |
| `period` | K 线周期：`1m`、`5m`、`15m`、`30m`、`1h`、`daily`、`weekly`、`monthly`、`quarterly`、`yearly` |
| `adjust` | 复权：`none`、`forward`、`backward` |
| `days` | MAC 多日分时天数，范围 1-30 |

协议内部的 `start`、`count`、数字 `category`、`times`、位图、排序和过滤参数不对外开放。

## Docker

```bash
docker build -t tdx-api:local .
docker run --rm -p 8080:8080 tdx-api:local
docker compose up --build -d
docker compose ps
docker compose logs -f tdx-api
docker compose down
```

Compose 使用 `/health/ready` 执行健康检查，并预留 20 秒停止宽限时间。

## 验证

```bash
go generate ./internal/httpapi
go test ./...
go test -race ./...
go vet ./...
openspec validate expose-gotdx-stock-api --strict
```

真实 TDX 集成测试默认跳过：

```bash
TDX_INTEGRATION=1 go test ./internal/tdx -run TestLive
```

## 能力边界

稳定 API 仅允许 `.SZ`（深圳）和 `.SH`（上海）证券代码；无证券代码的全市场接口仅允许 `market=0` 或 `market=1`。API 不包含北京、香港、美国、扩展市场、商品 `Goods*`、ICFQS 原始接口、实验性协议或旧版兼容协议。完整对应关系见 [`docs/COVERAGE.md`](docs/COVERAGE.md)。第三方 TDX 节点可能不可用、缓慢或返回不一致数据，服务仅提供有界重试、主机池故障转移和标准化错误响应。

## 固定版本

- Go 工具链及构建镜像：`go1.26.0`、`golang:1.26.0-alpine3.23`
- gotdx：`v0.0.0-20260812105458-c48bd706def9`，提交 `c48bd706def9f3e11b4e77078cb88e6d69ab4694`
- Gin：`v1.11.0`
