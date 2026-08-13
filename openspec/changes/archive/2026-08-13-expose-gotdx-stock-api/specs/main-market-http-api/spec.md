## Purpose

为 `gotdx` 主市场协议支持的沪深证券、指数、行情和公司资料提供稳定的 `/api/v1/stocks` HTTP 接口。

## ADDED Requirements

### Requirement: 主市场路由边界
服务 SHALL 将普通 `gotdx.Client` 提供的稳定主市场能力统一公开在 `/api/v1/stocks` 下，不得为指数、公告或其他主市场能力注册独立顶级前缀。证券代码 `code` SHALL 使用 `<6 位数字>.SH` 或 `<6 位数字>.SZ` 格式通过查询参数或 JSON 请求体提供，不得出现在接口路径中。

#### Scenario: 查询指数 K 线
- **WHEN** 客户端向 `/api/v1/stocks/index-bars` 提供有效的指数代码和 K 线参数
- **THEN** 服务通过主市场客户端调用指数 K 线能力并返回结果

#### Scenario: 旧指数前缀不可访问
- **WHEN** 客户端请求 `/api/v1/indexes` 下的任意路由
- **THEN** 服务使用标准错误响应返回 HTTP 404

#### Scenario: 旧市场前缀不可访问
- **WHEN** 客户端请求 `/api/v1/market` 下的任意路由
- **THEN** 服务使用标准错误响应返回 HTTP 404

### Requirement: 主市场证券与行情访问
API SHALL 提供证券数量、分页证券列表、单个及批量行情快照，以及受支持沪深代码的详细行情。

#### Scenario: 查询证券列表
- **WHEN** 客户端提供有效的 `market` 和分页范围
- **THEN** API 返回匹配的主市场证券

#### Scenario: 查询批量行情
- **WHEN** 客户端提交每项仅含 `.SH` 或 `.SZ` 后缀 `code` 的 `symbols` 数组
- **THEN** 服务校验每个证券、推导其市场并执行批量行情查询，不接受项内 `market`

### Requirement: 主市场时序与成交数据访问
API SHALL 提供股票和指数 K 线、当前及历史分时、图表采样、当前成交、历史委托和历史成交能力。

#### Scenario: 查询历史 K 线
- **WHEN** 客户端提供有效的证券代码、K 线周期、偏移量和数量
- **THEN** API 按上游顺序返回可用 K 线

#### Scenario: 查询历史成交
- **WHEN** 客户端提供有效的证券代码、日期和数量
- **THEN** API 返回该请求可用的成交或委托记录

### Requirement: 主市场分析与事件访问
API SHALL 提供指数信息、指数动量、筹码分布、集合竞价、市场异动和公告能力。

#### Scenario: 查询市场事件数据
- **WHEN** 客户端向集合竞价、异动或公告接口提交有效参数
- **THEN** API 返回对应的上游记录，且不得静默替换为其他数据集

### Requirement: F10、财务与公司行为访问
API SHALL 提供聚合 F10、财务信息和除权除息信息，原始文件获取和解析 SHALL 由服务器内部处理。

#### Scenario: 查询 F10 信息
- **WHEN** 客户端向 F10 接口提供受支持的带交易所后缀股票代码
- **THEN** API 返回聚合 F10、财务或公司行为数据

#### Scenario: 原始文件接口不可访问
- **WHEN** 客户端请求文件元数据、文件下载或要求传入上游文件名
- **THEN** 服务返回 HTTP 404，且不调用 `gotdx` 文件方法

### Requirement: 主市场输入边界
API SHALL 对按证券查询只接受 `.SZ` 和 `.SH` 代码并由后缀推导市场；仅无 `code` 的全市场查询接受 `market=0` 或 `market=1`。未知参数、无效日期、业务枚举、分页或批量长度 MUST 在调用上游前返回 HTTP 400。

#### Scenario: 拒绝非沪深市场
- **WHEN** 客户端提供非 `.SH`、`.SZ` 证券后缀，或在全市场接口提供其他市场值
- **THEN** 服务返回 HTTP 400，且不调用 `gotdx`

#### Scenario: 协议参数不可注入
- **WHEN** 客户端提供 `start`、数字 `category`、`times`、位图或其他未公开协议参数
- **THEN** 服务返回 HTTP 400，并使用服务器管理的协议参数

### Requirement: 主市场上游生命周期
服务 SHALL 复用并发安全的主市场客户端资源，使用可配置的主机池和超时，对可安全重试的查询执行有界重连或故障转移，并在关闭时释放连接。

#### Scenario: 主市场节点失败
- **WHEN** 主市场查询在获得有效响应前发生可重试连接错误
- **THEN** 服务在返回标准上游错误前执行次数受限的重试或故障转移

### Requirement: 主市场 HTTP 通用契约
主市场接口 SHALL 使用统一 JSON 成功与错误结构、请求 ID 和稳定错误码，SHALL 应用默认宽松且可配置的 CORS，并 SHALL 被机器可读 OpenAPI 文档完整覆盖。

#### Scenario: 主市场上游超时
- **WHEN** 主市场上游未在配置期限内响应
- **THEN** API 返回 HTTP 504 和标准超时错误码

#### Scenario: 发现主市场接口
- **WHEN** 客户端获取 OpenAPI 文档
- **THEN** 文档包含全部 `/api/v1/stocks` 稳定操作
