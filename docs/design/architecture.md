# MeowHomo 架构说明

本项目采用 **Rust + Go 混合服务端架构** 搭配 **React 前端**，并支持 Web 和 Tauri（桌面端）双模式部署。

## 1. 整体架构图

```text
                 [前端浏览器 / Tauri Webview]
         (React + Tailwind v3 + shadcn/ui + CesiumJS)
       Zustand 局部更新 / 响应式 UI (桌面端+移动端适配)
                            │
               HTTP (REST API) + SSE (Server-Sent Events)
               (在 Tauri 模式下，也可以调用 Tauri Command)
                            │
             [Rust 核心控制面 (axum)]
      (业务逻辑、数据库 CRUD、TLS 证书管理、反向代理、JWT 认证)
                            │
                      gRPC 协议
              (配置下发、状态查询、日志流订阅)
                            │
                [Go 内核管理器进程]
      (集成 MetaCubeX/mihomo 库，直接管理代理核心)
```

## 2. 为什么选择 Rust + Go 混合架构？

- **Go 的优势**：网络代理、协议解析领域（如 Clash 生态、Sing-box）几乎全部由 Go 编写。为了无缝接入 `mihomo` 强大的策略引擎和协议支持，必须且最适合用 Go。
- **Rust 的优势**：极低的内存占用，极强的内存安全性。对于面板服务（控制面），常驻后台时只需占用极少的资源，极大优化服务器开销。同时 Rust 在嵌入式 Webview（Tauri）方面有巨大优势，利于跨平台发布桌面端。
- **分离与解耦**：控制平面（Rust）和数据平面（Go mihomo）隔离开：Go 模块可以崩溃/热重载，它死掉不影响面板的存活；Web 界面始终可以连接到 Rust 控制面查看状态并由 Rust 负责拉起或重启 Go 引擎进程。

## 3. 通信机制

1. **前端 -> 后端 (Rust)**
   - 使用标准 RESTful API 传输 JSON 数据进行配置保存。
   - `apps/web/src/api` 下统一管控接口定义。

2. **后端 (Rust) -> 前端**
   - 采用 **SSE (Server-Sent Events)** 建立长连接，单向推送后端状态变化（如日志、节点延迟、状态提示）。
   - 具备降级机制，SSE 连接失败时，前端自动降级为 Polling(轮询) 请求 REST API。
   - 严禁触发 `location.reload()` 全局刷新页面，全量借助 Zustand Store 进行局部 UI 更新。

3. **Rust -> Go 内核**
   - 采用 **gRPC** 建立进程间高速通信信道（详见 `proto/engine.proto`）。
   - Rust 组装最终的 YAML 配置（字符串形式）通过 `StartRequest` / `ReloadRequest` 发送给 Go。
   - Go 调用 `mihomo.Start()` 并通过 gRPC 的 `stream LogEntry` 将原生日志源源不断地回传给 Rust 控制面。

## 4. 目录职责 (Monorepo)

- `crates/` (Rust Workspace)
  - `server/`: Axum 框架主入口。
  - `core/`: 独立出的业务逻辑组件，被 `server` 和 `tauri-plugin` 共享。
  - `tauri-plugin/`: 用于将 Tauri 前端与本地 Rust Core 进行桥接绑定。
- `engine/` (Go Module)
  - 核心桥接器。纯粹无 UI 的后台驻留程序，接收 gRPC 命令。
- `apps/`
  - `web/`: Vite驱动的 React App。
- `packages/`
  - `@meowhomo/ui`: 桌面大屏 UI 组件（包含 Shadcn 和 Icon）。
  - `@meowhomo/ui-mobile`: 面向移动设备（类似 Shadowrocket 风格布局）的组件库。
  - `@meowhomo/cesium`: 与 CesiumJS 和 Resium 的 WebGL 地球 3D 集成组件。
  - `@meowhomo/stores`: 前端全局状态状态管理 (Zustand)。
  - `@meowhomo/api`: 封装所有后端接口请求规范。
  - `@meowhomo/platform`: Web / Tauri 平台差异抽象层。在 Web 模式下走 HTTP 请求，在 Tauri 模式下走 `invoke()` 直调 Rust，上层代码无感知。

## 5. 部署架构

### 5.1 Linux 服务器部署

```text
┌─────────────────────────────────┐
│  systemd (meowhomo.service)     │
│  ┌──────────────┐ ┌───────────┐ │
│  │ Rust Server  │←│ Go Engine │ │
│  │ (axum:443)   │ │ (gRPC)    │ │
│  └──────┬───────┘ └───────────┘ │
│         │ 内嵌前端静态资源       │
│         │ (rust-embed)          │
└─────────┼───────────────────────┘
          │ HTTPS (ACME 自动证书)
       浏览器
```

- 使用 `goreleaser` 交叉编译多平台二进制
- `deploy/install.sh` 提供一键安装脚本
- systemd 管理进程生命周期

### 5.2 Docker 部署

```dockerfile
# 多阶段构建
FROM rust:latest AS rust-builder   # 编译 Rust 后端
FROM golang:latest AS go-builder   # 编译 Go 内核
FROM node:lts AS web-builder       # 构建前端静态资源
FROM debian:bookworm-slim          # 最终镜像，仅包含运行时
```

- 最终镜像体积控制在 50MB 以内
- 支持 `docker-compose.yml` 一键启动
- 数据卷挂载 `/data`（数据库 + 备份 + 证书）

### 5.3 桌面端部署 (Tauri v2)

- Tauri 将 Rust Server + Go Engine + 前端打包为单一安装包
- 产出格式：`.exe` (Win) / `.dmg` (Mac) / `.AppImage` + `.deb` (Linux)
- 通过 `@meowhomo/platform` 包自动切换为 Tauri Command 直调模式
