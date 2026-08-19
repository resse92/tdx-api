## MODIFIED Requirements

### Requirement: 主市场时序与成交数据访问
API SHALL 提供股票和指数 K 线、当前及历史分时、图表采样、当前成交、历史委托和历史成交能力。股票和指数 K 线接口 SHALL 使用 `start_date` 和 `end_date` 表达查询范围：日线及以上周期使用 `YYYYMMDD`，日线以下周期使用 `YYYYMMDDhhmmss`，并按北京时间解释；不得要求客户端提交上游分页参数。

#### Scenario: 查询历史 K 线
- **WHEN** 客户端向 `/api/v1/stocks` 下的股票或指数 K 线接口提供有效代码、K 线周期、`start_date` 和 `end_date`
- **THEN** API 返回该日期范围内按上游顺序排列的可用 K 线

#### Scenario: 日期范围转换为上游分页
- **WHEN** 客户端提交合法日期范围且服务调用主市场上游
- **THEN** 服务根据日期范围计算上游所需的 `offset+limit`，客户端不可观察或控制该内部分页值

#### Scenario: 日期范围无数据
- **WHEN** 客户端提供合法但没有可用 K 线的日期范围
- **THEN** API 返回统一成功结构中的空结果，不因上游分页换算失败而返回错误

#### Scenario: 查询历史成交
- **WHEN** 客户端提供有效的证券代码、日期和数量
- **THEN** API 返回该请求可用的成交或委托记录

### Requirement: 主市场输入边界
API SHALL 对按证券查询只接受 `.SZ` 和 `.SH` 代码并由后缀推导市场；仅无 `code` 的全市场查询接受 `market=0` 或 `market=1`。K 线接口 MUST 按周期接受对应格式的 `start_date` 和 `end_date`，并在调用上游前拒绝缺失、格式非法、起止顺序非法或超出服务限制的日期范围。K 线接口不得接受公共 `offset` 或 `limit` 参数；未知参数、无效日期、业务枚举、分页或批量长度 MUST 在调用上游前返回 HTTP 400。

#### Scenario: 拒绝无效 K 线日期范围
- **WHEN** 客户端提供缺失、格式非法、`start_date` 晚于 `end_date` 或超过允许跨度的 K 线日期范围
- **THEN** 服务返回 HTTP 400，且不调用 `gotdx`

#### Scenario: 拒绝旧 K 线分页参数
- **WHEN** 客户端在 K 线请求中提供 `offset` 或 `limit`
- **THEN** 服务返回 HTTP 400，且不调用 `gotdx`

#### Scenario: 拒绝非沪深市场
- **WHEN** 客户端提供非 `.SH`、`.SZ` 证券后缀，或在全市场接口提供其他市场值
- **THEN** 服务返回 HTTP 400，且不调用 `gotdx`

#### Scenario: 协议参数不可注入
- **WHEN** 客户端提供 `start`、数字 `category`、`times`、位图或其他未公开协议参数
- **THEN** 服务返回 HTTP 400，并使用服务器管理的协议参数
