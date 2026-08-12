## Purpose

通过环境变量配置、健康检查和一键式本地部署流程，使股票 API 能够以最小化容器形式进行可重复构建和可靠运行。

## ADDED Requirements

### Requirement: 可重复构建的容器镜像
项目 SHALL 提供多阶段 Docker 构建，将 API 编译为最小化 Linux 运行时镜像，以非 root 用户运行，仅包含必要的运行时资源，并暴露配置的 HTTP 端口。

#### Scenario: 构建镜像
- **WHEN** 运维人员使用文档中的 Docker 命令构建仓库
- **THEN** Docker 在宿主机无需安装 Go 工具链的情况下生成可运行的 API 镜像

#### Scenario: 检查运行身份
- **WHEN** 构建后的容器启动
- **THEN** API 进程以非 root 权限运行

### Requirement: 环境变量驱动的运行时配置
容器 SHALL 通过有安全默认值且有文档说明的环境变量接收 HTTP 地址、Gin 模式、CORS 设置、TDX 主机池、上游超时、重试次数和关闭超时。

#### Scenario: 使用默认配置启动
- **WHEN** 运维人员启动镜像且未提供可选环境变量覆盖值
- **THEN** API 监听文档约定的默认端口并允许跨域访问 API

#### Scenario: 覆盖部署配置
- **WHEN** 运维人员提供受支持的环境变量
- **THEN** 服务在启动时校验并应用这些配置值

#### Scenario: 启动配置无效
- **WHEN** 必需的环境变量格式错误或配置互相冲突
- **THEN** 进程以非零状态退出并输出明确的配置错误

### Requirement: 自动化 Compose 部署
项目 SHALL 提供 Docker Compose 定义，用于构建或启动 API、发布 HTTP 端口、应用环境配置、在异常失败后重启服务，并通过 API 健康检查报告服务状态。

#### Scenario: 使用 Compose 部署
- **WHEN** 运维人员执行文档中的 Docker Compose 启动命令
- **THEN** Compose 启动 API，并在就绪检查成功后将其报告为健康状态

### Requirement: 容器生命周期兼容性
容器 SHALL 将终止信号转发给 API 进程，并 SHALL 提供足够的停止宽限时间以满足服务的优雅关闭期限。

#### Scenario: 停止部署
- **WHEN** 运维人员停止 Compose 服务
- **THEN** API 收到终止信号、排空请求、关闭上游连接，并在配置的宽限期内退出
