# TrustDB 分布式与存算分离架构说明（ADR）

## 状态

草案，与实现同步演进。不修改本文件中的「非目标」除非经显式架构评审。

## 背景

TrustDB 单机路径：`ingest` → WAL → `batch` → proofstore（file/Pebble）→ Global Log outbox → `globallog` → anchor。`proofstore.Store`（见 `internal/proofstore/store.go`）是证明与 Global Log 元数据的统一持久化边界。

## 决策

### 1. TiKV 作为完整 proofstore / KV 后端

- TiKV **不是**旁路元数据目录或仅索引服务，而是可选的 **proofstore 后端**，需实现与 file/Pebble 相同的 `proofstore.Store` 契约（含 `CommitGlobalLogAppend`、`BatchArtifactWriter` 等）。
- 配置通过 `metastore: tikv` 与 PD 地址等参数选择（见 `internal/config` 与 `internal/proofstore/factory.go`）。当前代码只保留配置与包边界，原生 TiKV proofstore 未完成前，`metastore=tikv` 必须显式启动失败，不能退化为本地临时缓存。
- 值编码与 Pebble/file 一致：确定性 CBOR + 与 Pebble 相同的 bundle 信封语义，便于备份与迁移。

### 2. 存算分离

- **计算节点**：运行 TrustDB 进程（HTTP/gRPC、ingest、batch、签名、WAL 等）；默认仍使用**每节点本地 WAL**。
- **存储层**：多计算节点可连接**同一 TiKV 集群**共享 proofstore 数据；吞吐通过多节点 + 客户端侧负载均衡扩展。
- **不引入**服务端公共节点注册表、latest STH 目录、record 路由目录或集群查询聚合 API。

### 3. 来源标识（node_id / log_id）

- 每条对外证明链上的对象须能标明**哪一计算节点**与**哪一本地 Global Log** 产生（`node_id` / `log_id`），避免共享 TiKV 后语义混淆。
- `tree_size`、`batch_id`、STH 仅在对应 `(node_id, log_id)` 作用域内可比较；不得暗示存在全局唯一 Global Log。
- `.sproof` 离线验证仅依赖文件内材料与公钥，**不依赖**在线目录或注册表。

### 4. 客户端与 SDK 负载均衡

- 多 endpoint、重试、故障转移由 **SDK / 桌面客户端**配置与实现；服务端保持现有单实例 API 形态（可附加只读来源字段）。

### 5. Global Log 正确性与 TiKV

- Global Log append 的原子性由 **proofstore 后端事务**（TiKV 侧一次提交多键）保证，与现有 `CommitGlobalLogAppend` 语义一致。
- **不**使用 NATS 或其它消息系统作为 Global Log 顺序或提交真相来源。

## 并发与同一 log_id

- **默认**：每个计算节点使用独立 `log_id`；多节点并发写同一 TiKV 时通过 key 前缀（含 `log_id`）隔离。
- **同一 `log_id` 多写者 active-active**：必须先有 TiKV lease / fencing / CAS 等显式设计，不在首版范围。

## 非目标

- 全集群单一 Global Log 或全局 append 锁。
- 服务端节点发现、健康目录、或统一查询网关。
- 将 TiKV 仅用作「元数据索引」而 proof bundle 仍只存本地盘（与上述决策 1 矛盾）。

## 验证

- proofstore conformance：原生 TiKV 后端实现后，`internal/proofstore/proofstoretest` 必须对 TiKV 后端（在提供测试用 PD/TiKV 时）与 Pebble 一致。
- CI 默认环境无 TiKV 时，TiKV 原生 conformance 测试应 `Skip`，不阻断 `go test ./...`。

## 参考文件

- `internal/proofstore/store.go`
- `internal/proofstore/factory.go`
- `internal/proofstore/tikv/`（TiKV 实现）
- `formats/SPROOF_V1.md`（单文件证明；v2 来源字段见 `internal/sproof` 与模型变更）
