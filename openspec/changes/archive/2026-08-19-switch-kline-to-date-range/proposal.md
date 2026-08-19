## Why

当前所有 K 线接口要求调用方通过 `offset` 和 `limit` 估算上游分页范围，难以表达明确的历史日期区间，也会让调用方依赖上游记录顺序和分页细节。将公共查询方式改为日期范围后，服务可以统一根据交易日数据计算上游所需的 `offset+limit`，降低客户端复杂度并保持上游协议细节在服务内部。

## What Changes

- **BREAKING** 修改主市场和 MAC 市场的股票、指数及其他统一 K 线 HTTP 接口：使用 `start_date` 和 `end_date` 查询区间替代公共 `offset`、`limit` 参数。
- 统一日期范围格式：日线及以上周期使用 `YYYYMMDD`，日线以下周期使用 `YYYYMMDDhhmmss`；统一区间边界和非法范围校验，并在参数校验失败时不调用上游。
- 在服务编排层将日期区间转换为 gotdx 所需的 `offset+limit`，按上游返回顺序返回区间内数据。
- 更新 K 线相关 OpenAPI 描述、请求 DTO、错误契约和测试；不扩大 gotdx 能力覆盖。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `main-market-http-api`: 修改股票和指数 K 线的公共日期范围参数及其校验、分页转换行为。
- `mac-market-http-api`: 修改统一 MAC 证券 K 线的公共日期范围参数及其校验、分页转换行为。

## Impact

- 影响 `internal/httpapi` 的 K 线请求参数、路由处理、OpenAPI 生成和响应前的范围编排，以及 `internal/tdx` 对 gotdx `offset+limit` 的调用适配。
- 这是公共 HTTP 请求参数的破坏性变更：客户端必须迁移到 `start_date`/`end_date`，响应结构、路由前缀、统一错误响应和 CORS 策略保持不变。
- 不改变 gotdx 连接生命周期、部署方式或外部依赖；日期范围计算需要覆盖交易日缺口、空结果和上游失败场景。
