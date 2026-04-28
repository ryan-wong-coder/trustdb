# TrustDB .sproof v1 单文件证明格式

`.sproof` 是 TrustDB 当前推荐的证明交换格式。它把一次存证验证需要的证明材料放进一个确定性 CBOR 文件中，避免用户同时管理 `.tdproof`、`.tdgproof`、`.tdanchor-result` 多个文件。

## 格式边界

- 编码：确定性 CBOR，使用 `internal/cborx` 的 Core Deterministic Encoding。
- 顶层 schema：`trustdb.sproof.v1`。
- 顶层 format_version：`1`。
- 最大解码大小：16 MiB。
- 兼容性：v1 固定后，新增破坏性字段必须提升 schema 或 format_version。
- 安全边界：文件里的 `proof_level` 只是导出时的提示，验证器必须重新计算实际等级。

## 顶层结构

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `schema_version` | string | 是 | 固定为 `trustdb.sproof.v1`。 |
| `format_version` | uint | 是 | 当前固定为 `1`。 |
| `record_id` | string | 是 | 冗余索引字段，必须等于 `proof_bundle.record_id`。 |
| `proof_level` | string | 是 | 导出时可达到的最高等级提示，当前为 `L3`、`L4` 或 `L5`。验证时必须重算。 |
| `proof_bundle` | `ProofBundle` | 是 | L1-L3 必需证明材料。 |
| `global_proof` | `GlobalLogProof` | 否 | L4 材料，证明 batch root 已进入 Global Log 的某个 STH。 |
| `anchor_result` | `STHAnchorResult` | 否 | L5 材料，证明对应 STH/global root 已被外部 anchor。 |
| `exported_at_unix_nano` | int | 否 | 导出时间，纳秒 Unix 时间戳。为 0 时不影响验证。 |

## 等级降级语义

`.sproof` 不强制必须达到 L5。验证器按文件内实际证据重新计算等级：

| 文件内容 | 验证结果上限 |
| --- | --- |
| 只有 `proof_bundle` | L3 |
| `proof_bundle` + `global_proof` | L4 |
| `proof_bundle` + `global_proof` + `anchor_result` | L5 |
| `anchor_result` 缺少 `global_proof` | 非法文件，拒绝验证 |

`anchor_result` 不能跳过 `global_proof` 直接提升到 L5，因为 L5 锚定的是 STH/global root，不是 batch root。

## 验证算法

离线验证器应按以下顺序处理：

1. 解码确定性 CBOR，拒绝重复 map key、tag、未知字段、超限大小。
2. 检查 `schema_version == trustdb.sproof.v1`。
3. 检查 `format_version == 1`。
4. 检查 `record_id == proof_bundle.record_id`。
5. 检查 `proof_level` 与内嵌证据重新计算出的等级一致。
6. 用原始内容文件、客户端公钥、服务端公钥验证 `ProofBundle`，得到 L3。
7. 如果存在 `global_proof`，验证 batch root 到 STH 的 Global Log inclusion proof，成功后得到 L4。
8. 如果存在 `anchor_result`，验证它与 `global_proof.sth` 的 `tree_size`、`root_hash` 和 sink 约束一致，成功后得到 L5。
9. 最终输出以重新计算结果为准，不信任文件里的 `proof_level`。

## CLI

推荐入口：

```powershell
go run ./cmd/trustdb verify `
  --file .\example.txt `
  --sproof .trustdb-dev\example.sproof `
  --server-public-key .trustdb-dev\server.pub `
  --client-public-key .trustdb-dev\client.pub
```

如果要忽略 `.sproof` 内的 L5 anchor，只验证到 L4：

```powershell
go run ./cmd/trustdb verify `
  --file .\example.txt `
  --sproof .trustdb-dev\example.sproof `
  --server-public-key .trustdb-dev\server.pub `
  --client-public-key .trustdb-dev\client.pub `
  --skip-anchor
```

分步验证入口仍然保留给审计和调试：

```powershell
go run ./cmd/trustdb verify `
  --file .\example.txt `
  --proof .trustdb-dev\example.tdproof `
  --global-proof .trustdb-dev\example.tdgproof `
  --anchor .trustdb-dev\example.tdanchor-result `
  --server-public-key .trustdb-dev\server.pub `
  --client-public-key .trustdb-dev\client.pub
```

## 测试向量

仓库内置一个 L3 schema 向量，用来锁定 v1 顶层 CBOR 编码：

- 文件：`test/vectors/sproof-v1-l3.cbor`
- SHA-256：`test/vectors/sproof-v1-l3.sha256`

更新规则：只有在确认 `.sproof v1` 格式有意变更时，才允许执行：

```powershell
$env:TRUSTDB_UPDATE_VECTORS='1'
go test ./internal/sproof
```

如果只是验证当前实现，不要设置该环境变量。
