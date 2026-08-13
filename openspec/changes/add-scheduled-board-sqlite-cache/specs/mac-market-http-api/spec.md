## MODIFIED Requirements

### Requirement: MAC 板块访问
API SHALL 提供板块列表、板块成分和板块成分行情，板块成分及行情 SHALL 使用固定路径和必填 `board_symbol` 查询参数。`/api/v1/mac/boards` 和 `/api/v1/mac/boards/members` SHALL 优先读取持久化数据，并仅在对应数据不存在时请求 MAC 上游和保存成功结果；`/api/v1/mac/boards/quotes` SHALL 继续实时请求上游。缓存命中与回源成功 MUST 保持现有成功响应结构和 `limit` 边界。

#### Scenario: 通过查询参数查询板块
- **WHEN** 客户端向固定板块成分或行情路径提供有效的 `board_symbol`
- **THEN** API 使用该板块代码查询对应数据源，且不注册包含板块代码的动态路径

#### Scenario: 板块代码缺失
- **WHEN** 客户端省略 `board_symbol` 或提供无效格式
- **THEN** 服务返回 HTTP 400，且不读取缓存或调用 MAC 上游

#### Scenario: 查询已缓存板块列表
- **WHEN** 客户端请求 `/api/v1/mac/boards` 且存储中已有板块列表
- **THEN** 服务从持久化数据返回不超过 `limit` 的结果，且不调用 MAC 上游

#### Scenario: 查询未缓存板块成分
- **WHEN** 客户端请求 `/api/v1/mac/boards/members` 且指定板块没有持久化成分
- **THEN** 服务调用 MAC 上游，保存成功结果，并以现有成功响应结构返回不超过 `limit` 的结果

#### Scenario: 查询板块成分行情
- **WHEN** 客户端请求 `/api/v1/mac/boards/quotes` 并提供有效参数
- **THEN** 服务实时调用 MAC 上游且不使用持久化板块缓存
