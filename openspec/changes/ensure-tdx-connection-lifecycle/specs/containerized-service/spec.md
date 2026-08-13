## MODIFIED Requirements

### Requirement: 自动化 Compose 部署
项目 SHALL 提供 Docker Compose 定义，用于构建或启动 API、发布 HTTP 端口、应用环境配置、在异常失败后重启服务，并通过同时反映主市场和 MAC 真实连接状态的 API 就绪检查报告服务状态。

#### Scenario: 使用 Compose 部署
- **WHEN** 运维人员执行文档中的 Docker Compose 启动命令且两类 TDX 上游均可连接
- **THEN** Compose 启动 API，并在主市场与 MAC 连接均建立后将其报告为健康状态

#### Scenario: 上游连接未建立
- **WHEN** 主市场或 MAC 任一必需连接未建立或已断开
- **THEN** Compose 健康检查不得将容器报告为健康状态

### Requirement: 容器生命周期兼容性
容器 SHALL 将终止信号转发给 API 进程，并 SHALL 提供足够的停止宽限时间，使服务能够停止新的重连活动、排空请求并关闭全部上游连接。

#### Scenario: 停止部署
- **WHEN** 运维人员停止 Compose 服务
- **THEN** API 收到终止信号、停止连接恢复、排空请求、关闭上游连接，并在配置的宽限期内退出
