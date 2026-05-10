# GZ-14 游戏后台（局外系统）

基于 Go 的 MMO 游戏后台服务端，支持千人同图在线的场景管理、AOI（Area of Interest）网格系统、实时移动广播。

## 架构

```
┌─────────────────────────────────────────────────────────────┐
│                        Bot 压测端                            │
│               (internal/bot/, TCP 模拟客户端)                 │
└──────────────────────┬──────────────────────────────────────┘
                       │ TCP (Length+Protobuf)
          ┌────────────┴────────────┐
          │    Lobby :19001         │
          │  鉴权/登录/创角/Session  │
          └────────────┬────────────┘
                       │ 接口注入 (SceneAPI/LobbyAPI)
          ┌────────────┴────────────┐
          │    Scene :19002         │
          │   AOI/移动广播/场景管理  │
          └────────────┬────────────┘
                       │
          ┌────────────┴────────────┐
          │    DB 服务 (内嵌)        │
          │  ┌──────┐ ┌──────────┐  │
          │  │MySQL │ │  Redis   │  │
          │  │持久化 │ │ 缓存/TTL │  │
          │  └──────┘ └──────────┘  │
          └─────────────────────────┘
```

**设计要点：**
- 三服务架构：Lobby（连接/鉴权） + Scene（场景/AOI） + DB（持久化），通过接口注入解耦
- Grid-based AOI：1000×1000 地图，100×100 网格，3×3 邻居广播
- Protobuf 序列化，二进制协议（Length+MsgID+Sequence+ErrCode+Body）
- Redis 缓存 Session/在线状态/实时位置，MySQL 持久化角色数据（写回策略）

## 技术栈

| 组件 | 用途 |
|------|------|
| Go 1.21+ | 服务端语言，goroutine-per-connection 模型 |
| Protobuf (proto3) | 消息序列化 |
| MySQL + GORM | 持久化存储 |
| Redis (go-redis) | 缓存、Session、在线状态 |
| bcrypt | 密码加密 |

## 快速开始

### 前置条件

- Go ≥ 1.21
- MySQL 8.0+（运行于 localhost:3306）
- Redis 6+（运行于 localhost:6379）
- protoc（如需修改 .proto）

### 启动

```bash
# 1. 编译 protobuf（首次或修改 proto 后）
make proto

# 2. 编译所有二进制
make build

# 3. 启动 MySQL 和 Redis，然后启动服务端
make run
```

### 运行测试

```bash
# 单元测试
go test ./internal/network/... ./internal/scene/... -v

# 10 个 Bot E2E 测试
make test-bot

# 100 个 Bot 压测
make bench-bot

# 登录压测（200 Bot）
make bench-login

# 反复登录登出
make bench-relogin
```

### 高级参数

```bash
# 自定义 Bot 测试
go run cmd/bot/main.go \
  -addr 127.0.0.1:19001 \
  -bots 500 \
  -rate 30 \
  -move-ms 200 \
  -duration-sec 30 \
  -mode full

#   mode full    - 注册→登录→创角→进场景→移动→登出
#   mode login   - 仅登录测试吞吐量
#   mode move    - 只测移动广播
#   mode relogin - 多轮反复登录登出
#   mode reconnect - 断线重连 E2E 测试

# 性能分析
go run cmd/server/main.go -pprof
# pprof 可通过 http://localhost:6060 访问
```

## 项目结构

```
├── cmd/
│   ├── server/main.go       # 服务端入口
│   └── bot/main.go          # 压测工具入口
├── internal/
│   ├── network/             # 网络层
│   │   ├── conn.go          # TCP 连接封装
│   │   ├── server.go        # TCP 服务器
│   │   ├── packet.go        # 封包/解包（粘包半包）
│   │   ├── codec.go         # Protobuf 编解码
│   │   └── msgid.go         # 消息 ID 定义
│   ├── lobby/
│   │   └── server.go        # Lobby 服务（登录/登出/创角）
│   ├── scene/
│   │   ├── server.go        # Scene 服务（移动/AOI）
│   │   ├── scene.go         # 场景管理器
│   │   ├── grid.go          # Grid AOI 算法
│   │   └── entity.go        # 实体管理器
│   ├── bot/
│   │   ├── client.go        # Bot 客户端
│   │   └── manager.go       # Bot 管理器
│   ├── db/
│   │   ├── manager.go       # DB 管理（MySQL+Redis）
│   │   ├── models.go        # GORM 模型
│   │   ├── redis.go         # Redis 客户端
│   │   └── errors.go        # 错误定义
│   └── protocol/            # 生成的 pb.go 文件
├── proto/
│   └── gz14.proto           # Protobuf 定义
├── pkg/config/
│   └── config.go            # 配置
├── docs/                    # 文档
└── Makefile
```

## 协议

二进制协议格式：

```
[Length:4B][MsgID:2B][Sequence:4B][ErrCode:2B][Body:NB]
```

消息类别：

| 类别 | MsgID 范围 | 服务 |
|------|-----------|------|
| 登录/鉴权 | 0x01xx | Lobby |
| 场景/移动 | 0x02xx | Scene |
| AOI | 0x03xx | Scene |
| 内部 RPC | 0xE0xx | Lobby↔Scene |

## 性能

> 测试机器：MacBook Pro (Apple M2, 16 GB RAM)，服务端与压测端同机部署

| 指标 | 实测值 | 目标 |
|------|-------|------|
| 1000 并发连接 (100% 登录) | ✅ 1000 bots @20/s, P99=903ms | ≥ 1000 |
| 登录 P50/P95/P99 (1000 bots) | 127/598/903ms ✅ | P99 < 1s |
| 广播吞吐量 | ~45K msg/s ✅ | ≥ 5000 msg/s |
| 混合压力 (300 移动+300 登录并发) | ✅ 无崩溃, 2.78M 广播接收 | 无崩溃 |
| 断线重连 | ✅ 3/3 成功, <50ms | Token 续期+场景恢复 |
| 心跳超时清理 | ✅ 30s 超时自动清理 | 资源及时回收 |
| 服务端内存 | ~67 MB RSS ✅ | < 2 GB |

## 详细文档

- [需求分析文档](docs/需求分析文档.md)
- [技术选型文档](docs/技术选型文档.md)
- [测试方案](docs/测试方案.md)
- [功能点开发日志](docs/功能点开发日志.md)
- [测试报告](docs/测试报告.md)
- [测试日志归档](docs/test-logs/)
