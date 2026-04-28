# TrustDB 代码贡献指南

这份文档说明 TrustDB 仓库的代码贡献方式。它面向后续的功能开发、Bug 修复、重构、文档更新、测试补齐和工程质量收敛。

TrustDB 当前采用 Issue-first 流程：先把问题和验收标准写清楚，再开发。

## 基本原则

- 先提 Issue，再写代码。没有 Issue 的开发不进入实现阶段。
- 每个变更都要能追溯到一个 Issue，提交信息或 PR 描述中引用对应编号。
- 只提交与当前 Issue 相关的变更，避免把无关格式化、实验代码、本地数据混在一起。
- README 和贡献文档只能描述当前已经实现的能力，不把路线图写成现状。
- 任何证明语义、存储格式、CLI/API 行为变化，都要同步更新测试和用户可见文档。
- 不提交本地运行数据、密钥、数据库目录、备份包、构建产物或私有部署配置。

## 开发前准备

推荐环境：

- Go：使用 `go.mod` 中声明的版本。
- Git：用于分支、提交和远程同步。
- Node.js / npm：用于 `clients/desktop/frontend`。
- Wails：仅在需要构建桌面客户端安装包时需要。
- Windows race/Wails 构建如遇 CGO 编译错误，需要先配置可用的 C/C++ 编译环境。

克隆仓库：

```powershell
git clone git@github.com:ryan-wong-coder/trustdb.git
cd trustdb
```

拉取依赖并校验：

```powershell
go mod download
go mod verify

cd clients/desktop
go mod download
go mod verify

cd frontend
npm ci
```

## Issue-first 流程

每个开发任务先创建 GitHub Issue。Issue 至少包含：

- 背景 / 问题
- 目标
- 非目标
- 影响范围
- 实现方案或候选方案
- 验收标准
- 测试计划
- 风险与回滚

如果任务涉及架构、证明等级、存储格式、备份格式、外部锚定或公开 API，Issue 中必须先写清楚兼容性边界和安全影响。

## 分支规范

从最新 `main` 创建分支：

```powershell
git switch main
git pull --ff-only
git switch -c issue-<number>-<short-name>
```

示例：

```powershell
git switch -c issue-2-contributing-guide
```

分支命名建议：

- `issue-<number>-<short-name>`：常规功能、修复、文档任务。
- `hotfix-<number>-<short-name>`：紧急修复。
- `spike-<number>-<short-name>`：探索性验证。探索结果合并前要收敛成正式实现或文档。

不要直接在 `main` 上开发。

## 提交规范

提交信息使用简洁祈使句，并引用 Issue：

```text
Add contribution guide

Refs #2
```

修复类提交可以使用：

```text
Fix WAL replay memory spike

Fixes #12
```

提交前检查：

```powershell
git status --short
git diff
```

确认没有以下内容被提交：

- `.trustdb/`、`.trustdb-dev/`、`.localtest/`、`.localdeploy/`
- `node_modules/`、`dist/`、`build/`、`release/`
- `*.key`、`*.pub`、`*.pem`、`*.tdkeys`
- `.env`、备份包、临时文件、日志文件
- 与当前 Issue 无关的格式化或实验文件

## 工程约束

TrustDB 不是普通 CRUD 服务，贡献时要优先保护证明语义、持久化边界、恢复能力和大规模数据路径。下面这些约束比个人代码风格更重要。

### 证明语义

- L1/L2/L3/L4/L5 的含义不能在 UI、CLI、HTTP API 和验证器之间出现分叉。
- L4 只表示 batch root 已进入 Global Transparency Log，并能证明它包含在某个 STH 中。
- L5 只表示对应 STH/global root 已被外部 notary 锚定；不要恢复 batch root 直锚模式。
- `.sproof` 是主要交换格式；`.tdproof`、`.tdgproof`、`.tdanchor-result` 是高级分步格式，不能让两套路径得出不同结论。
- 任何证明对象的字段、编码、hash input、signature input 变化，都必须有向后兼容说明或明确声明不兼容，并补充验证测试。

### 确定性与可验证性

- Proof、receipt、STH、anchor result、backup manifest 等可信对象必须使用确定性编码和稳定字段顺序。
- hash 输入必须在代码中有单一入口，禁止不同模块临时拼接各自版本的 hash payload。
- 验证器必须独立复算，而不是信任服务端返回的等级、状态或摘要文本。
- 错误信息可以友好，但不能吞掉验证失败的具体原因；安全相关失败不得降级成成功或 unknown。
- 测试要覆盖“篡改后失败”，不只覆盖 happy path。

### 持久化与恢复

- WAL accepted、batch committed、global log appended、anchor published 是不同耐久边界，代码不能把它们混成一个状态。
- 启动恢复必须是幂等的；重复 replay、重复 outbox 处理、重复 restore 不能产生重复 leaf 或改变已有 proof。
- 恢复路径禁止为了方便一次性加载全部 WAL、roots、manifests、global leaves、records 或 backup entries。
- group commit、checkpoint、segment cleanup 的改动必须说明崩溃窗口和恢复语义。
- 任何“修复损坏数据”的逻辑都必须保守，能隔离坏记录时不要扩大损坏范围。

### 可扩展数据路径

- 面向生产的路径默认按 Pebble proofstore 设计；file proofstore 只保证开发和小规模演示语义。
- 列表、扫描、pending 查询、anchor upgrade、backup export/restore 必须优先使用 cursor/range/status index。
- 不允许在 HTTP/CLI/desktop 主路径中重新引入全量扫描、全量排序或全量内存聚合。
- Global Log proof 生成应读取必要 node/tile/range；不能为了生成单个 proof 重建整棵树。
- 大规模路径新增 API 时，必须同时考虑 limit、cursor、direction、错误中断和可恢复性。

### 并发与后台任务

- batch commit、global append、STH anchor、OTS upgrade、backup/restore 应保持解耦；慢外部服务不能拖死 ingest 或 batch worker。
- outbox worker 必须记录 attempts、last_error、next_attempt_at，并避免无界重试扫全表。
- goroutine 必须有清晰的 context/lifecycle；服务关闭时要能停止、flush 或明确放弃非关键工作。
- 共享状态优先通过持久化状态机或单一 owner goroutine 管理，避免跨 worker 隐式共享可变状态。

### 公共接口

- HTTP/CLI/桌面客户端的同名概念必须返回同一类事实，不允许同一证明等级在不同入口中语义不同。
- 新增公开字段时优先使用明确单位和时间基准，例如 `*_unix_nano`；不要暴露 Go 专用类型到 Wails DTO。
- 错误码、HTTP status、CLI exit behavior 要能被自动化脚本判断。
- API 文档和 README 示例必须来自当前实现；不能提前写尚未落地的接口。

### 桌面客户端

- records 本地存储必须保持索引化；不要退回单 JSON 全量读写。
- 列表、搜索、详情、批量刷新必须按需加载，不能让十万级本机记录卡死启动或渲染。
- 客户端显示的证明等级应来自本地 proof artifact 或服务端可验证状态，不能只靠前端推断。
- 导出功能优先维护 `.sproof` 主路径；分步文件保留为高级入口，二者编码结果必须可相互验证。
- UI 改动需要检查小窗口、长文件名、长 hash、失败状态和加载中状态。

### 文档与示例

- 文档只能写当前实现；路线图、设计草稿和未落地功能不要进入 README 或贡献指南。
- 命令示例必须可执行，或明确写出前置条件。
- Windows 示例优先使用 PowerShell；跨平台差异需要单独说明。
- 若实现与设计文档冲突，以实现为准先修 README/贡献文档，再开 Issue 处理设计文档。

## 测试要求

按变更范围选择测试。常规后端变更至少运行：

```powershell
go test ./...
```

涉及并发、WAL、proofstore、global log、anchor、backup、HTTP 服务时，补充：

```powershell
go test -race ./...
go test -tags=integration ./...
go test -tags=e2e ./...
```

涉及桌面客户端 Go 层：

```powershell
cd clients/desktop
go test ./...
go test -race ./...
```

涉及桌面前端：

```powershell
cd clients/desktop/frontend
npm run build
```

涉及完整桌面包：

```powershell
cd clients/desktop
wails build
```

如果某个测试因本机工具链、网络或外部服务不可用而无法运行，必须在 Issue 或 PR 中写明原因、已运行的替代检查和残余风险。

## 文档要求

需要同步更新文档的情况：

- 新增或修改 CLI 命令、参数、配置项、环境变量。
- 新增或修改 HTTP API。
- 改变 L1-L5 证明语义。
- 改变 `.tdproof`、`.tdgproof`、`.tdanchor-result`、`.sproof`、`.tdbackup` 格式。
- 改变默认存储、WAL、anchor、backup 或桌面客户端行为。
- 新增运维前置条件或安全注意事项。

仓库当前提交的主入口文档是：

- `README.md`：用户入口、当前功能、架构、快速向导。
- `CONTRIBUTING.md`：贡献流程。
- `clients/desktop/README.md`：桌面客户端说明。
- `configs/*.yaml`：配置示例。

`doc/` 和 `docs/` 当前作为本地设计/运维草稿目录被忽略，不作为默认提交内容。若某个 Issue 明确要求发布正式文档，需要先在 Issue 中说明目标路径和发布范围。

## PR 要求

提交 PR 前，请确认：

- PR 标题说明变更目的。
- PR 描述关联 Issue，例如 `Refs #2` 或 `Fixes #2`。
- PR 描述包含变更摘要、测试结果、风险和回滚方式。
- 只包含当前 Issue 范围内的文件。
- 所有用户可见行为变化都更新了 README、配置示例或相关说明。
- 新增功能有测试，修复 Bug 有回归测试或明确说明无法自动化的原因。

建议 PR 描述模板：

```markdown
## Summary

- 

## Linked Issue

Refs #

## Tests

- [ ] go test ./...
- [ ] go test -race ./...
- [ ] go test -tags=integration ./...
- [ ] go test -tags=e2e ./...
- [ ] cd clients/desktop && go test ./...
- [ ] cd clients/desktop/frontend && npm run build

## Risk / Rollback

- 
```

## 安全与密钥

- 不要提交真实私钥、注册表密钥、服务器密钥、`.env` 或生产配置。
- 示例密钥也不要放进仓库，除非 Issue 明确要求测试 fixture，且文件名、路径、用途都表明它不可用于生产。
- 对外部 anchor、OpenTimestamps、远程服务调用的改动，需要说明失败、超时、重试和降级行为。
- 任何会影响证明可信边界的改动，都要在 Issue/PR 中单独标出。

## 许可证

TrustDB 使用 AGPL-3.0-only。提交代码即表示你同意你的贡献按仓库许可证发布。

如果引入第三方依赖或资产，需要确认其许可证与 AGPL-3.0-only 兼容，并在 PR 中说明来源和用途。

## 发布与部署边界

普通 PR 不直接部署生产服务，也不修改本机或远程运行数据。部署类任务必须有单独 Issue，并在 Issue 中写明：

- 部署目标
- 备份方式
- 回滚方式
- 验证命令
- 影响窗口

## 维护者合并前检查

- Issue 是否清楚且与 PR 对应。
- 变更是否保持证明语义一致。
- 存储和恢复路径是否避免重新引入全量扫描/全量加载。
- 文档是否只写当前实现。
- 测试是否覆盖主要风险。
- 是否有敏感文件或本地数据被误提交。
