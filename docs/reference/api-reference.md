# MeowHomo API 接口参考

本文档覆盖 REST API（Rust 后端 ↔ 前端）和 gRPC（Rust ↔ Go 内核）两层接口。

## 1. REST API (Rust axum)

所有接口默认前缀 `/api/v1`，返回 JSON。需要 JWT Bearer Token 认证（登录接口除外）。

### 1.1 认证

| 方法 | 路由 | 说明 |
|------|------|------|
| POST | `/api/v1/auth/login` | 登录，返回 JWT Token |
| POST | `/api/v1/auth/refresh` | 刷新 Token |

### 1.2 节点管理

| 方法 | 路由 | 说明 |
|------|------|------|
| GET | `/api/v1/nodes` | 获取全部节点（按来源分组）|
| POST | `/api/v1/nodes/subscription` | 添加订阅 URL |
| POST | `/api/v1/nodes/manual` | 手动添加单条链接 |
| POST | `/api/v1/nodes/local` | 创建本机节点（trojan）|
| PUT | `/api/v1/nodes/:id` | 更新节点信息 |
| DELETE | `/api/v1/nodes/:id` | 删除节点 |
| POST | `/api/v1/nodes/subscriptions/refresh` | 刷新所有订阅 |
| GET | `/api/v1/nodes/:id/qrcode` | 获取节点 QR 码 |
| GET | `/api/v1/nodes/:id/link` | 获取节点直连链接 |

### 1.3 策略管理

| 方法 | 路由 | 说明 |
|------|------|------|
| GET | `/api/v1/strategies` | 获取所有策略组 |
| POST | `/api/v1/strategies` | 创建策略组 |
| PUT | `/api/v1/strategies/:id` | 更新策略（类型/节点/优先级）|
| DELETE | `/api/v1/strategies/:id` | 删除策略组 |
| POST | `/api/v1/strategies/:id/speedtest` | 对策略组执行测速 |
| PUT | `/api/v1/strategies/:id/nodes/order` | 拖拽排序节点优先级 |
| PUT | `/api/v1/strategies/:id/expose` | 配置对外暴露（端口/认证/白名单）|

### 1.4 内核控制

| 方法 | 路由 | 说明 |
|------|------|------|
| GET | `/api/v1/engine/status` | 获取引擎状态 |
| POST | `/api/v1/engine/restart` | 重启引擎 |
| POST | `/api/v1/engine/reload` | 热重载配置 |
| GET | `/api/v1/engine/connections` | 获取活跃连接列表 |
| DELETE | `/api/v1/engine/connections/:id` | 关闭指定连接 |

### 1.5 设置

| 方法 | 路由 | 说明 |
|------|------|------|
| GET | `/api/v1/settings` | 获取全部设置 |
| PUT | `/api/v1/settings` | 更新设置 |
| POST | `/api/v1/settings/tls/acme` | 触发 ACME 证书签发 |
| GET | `/api/v1/settings/tls/status` | 获取证书状态 |

### 1.6 备份

| 方法 | 路由 | 说明 |
|------|------|------|
| GET | `/api/v1/backup/export?format=json\|yaml\|zip` | 导出备份 |
| POST | `/api/v1/backup/import` | 导入备份（multipart 上传）|
| GET | `/api/v1/backup/list` | 获取历史自动备份列表 |

### 1.7 日志

| 方法 | 路由 | 说明 |
|------|------|------|
| GET | `/api/v1/logs` | 获取历史日志（分页 + 过滤）|
| GET | `/api/v1/logs/export` | 导出日志文件 |

### 1.8 订阅分发

| 方法 | 路由 | 说明 |
|------|------|------|
| GET | `/api/v1/subscription/tokens` | 获取订阅 token 列表 |
| POST | `/api/v1/subscription/tokens` | 生成新订阅 token |
| DELETE | `/api/v1/subscription/tokens/:id` | 撤销 token |
| GET | `/sub/:token` | 公开接口：根据 token 返回 Clash 订阅配置（无需 JWT）|

## 2. SSE 事件流

| 端点 | 推送事件 |
|------|---------|
| `GET /api/v1/events` | 统一 SSE 通道，按 `event` 字段区分类型 |

**事件类型**：

| event 字段 | data 内容 | 触发时机 |
|-----------|----------|---------|
| `log` | `LogEntry` JSON | 实时日志（含猫娘角色） |
| `traffic` | `{ upload_speed, download_speed }` | 每秒流量统计 |
| `connection` | `{ total, active }` | 连接数变化 |
| `node_status` | `{ node_id, latency, alive }` | 节点状态变化 |
| `port_migration` | `{ old_port, new_port, reason }` | 端口迁移通知 |
| `engine_status` | `{ running, version, memory }` | 引擎状态变化 |
| `notification` | `{ level, title, message }` | 系统级通知（证书过期等）|

## 3. gRPC 接口 (Rust ↔ Go)

详见 `proto/engine.proto`，主要 RPC 方法：

| RPC | 请求 | 响应 | 说明 |
|-----|------|------|------|
| `Start` | `StartRequest { config_yaml }` | `StatusResponse` | 启动引擎（传入完整 YAML 配置）|
| `Stop` | `Empty` | `StatusResponse` | 停止引擎 |
| `Reload` | `ReloadRequest { config_yaml }` | `StatusResponse` | 热重载配置 |
| `GetStatus` | `Empty` | `StatusResponse` | 查询运行状态 |
| `GetConnections` | `Empty` | `ConnectionsResponse` | 获取活跃连接 |
| `GetTraffic` | `Empty` | `TrafficResponse` | 获取流量统计 |
| `StreamLogs` | `Empty` | `stream LogEntry` | 服务端流式推送日志 |

> **注意**：Rust 后端是 gRPC 的 **客户端**，Go 内核是 **服务端**。Rust 主动发起连接和请求。
