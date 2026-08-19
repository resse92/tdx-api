## 1. 日期范围模型与分页规划

- [x] 1.1 在 `internal/httpapi` 定义 K 线日期范围请求模型，按周期解析 `YYYYMMDD` 或 `YYYYMMDDhhmmss`、包含起止边界、最大跨度和旧 `offset`/`limit` 参数拒绝规则，并为合法、缺失、格式错误、逆序和超范围输入编写表驱动测试
- [x] 1.2 在 `internal/tdx` 或独立内部编排边界实现按周期将日期范围转换为上游 `offset+limit` 的规划器，支持交易日缺口、有限窗口扩大、最大边界和整数溢出保护，并为规划结果编写表驱动测试
- [x] 1.3 定义统一的 K 线日期范围过滤和结果边界处理，保留上游顺序，覆盖周末、节假日、空结果、未覆盖边界和请求取消测试

## 2. 主市场与 MAC 接入

- [x] 2.1 修改 `internal/tdx` 的主市场股票 K 线、指数 K 线和 MAC 统一 K 线调用参数，将日期范围交给分页规划器并仅向 gotdx 传递计算后的 `offset+limit`
- [x] 2.2 更新 `internal/httpapi` 的 `/api/v1/stocks/bars`、`/api/v1/stocks/index-bars` 和 `/api/v1/mac/symbols/bars` 请求编排，校验通过后调用新的日期范围接口，并用 fake 上游断言主市场和 MAC 的分页参数及过滤结果
- [x] 2.3 使用 `httptest` 覆盖三个 K 线接口的成功日期查询、空结果、无效日期、旧分页参数、未知协议参数、非沪深代码和上游错误，确认无效请求不触碰 gotdx

## 3. 公共契约与迁移文档

- [x] 3.1 修改 `internal/httpapi/openapi.go`，从受影响 K 线操作移除 `offset`、`limit`，增加必填 `start_date`、`end_date` 的格式、范围和 HTTP 400 说明，同时保持其他分页接口不变
- [x] 3.2 重新生成并核对 `docs/openapi.json`，更新受影响能力说明和调用示例，确认路由、响应结构、错误结构和 CORS 行为未发生额外变化
- [x] 3.3 为日期范围迁移补充请求参数兼容性和 OpenAPI 回归测试，确保所有 K 线操作均覆盖新参数且不意外改变非 K 线接口

## 4. 完整验证

- [x] 4.1 对修改后的 Go 文件运行 `gofmt`，执行 `go test ./...`、`go test -race ./...` 和 `go vet ./...`
- [x] 4.2 运行 `openspec validate switch-kline-to-date-range --strict`，修复规格格式、完整 requirement 或场景校验问题
- [x] 4.3 复核真实 TDX 集成测试仍为显式启用，确认无新增依赖、连接生命周期变化或容器配置变化
