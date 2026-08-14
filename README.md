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

# 上证指数日 K 线（指数也属于主市场 stocks 前缀）
curl 'http://127.0.0.1:8080/api/v1/stocks/index-bars?code=000001.SH&period=daily&limit=20'

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
| `SQLITE_PATH` | `./data/boards.sqlite` | 板块缓存 SQLite 文件路径 |
| `BOARD_REFRESH_TIMEOUT` | `2m` | 全量板块刷新超时 |

服务启动时会先为每个客户端组建立主市场和 MAC 连接，全部连接成功后才开始监听 HTTP。任一上游不可达时进程以非零状态退出，由部署平台按重启策略再次尝试。请求执行前会检查对应协议连接；可重试网络错误只断开失败的协议连接，并在请求期限和 `TDX_RETRY_LIMIT` 范围内重新连接。

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

Compose 使用 `/health/ready` 执行健康检查；只有全部主市场与 MAC 连接可用且服务未关闭时才报告健康。Compose 预留 20 秒停止宽限时间，关闭开始后服务不再建立新连接，并等待在途请求结束后断开上游。板块缓存存储在每个 Compose 实例独占的 `board-cache` 卷中，容器内路径为 `/data/boards.sqlite`。

MAC 板块列表和板块成分优先从 SQLite 读取；缓存未加载时才回源并保存成功响应。服务按北京时间每日 09:00、15:00 和 20:00 刷新完整板块数据，刷新失败继续提供上一份完整缓存。板块成分行情保持实时上游查询。

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

公共接口按上游协议分组：普通主市场客户端的股票、指数和 F10 能力统一位于 `/api/v1/stocks`，MAC 客户端能力位于 `/api/v1/mac`。稳定 API 仅允许 `.SZ`（深圳）和 `.SH`（上海）证券代码；无证券代码的全市场接口仅允许 `market=0` 或 `market=1`。API 不包含北京、香港、美国、扩展市场、商品 `Goods*`、ICFQS 原始接口、实验性协议或旧版兼容协议。完整对应关系见 [`docs/COVERAGE.md`](docs/COVERAGE.md)。第三方 TDX 节点可能不可用、缓慢或返回不一致数据，服务仅提供有界重试、主机池故障转移和标准化错误响应。

## 固定版本

- Go 工具链及构建镜像：`go1.26.0`、`golang:1.26.0-alpine3.23`
- gotdx：`v0.0.0-20260812105458-c48bd706def9`，提交 `c48bd706def9f3e11b4e77078cb88e6d69ab4694`
- Gin：`v1.11.0`
