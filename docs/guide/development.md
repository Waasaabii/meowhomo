# MeowHomo 研发规约指南

本文件主要指导二次开发、依赖更新、源码溯源以及发行上线。

## 1. 外部开源项目的引入与化用映射表

我们虽不重新造轮子，但要求高度掌控代码逻辑，避免一揽子引入引发不可控的破坏。必须遵循以下的对应引用规范：

| 溯源参考仓库对象 | 归属目录 | 目标复用模块设计 | 具体用途 |
| --- | --- | --- | --- |
| `MetaCubeX/mihomo` | `engine/` Go 端 | 原生核心层 | 作为标准的 `go.mod` package 引入内嵌。提供主要的代理协议和底漆策略（fallback, urltest）。配置结构反向指导 Rust 配置下发结构设计。 |
| `nitezs/sub2clash` | `engine/` Go 端 | URL 转换层 | 不引入全量依赖，仅抽取并复刻 `parser/` 目录下的解析层代码（处理 ss、vmess、trojan 的 Base64 深潜解密编码与转译），化用即可。 |
| `SagerNet/sing-box`| `engine/` Go 端 | Inbound 模型参考 | 特殊关注并参考它的 inbound 内部结构设计，用于“**本机节点新建逻辑**”。 |
| `Wei-Shaw/sub2api` | Rust / `deploy/` | 构建打包和安装 | 提取了其非常成熟的 `.goreleaser.yaml`，并借用它的 `deploy/install.sh` 系统脚本模板重写本项目的 Linux 全自动下发脚本。借用 `setup/` （安装精灵指导步进式初始化）的交互思维指导我们在 Rust 里书写 Setup Web 向导的流程控制。 |

### 1.1 文件级引用细节

```text
engine/ (Go 内核管理器)
├── go.mod
│   ├── require github.com/MetaCubeX/mihomo  ← mihomo 引擎
│   └── require github.com/nitezs/sub2clash  ← 订阅解析(可选，或复制代码)
├── mihomo_engine.go    ← 参考 tem/mihomo/hub/              引擎启动/停止/重载
├── config_builder.go   ← 参考 tem/mihomo/config/           配置结构体定义
├── node_parser.go      ← 参考 tem/sub2clash/parser/        ss/vmess/trojan 协议解析
└── inbound.go          ← 参考 tem/sing-box/inbound/        本机节点 Trojan 入站创建

crates/server/ (Rust 后端)
├── setup.rs            ← 参考 tem/sub2api/internal/setup/  初始化向导 Web 流程

deploy/
├── install.sh          ← 参考 tem/sub2api/deploy/install.sh  一键安装脚本
├── Dockerfile          ← 参考 tem/sub2api/Dockerfile         多阶段构建
└── .goreleaser.yaml    ← 参考 tem/sub2api/.goreleaser.yaml   多平台编译发布
```

## 2. 第三方协议组件自动化与上游同步机制

如何确保应用支持的 Proxy 协议或者核心不落后于上游？

1. **GitHub Dependabot (主动发现)**
   - 依赖文件（`Cargo.toml`, `go.mod`, `package.json`）均接入 Dependabot 机器人。设定 Weekly/Monthly 的更新监控，自动触发提交 PR。
2. **CI 定期验证探针**
   - 设定每日或每周的 Github Actions 检查 `mihomo` 在 Github Releases 上发出的最新 Tag。
   - 检测到高版本即触发本项目内部测试编译（冒烟测试 gRPC 是否失联，Yaml 解析结构是否变更破坏等）。自动化合并后提供“**未发布构建(Nightly/Pre-release)**”。
3. **后台检查提醒交互用户**
   - 每次面板重启或每间隔一定时间，拉取 Github API 比对当前我们打包进来的 `mihomo` 版本序列号。若大幅滞后，在 Web Dashboard的【设置页】出现醒目的红点标记：“mihomo 引擎存在可用新版本升级”。由管理员一键执行后台拉取更新。

## 3. DevOps 部署与集成

### 3.1 Linux 二进制部署 (goreleaser + systemd)

```bash
# 一键安装
$ curl -sSL https://raw.githubusercontent.com/Waasaabii/meowhomo/main/deploy/install.sh | bash

# 服务管理
$ systemctl start meowhomo
$ systemctl enable meowhomo
```

### 3.2 Docker 部署

```bash
# 使用 docker-compose
$ docker compose up -d

# 或直接 docker run
$ docker run -d \
    -p 443:443 -p 10001-10010:10001-10010 \
    -v meowhomo-data:/data \
    ghcr.io/waasaabii/meowhomo:latest
```

`Dockerfile` 采用多阶段构建：
1. **rust-builder**：编译 Rust 后端二进制
2. **go-builder**：编译 Go 内核二进制
3. **web-builder**：构建前端静态资源
4. **runtime**：基于 `debian:bookworm-slim`，仅拷入产物，镜像 < 50MB

### 3.3 桌面端 (Tauri v2)

```bash
# CI 环境中构建
$ pnpm tauri build
# 产出: .exe (Win) / .dmg (Mac) / .AppImage + .deb (Linux)
```
