# MAC 市场 HTTP API 规格

## Purpose

为 `gotdx` MAC 协议支持的板块、证券增强行情和市场监控能力提供稳定的 `/api/v1/mac` HTTP 接口。

## Requirements

### Requirement: MAC 路由边界
服务 SHALL 将 MAC 客户端提供的稳定能力统一公开在 `/api/v1/mac` 下，不得混入 `/api/v1/stocks`。证券代码 `code` SHALL 使用 `<6 位数字>.SH` 或 `<6 位数字>.SZ` 格式，板块代码 `board_symbol` SHALL 通过查询参数提供，二者均不得出现在接口路径中。

#### Scenario: 查询 MAC 证券能力
- **WHEN** 客户端向 `/api/v1/mac/symbols` 下的稳定接口提供有效代码
- **THEN** 服务通过 MAC 客户端执行对应查询

#### Scenario: MAC 能力不混入主市场前缀
- **WHEN** 客户端尝试通过 `/api/v1/stocks` 请求仅由 MAC 协议提供的板块或资金流能力
- **THEN** 服务返回 HTTP 404

### Requirement: MAC 板块访问
API SHALL 提供板块列表、板块成分和板块成分行情，板块成分及行情 SHALL 使用固定路径和必填 `board_symbol` 查询参数。

#### Scenario: 通过查询参数查询板块
- **WHEN** 客户端向固定板块成分或行情路径提供有效的 `board_symbol`
- **THEN** API 使用该板块代码查询上游，且不注册包含板块代码的动态路径

#### Scenario: 板块代码缺失
- **WHEN** 客户端省略 `board_symbol` 或提供无效格式
- **THEN** 服务返回 HTTP 400，且不调用 MAC 上游

### Requirement: MAC 证券与市场访问
API SHALL 提供可选日期的证券快照与成交、集合竞价、多日分时、证券摘要、资金流向、市场监控、证券所属板块及统一证券 K 线。

#### Scenario: 查询统一 MAC 证券 K 线
- **WHEN** 客户端提供受支持的带交易所后缀证券代码、周期、偏移量和数量
- **THEN** API 返回请求范围内可用的 MAC 证券 K 线

#### Scenario: 查询 MAC 市场监控
- **WHEN** 客户端提供有效的沪深 `market` 和数量
- **THEN** API 返回对应市场的监控记录

### Requirement: MAC 输入边界
API SHALL 在调用 MAC 上游前校验必需标识、沪深市场、日期、业务枚举、分页和数量。未知参数 MUST 返回 HTTP 400，位图、排序、过滤、协议起点和倍率不得作为公共参数。

#### Scenario: 拒绝未知参数
- **WHEN** 客户端提供位图、排序、过滤、`start`、数字 `category`、`times` 或其他未公开参数
- **THEN** 服务返回 HTTP 400，且不调用 MAC 上游

#### Scenario: 拒绝非沪深证券
- **WHEN** 客户端提供非 `.SH` 或 `.SZ` 后缀代码
- **THEN** 服务返回 HTTP 400，且不调用 MAC 上游

### Requirement: MAC 原始与运维接口不公开
MAC 原始文件、服务器信息、心跳、K 线偏移和动态协议控制能力 SHALL 保持为服务内部实现细节，不得注册公共路由。

#### Scenario: 请求 MAC 原始文件
- **WHEN** 客户端请求 MAC 文件列表、元数据或下载路由
- **THEN** 服务返回 HTTP 404，且不调用对应文件方法

### Requirement: MAC 上游生命周期
服务 SHALL 在应用开始接受 HTTP 流量前建立全部必需的 MAC 连接，并 SHALL 复用并发安全的 MAC 客户端资源。服务 MUST 跟踪真实连接状态，在连接断开后执行有界重连或主机故障转移，并在关闭时停止恢复活动并释放连接。

#### Scenario: 启动时连接 MAC
- **WHEN** 应用启动且至少一个配置的 MAC 节点可用
- **THEN** 服务在开始接受 HTTP 流量前建立 MAC 连接

#### Scenario: MAC 启动连接失败
- **WHEN** 应用启动时没有配置的 MAC 节点能在期限内完成连接
- **THEN** 应用返回明确启动错误且不进入可接收流量的状态

#### Scenario: MAC 连接断开
- **WHEN** 已建立的 MAC 连接在查询期间发生可重试网络错误
- **THEN** 服务标记连接为不可用，并在配置的次数和期限内重新连接或切换节点

#### Scenario: 请求前检查 MAC 连接
- **WHEN** MAC 接口收到请求且对应连接当前不可用
- **THEN** 服务在调用业务协议前同步尝试恢复连接，恢复成功后执行请求，恢复失败则返回标准上游错误

#### Scenario: 服务关闭
- **WHEN** 应用进入优雅关闭流程
- **THEN** 服务停止新的连接恢复活动、排空在途调用并关闭全部 MAC 连接

### Requirement: MAC HTTP 通用契约
MAC 接口 SHALL 使用统一 JSON 成功与错误结构、请求 ID 和稳定错误码，SHALL 应用默认宽松且可配置的 CORS，并 SHALL 被机器可读 OpenAPI 文档完整覆盖。就绪检查 MUST 仅在全部必需的 MAC 连接真实可用时报告成功。

#### Scenario: MAC 上游超时
- **WHEN** MAC 上游未在配置期限内响应
- **THEN** API 返回 HTTP 504 和标准超时错误码

#### Scenario: MAC 连接不可用
- **WHEN** 请求前连接恢复失败且错误不是超时
- **THEN** API 返回 HTTP 502 和标准上游不可用错误码

#### Scenario: MAC 连接未就绪
- **WHEN** 必需的 MAC 连接未建立或已断开且尚未恢复
- **THEN** 就绪检查返回失败状态

#### Scenario: 发现 MAC 接口
- **WHEN** 客户端获取 OpenAPI 文档
- **THEN** 文档包含全部 `/api/v1/mac` 稳定操作

### Requirement: MAC 连接可观测性
服务 SHALL 使用结构化日志记录 MAC 连接尝试、连接成功、连接失败、协议错误导致的断开、请求触发的重连和服务关闭。日志 MUST 包含协议类别和客户端组标识，错误日志 MUST 包含错误原因，且不得记录请求中的证券列表或上游原始响应内容。

#### Scenario: MAC 连接成功
- **WHEN** 启动连接或请求触发的重连成功
- **THEN** 服务记录包含 `protocol=mac`、客户端组和当前地址的成功日志

#### Scenario: MAC 连接失败
- **WHEN** MAC 连接或重连失败
- **THEN** 服务记录包含 `protocol=mac`、客户端组、尝试类型和错误原因的错误日志

#### Scenario: MAC 连接断开
- **WHEN** 可重试协议错误使 MAC 连接失效或服务关闭该连接
- **THEN** 服务记录断开原因和连接状态变化，不记录业务请求或原始响应数据
