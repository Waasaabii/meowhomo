# 🐱 MeowHomo

Clash 订阅转化 & 策略管理面板。Rust + Go 混合架构，全 Web 管理。

## 架构

```
React + Tailwind v3 + shadcn/ui + CesiumJS
  │ Tauri: command 直调 │ Web: HTTP/SSE
Rust 后端 (axum) — 业务+TLS+反代+SSE+DB
  │ gRPC
Go 内核管理器 (import mihomo) — 引擎控制
```

## 项目结构

```
meowhomo/
├── crates/                    # Rust
│   ├── server/                # axum Web 入口
│   ├── core/                  # 核心业务逻辑
│   └── tauri-plugin/          # Tauri command [预留]
├── engine/                    # Go 内核管理器
├── apps/web/                  # React 前端
├── packages/                  # 前端复用包
│   ├── ui/                    # 桌面端组件
│   ├── ui-mobile/             # 移动端组件
│   ├── cesium/                # CesiumJS 3D 地球
│   ├── api/                   # REST + SSE 客户端
│   ├── stores/                # zustand 状态管理
│   └── platform/              # Web/Tauri 平台抽象
├── proto/                     # gRPC 接口定义
├── deploy/                    # 部署配置
└── docs/                      # 文档
```

## 开发

```bash
# 前端
cd apps/web && bun install && bun dev

# Rust 后端
cargo run -p meowhomo-server

# Go 内核
cd engine && go run .
```

## 许可证

MIT
