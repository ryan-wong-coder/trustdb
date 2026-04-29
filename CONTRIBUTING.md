# TrustDB 贡献指南 / Contributing Guide

TrustDB 是一个单机可验证存证数据库。贡献代码时，最重要的不是遵守某个表面风格，而是保护证明语义、持久化边界、恢复能力和大规模数据路径。本指南面向功能开发、Bug 修复、重构、文档、测试和发布维护。

This document is bilingual. The Chinese section is authoritative for day-to-day project conventions; the English section mirrors the same expectations for international contributors.

## 中文版

### 贡献原则

- Issue first：任何开发、重构、文档发布或行为变更都先创建 Issue，写清背景、目标、非目标、风险和验收标准。
- Small pull requests：一个 PR 只解决一个 Issue，不混入无关格式化、实验代码、本地数据或顺手重构。
- Current implementation only：README、贡献指南和用户文档只能描述已经实现的能力，不把路线图写成现状。
- Tests protect semantics：涉及证明、存储、恢复、API、SDK 或桌面客户端行为时，测试要覆盖语义边界和失败路径，而不只覆盖 happy path。
- Review before merge：`main` 受 CODEOWNERS 和分支保护约束，合并前需要通过 CI 和维护者 review。
- Security by default：私钥、生产配置、本地数据库、备份包、构建产物和部署状态不得进入 Git。

### 仓库架构地图

TrustDB 当前是单机架构，但内部按可信边界拆分。贡献前请先判断你的改动落在哪个边界内。

```text
cmd/trustdb
  CLI entrypoint and server wiring

internal/model
  Canonical proof, receipt, STH, anchor, backup, and API data models

internal/cborx + internal/trustcrypto
  Deterministic CBOR encoding and cryptographic signing/hash utilities

internal/claim + internal/receipt + internal/verify + internal/prooflevel
  Claim construction, accepted receipts, local verification, and L1-L5 semantics

internal/ingest + internal/wal
  Bounded ingest queue, accepted-write durability, WAL replay, fsync policy, checkpoints

internal/batch + internal/merkle
  Batch construction, Merkle roots, proof bundles, committed root metadata

internal/globallog
  Global Transparency Log, STH generation, inclusion proofs, consistency proofs, tiles/frontier state

internal/anchor
  STH/global-root anchor outbox, sinks, retry/upgrade behavior, OpenTimestamps integration

internal/proofstore + internal/proofstore/pebble
  File and Pebble proofstore implementations, indexes, cursor/range access, persisted artifacts

internal/httpapi + internal/grpcapi
  Public service surfaces for HTTP and gRPC, preserving shared proof object semantics

internal/backup + internal/sproof
  Portable backup/restore and single-file proof exchange formats

sdk
  Public Go SDK used by automation and the desktop client transport layer

clients/desktop
  Wails + Vue desktop app, local identity, server settings, Pebble-backed local record index, proof export/verify

configs + formats + scripts
  Supported configuration profiles, stable public file-format docs, and release/development helpers
```

### 数据流和可信边界

```text
File -> SignedClaim -> AcceptedReceipt -> ProofBundle -> GlobalLogProof -> STHAnchorResult
       L1             L2                L3             L4              L5
```

- L1：客户端对包含文件哈希和元数据的 claim 签名。
- L2：服务端校验 claim，并在 WAL durable boundary 内接受。
- L3：batch worker 将 accepted records 提交进 Merkle batch，生成 `ProofBundle`。
- L4：batch root 进入 Global Transparency Log，并能证明其包含在某个 STH 中。
- L5：对应 STH/global root 被外部 anchor sink 锚定。

关键边界：

- L4 不等于外部锚定。L4 只证明 batch root 已进入 global log。
- L5 只锚定 STH/global root，不恢复 per-batch root 直锚模式。
- `.sproof` 是主要交换格式；`.tdproof`、`.tdgproof`、`.tdanchor-result` 是高级分步格式。
- 验证器必须独立复算 proof、hash 和 signature，不能信任服务端返回的等级标签。

### 变更类型与架构责任

| 变更范围 | 主要关注点 | 常见测试 |
| --- | --- | --- |
| Proof/model/encoding | 确定性 CBOR、hash/signature input、兼容性声明、篡改失败测试 | `go test ./...`，格式/验证回归测试 |
| WAL/ingest/replay | durable boundary、幂等 replay、checkpoint、崩溃窗口、内存峰值 | unit、race、integration |
| Batch/global log | Merkle root 稳定性、proof 可恢复、consistency proof、避免 O(N) 主路径 | unit、benchmark、integration/e2e |
| Proofstore/Pebble | key schema、二级索引、cursor/range API、file backend 降级语义 | proofstore tests、large-path tests |
| Anchor/OTS | STH-only 语义、outbox retry、next_attempt_at、外部服务失败隔离 | unit、integration with fake sink |
| HTTP/gRPC/SDK | transport parity、错误可自动化判断、CBOR payload 语义一致 | unit、e2e、SDK tests |
| Desktop client | Go SDK transport、Pebble-backed local index、分页/虚拟列表、`.sproof` 主路径 | desktop Go tests、frontend build、manual smoke |
| Backup/restore | 流式处理、entry hash、resume checkpoint、重复 restore 幂等 | backup tests、interruption tests |
| Documentation | 只描述当前实现、同步中英入口、命令和路径可验证 | `git diff --check` |

### 架构不变量

#### 证明语义

- L1-L5 的含义必须在 CLI、HTTP、gRPC、SDK、桌面客户端和验证器之间保持一致。
- 任何 proof object 的字段、编码、hash input 或 signature input 变化，都必须明确兼容性边界。
- 安全失败不得被降级成成功、unknown 或仅 UI 警告。
- 回归测试应覆盖被篡改 artifact、错误公钥、错误 batch root、错误 STH、缺失 anchor 等失败路径。

#### 持久化与恢复

- WAL accepted、batch committed、global log appended、anchor published 是不同耐久边界。
- replay、outbox retry、backup restore 必须幂等。
- 启动恢复不能一次性加载全量 WAL、records、roots、global leaves、STH、anchors 或 backup entries。
- 修改 fsync、checkpoint、segment cleanup、repair 或 restore 逻辑时，PR 必须说明崩溃窗口和恢复语义。

#### 可扩展路径

- 生产路径默认按 Pebble proofstore 设计；file proofstore 只承诺开发和小规模演示。
- 列表、pending scan、anchor upgrade、backup export/restore、global proof 生成必须优先使用 cursor/range/status index。
- HTTP、CLI、SDK、desktop 主路径中不要重新引入全量扫描、全量排序或全量内存聚合。
- Global Log proof 应读取必要 node/tile/range；不要为单个 proof 重建整棵树。

#### 接口一致性

- HTTP 和 gRPC 只是 transport 差异，不能改变 proof object 语义。
- Go SDK 是客户端复用服务能力的主入口；桌面客户端不应重新扩散手写 HTTP 语义。
- Wails 暴露 DTO 不使用 Go-only 类型；时间、大小、状态字段要有明确单位和稳定编码。
- CLI exit behavior、HTTP status、gRPC error 和 SDK error 应能被自动化脚本可靠判断。

#### 桌面客户端

- 本地 records 使用索引化存储，不退回单 JSON 全量读写。
- 列表、搜索、详情、批量刷新按需加载，面向十万级本机记录保持可用。
- `.sproof` 是主导出入口；分步 proof 文件保留为高级功能。
- UI 改动需要考虑小窗口、长文件名、长 hash、加载中、失败状态和离线验证路径。

### Issue 和 PR 要求

Issue 应包含：

- 背景和真实问题。
- 目标和非目标。
- 影响的架构边界。
- 安全、兼容性、迁移或恢复影响。
- 验收标准和测试计划。

PR 应包含：

- 关联 Issue，例如 `Refs #99`、`Fixes #99` 或 `Closes #99`。
- 变更摘要。
- 测试结果。
- 风险和回滚方式。
- 如果未运行某些相关测试，说明原因和残余风险。

不要在 PR 中删除风险字段，也不要用“本地能跑”替代 CI 失败分析。

### 质量门

根据变更范围选择测试。后端主路径通常需要：

```powershell
go test ./...
go test -race ./...
go test -tags=integration ./...
go test -tags=e2e ./...
```

桌面客户端相关变更通常需要：

```powershell
cd clients/desktop
go test ./...
go test -race ./...
cd frontend
npm run build
```

完整桌面安装包变更需要额外验证：

```powershell
cd clients/desktop
wails build -clean -nsis
```

CI 当前覆盖 repository hygiene、backend unit/race/integration/e2e、desktop Go tests 和 desktop frontend build。CI 失败时先定位差异，再决定修复、拆分或记录残余风险。

### 文档边界

- `README.md` 和 `README.zh-CN.md` 是用户入口，只写当前已经实现的系统能力。
- `CONTRIBUTING.md` 维护贡献流程、架构边界和质量门。
- `formats/` 记录稳定公开文件格式，例如 `.sproof`。
- `configs/` 中的配置示例应与真实实现保持一致。
- `doc/` 和 `docs/` 是本地草稿目录，不提交到仓库。

### 安全、依赖和许可证

- 不提交真实密钥、token、生产配置、本地数据库、`.tdbackup`、`.sproof` 样本中的敏感内容或构建产物。
- 新依赖必须说明用途、许可证兼容性和供应链风险。
- TrustDB 使用 AGPL-3.0-only；提交贡献即表示贡献按该许可证发布。
- 安全问题优先私下报告给维护者，不要在公开 Issue 中泄露可利用细节。

### 维护者合并检查

合并前确认：

- Issue 和 PR 范围一致。
- CI 已通过，或残余风险被明确记录并被维护者接受。
- 证明语义、持久化边界和大规模路径没有退化。
- 文档只描述当前实现。
- 没有提交本地数据、密钥、构建产物或草稿文档。
- CODEOWNERS review 要求已满足。

---

## English Version

### Contribution Principles

- Issue first: every feature, bug fix, refactor, documentation release, or behavior change starts with a GitHub Issue that states context, goals, non-goals, risks, and acceptance criteria.
- Small pull requests: one PR solves one Issue. Do not mix unrelated formatting, experiments, local data, or opportunistic refactors.
- Current implementation only: README files, user-facing docs, and this guide must describe implemented behavior, not roadmap items.
- Tests protect semantics: changes touching proofs, storage, recovery, APIs, SDKs, or the desktop client must test semantic boundaries and failure paths, not only happy paths.
- Review before merge: `main` is protected by CODEOWNERS and branch protection. CI and maintainer review are required before merge.
- Security by default: private keys, production configs, local databases, backup bundles, build artifacts, and deployment state must not be committed.

### Repository Architecture Map

TrustDB is currently a single-node system, but the implementation is split by trust and durability boundaries. Before changing code, identify which boundary you are touching.

```text
cmd/trustdb
  CLI entrypoint and server wiring

internal/model
  Canonical proof, receipt, STH, anchor, backup, and API data models

internal/cborx + internal/trustcrypto
  Deterministic CBOR encoding and cryptographic signing/hash utilities

internal/claim + internal/receipt + internal/verify + internal/prooflevel
  Claim construction, accepted receipts, local verification, and L1-L5 semantics

internal/ingest + internal/wal
  Bounded ingest queue, accepted-write durability, WAL replay, fsync policy, checkpoints

internal/batch + internal/merkle
  Batch construction, Merkle roots, proof bundles, committed root metadata

internal/globallog
  Global Transparency Log, STH generation, inclusion proofs, consistency proofs, tiles/frontier state

internal/anchor
  STH/global-root anchor outbox, sinks, retry/upgrade behavior, OpenTimestamps integration

internal/proofstore + internal/proofstore/pebble
  File and Pebble proofstore implementations, indexes, cursor/range access, persisted artifacts

internal/httpapi + internal/grpcapi
  Public service surfaces for HTTP and gRPC, preserving shared proof object semantics

internal/backup + internal/sproof
  Portable backup/restore and single-file proof exchange formats

sdk
  Public Go SDK used by automation and the desktop client transport layer

clients/desktop
  Wails + Vue desktop app, local identity, server settings, Pebble-backed local record index, proof export/verify

configs + formats + scripts
  Supported configuration profiles, stable public file-format docs, and release/development helpers
```

### Data Flow and Trust Boundaries

```text
File -> SignedClaim -> AcceptedReceipt -> ProofBundle -> GlobalLogProof -> STHAnchorResult
       L1             L2                L3             L4              L5
```

- L1: the client signs a claim containing file hash and metadata.
- L2: the server validates the claim and accepts it within the WAL durability boundary.
- L3: the batch worker commits accepted records into a Merkle batch and emits a `ProofBundle`.
- L4: the batch root is included in the Global Transparency Log and can be proven against an STH.
- L5: the corresponding STH/global root is anchored by an external anchor sink.

Important boundaries:

- L4 is not external anchoring. It only proves inclusion in the global log.
- L5 anchors only STH/global roots. Do not reintroduce direct per-batch root anchoring.
- `.sproof` is the primary exchange format. `.tdproof`, `.tdgproof`, and `.tdanchor-result` are advanced split artifacts.
- Verifiers must recompute proofs, hashes, and signatures independently. They must not trust server-provided level labels.

### Change Areas and Architectural Responsibilities

| Area | Main responsibility | Typical checks |
| --- | --- | --- |
| Proof/model/encoding | Deterministic CBOR, hash/signature inputs, compatibility boundaries, tamper-failure tests | `go test ./...`, format and verifier regression tests |
| WAL/ingest/replay | Durable boundary, idempotent replay, checkpoints, crash windows, memory usage | unit, race, integration |
| Batch/global log | Stable Merkle roots, recoverable proofs, consistency proofs, no O(N) hot paths | unit, benchmark, integration/e2e |
| Proofstore/Pebble | Key schema, secondary indexes, cursor/range APIs, file backend downgrade semantics | proofstore tests, large-path tests |
| Anchor/OTS | STH-only semantics, outbox retry, `next_attempt_at`, external failure isolation | unit, fake-sink integration |
| HTTP/gRPC/SDK | Transport parity, scriptable errors, shared CBOR payload semantics | unit, e2e, SDK tests |
| Desktop client | Go SDK transport, Pebble-backed local index, paginated/virtualized lists, `.sproof` main path | desktop Go tests, frontend build, manual smoke |
| Backup/restore | Streaming entries, entry hash, resume checkpoints, idempotent repeated restore | backup tests, interruption tests |
| Documentation | Implemented behavior only, bilingual entry points, verifiable commands and paths | `git diff --check` |

### Architectural Invariants

#### Proof Semantics

- L1-L5 meanings must stay consistent across CLI, HTTP, gRPC, SDK, desktop UI, and verifiers.
- Changes to proof object fields, encoding, hash inputs, or signature inputs must state compatibility boundaries.
- Security failures must not be downgraded to success, unknown, or a UI-only warning.
- Regression tests should cover tampered artifacts, wrong public keys, wrong batch roots, wrong STHs, and missing anchors.

#### Durability and Recovery

- WAL accepted, batch committed, global log appended, and anchor published are separate durability boundaries.
- Replay, outbox retry, and backup restore must be idempotent.
- Startup recovery must not load all WAL records, records, roots, global leaves, STHs, anchors, or backup entries into memory.
- PRs changing fsync, checkpoints, segment cleanup, repair, or restore logic must explain crash windows and recovery semantics.

#### Scalable Paths

- Production paths are designed around the Pebble proofstore. The file proofstore is for development and small demos.
- Lists, pending scans, anchor upgrades, backup export/restore, and global proof generation should use cursor/range/status indexes.
- Do not reintroduce full scans, global sorting, or all-record in-memory aggregation into HTTP, CLI, SDK, or desktop hot paths.
- Global Log proofs should read necessary nodes, tiles, or ranges. Do not rebuild the whole tree for a single proof.

#### Interface Consistency

- HTTP and gRPC are transport choices; they must not change proof object semantics.
- The Go SDK is the primary reusable client surface. The desktop client should not spread independent handwritten HTTP semantics.
- Wails DTOs must avoid Go-only types. Time, size, and state fields need explicit units and stable encoding.
- CLI exit behavior, HTTP status, gRPC errors, and SDK errors should be reliable for automation.

#### Desktop Client

- Local records must remain indexed. Do not return to one large JSON file for all records.
- Lists, search, details, and batch refresh should load on demand and remain usable with hundreds of thousands of local records.
- `.sproof` is the primary export path. Split proof artifacts remain advanced functionality.
- UI changes should account for narrow windows, long filenames, long hashes, loading states, failure states, and offline verification.

### Issue and Pull Request Expectations

Issues should include:

- Context and the real problem.
- Goals and non-goals.
- Affected architecture boundaries.
- Security, compatibility, migration, or recovery impact.
- Acceptance criteria and test plan.

Pull requests should include:

- Linked Issue, such as `Refs #99`, `Fixes #99`, or `Closes #99`.
- Change summary.
- Test results.
- Risks and rollback notes.
- If relevant checks were not run, explain why and state residual risk.

Do not delete risk fields from PR templates, and do not use “works locally” as a substitute for CI failure analysis.

### Quality Gates

Choose checks based on the touched boundary. Backend hot-path changes usually require:

```powershell
go test ./...
go test -race ./...
go test -tags=integration ./...
go test -tags=e2e ./...
```

Desktop client changes usually require:

```powershell
cd clients/desktop
go test ./...
go test -race ./...
cd frontend
npm run build
```

Desktop installer changes also require:

```powershell
cd clients/desktop
wails build -clean -nsis
```

CI currently runs repository hygiene, backend unit/race/integration/e2e tests, desktop Go tests, and the desktop frontend build. When CI fails, identify the difference first, then fix, split, or document the residual risk.

### Documentation Boundaries

- `README.md` and `README.zh-CN.md` are user entry points and must describe only implemented behavior.
- `CONTRIBUTING.md` maintains contribution workflow, architecture boundaries, and quality gates.
- `formats/` documents stable public file formats, such as `.sproof`.
- `configs/` examples must stay aligned with real implementation.
- `doc/` and `docs/` are local draft directories and must not be committed.

### Security, Dependencies, and License

- Do not commit real keys, tokens, production configs, local databases, `.tdbackup` files, sensitive `.sproof` samples, or build artifacts.
- New dependencies must explain purpose, license compatibility, and supply-chain risk.
- TrustDB is licensed under AGPL-3.0-only. By contributing, you agree that your contribution is released under that license.
- Report security issues privately to maintainers first. Do not disclose exploitable details in public Issues.

### Maintainer Merge Checklist

Before merging, confirm that:

- Issue and PR scope match.
- CI passed, or residual risk is explicitly documented and accepted.
- Proof semantics, durability boundaries, and scalable paths did not regress.
- Documentation describes current implementation only.
- No local data, secrets, build artifacts, or draft docs are committed.
- CODEOWNERS review requirements are satisfied.
