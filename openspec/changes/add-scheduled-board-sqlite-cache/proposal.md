## Why

板块列表和板块成分目前每次查询都依赖 MAC 上游，既增加响应延迟和连接压力，也会在上游短暂不可用时直接影响常用板块数据访问。需要通过按北京时间定时落盘和缓存缺失时回源，提供可持续复用的本地数据。

## What Changes

- 新增 SQLite 板块数据存储，保存板块列表、各板块成分及最近一次成功更新时间。
- 新增板块数据刷新任务，每天按 `Asia/Shanghai` 时区在 09:00、15:00、20:00 从 MAC 上游刷新板块列表及列表中每个板块的成分。
- 修改板块列表和板块成分查询流程：优先返回 SQLite 数据；对应缓存没有数据时请求 MAC 上游，并将成功结果写入 SQLite 后返回。
- 定时刷新采用成功后替换语义；单次上游失败不得清空已有缓存，并记录可诊断日志。
- 增加 SQLite 路径配置、启动初始化和优雅关闭行为，并为容器部署提供可持久化数据目录。

## Capabilities

### New Capabilities
- `board-data-cache`: 定义 SQLite 板块数据持久化、北京时间定时刷新、缺失回源及故障保留行为。

### Modified Capabilities
- `mac-market-http-api`: 将板块列表和板块成分的读取来源改为 SQLite 优先、缓存缺失时回源，同时保持现有 HTTP 契约。

## Impact

- 影响 `cmd/api` 的依赖装配和关闭流程、`internal/config` 配置、`internal/httpapi` 板块查询编排及 `internal/tdx` 的后台调用方式。
- 新增板块缓存/调度包和 SQLite 驱动依赖，并需要为容器挂载可写、可持久化的数据目录。
- `/api/v1/mac/boards` 与 `/api/v1/mac/boards/members` 的路径、参数和响应结构保持兼容；`/api/v1/mac/boards/quotes` 不纳入持久化缓存，继续实时请求上游。
- 不扩大 `gotdx` 能力覆盖，不改变 CORS 策略；部署时新增 SQLite 文件路径及持久化卷配置。
