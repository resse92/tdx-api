## MODIFIED Requirements

### Requirement: MAC 证券与市场访问
API SHALL 提供可选日期的证券快照与成交、集合竞价、多日分时、证券摘要、资金流向、市场监控、证券所属板块及统一证券 K 线。统一 MAC 证券 K 线接口 SHALL 使用 `start_date` 和 `end_date` 表达查询范围：日线及以上周期使用 `YYYYMMDD`，日线以下周期使用 `YYYYMMDDhhmmss`，并按北京时间解释；不得要求客户端提交上游分页参数。

#### Scenario: 查询统一 MAC 证券 K 线
- **WHEN** 客户端向 `/api/v1/mac` 下的统一证券 K 线接口提供有效的带交易所后缀证券代码、周期、`start_date` 和 `end_date`
- **THEN** API 返回该日期范围内可用的 MAC 证券 K 线

#### Scenario: 日期范围转换为上游分页
- **WHEN** 客户端提交合法日期范围且服务调用 MAC 上游
- **THEN** 服务根据日期范围计算上游所需的 `offset+limit`，客户端不可观察或控制该内部分页值

#### Scenario: 查询 MAC 市场监控
- **WHEN** 客户端提供有效的沪深 `market` 和数量
- **THEN** API 返回对应市场的监控记录

### Requirement: MAC 输入边界
API SHALL 在调用 MAC 上游前校验必需标识、沪深市场、日期、业务枚举、分页和数量。统一 MAC 证券 K 线接口 MUST 按周期接受对应格式的 `start_date` 和 `end_date`，并拒绝缺失、格式非法、起止顺序非法或超出服务限制的日期范围；该接口不得接受公共 `offset` 或 `limit` 参数。未知参数、位图、排序、过滤、协议起点和倍率不得作为公共参数。

#### Scenario: 拒绝无效 MAC K 线日期范围
- **WHEN** 客户端提供缺失、格式非法、`start_date` 晚于 `end_date` 或超过允许跨度的 MAC K 线日期范围
- **THEN** 服务返回 HTTP 400，且不调用 MAC 上游

#### Scenario: 拒绝旧 MAC K 线分页参数
- **WHEN** 客户端在统一 MAC 证券 K 线请求中提供 `offset` 或 `limit`
- **THEN** 服务返回 HTTP 400，且不调用 MAC 上游

#### Scenario: 拒绝未知参数
- **WHEN** 客户端提供位图、排序、过滤、`start`、数字 `category`、`times` 或其他未公开参数
- **THEN** 服务返回 HTTP 400，且不调用 MAC 上游

#### Scenario: 拒绝非沪深证券
- **WHEN** 客户端提供非 `.SH` 或 `.SZ` 后缀代码
- **THEN** 服务返回 HTTP 400，且不调用 MAC 上游
