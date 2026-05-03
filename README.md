# TrustDB

CI

[中文说明](README.zh-CN.md) | [Blog](https://www.blog.ryan-wong.cn/)

TrustDB system architecture

TrustDB is a verifiable evidence database that can run as a simple single-node service or as a horizontally scalable deployment with storage-compute separation. It turns local file claims into signed receipts, Merkle batch proofs, global transparency-log proofs, and optional external STH anchors.

Current repository module:

```text
github.com/ryan-wong-coder/trustdb
```

License: AGPL-3.0-only. See [LICENSE](LICENSE).

## Performance Report

The high-write private-IP CVM retest report is available in English and Chinese:

- [TrustDB High-Write Private-IP CVM Retest Report](docs/performance/trustdb-performance-report-2026-04-30.en.md)
- [TrustDB 性能优先双机内网复测报告](docs/performance/trustdb-performance-report-2026-04-30.zh-CN.md)

## Desktop Client

TrustDB desktop client overview

The Wails + Vue desktop client provides local identity setup, HTTP/gRPC server configuration, file attestation, record and proof management, `.sproof` export, and local proof verification.

## Current Features

- Deterministic CBOR proof objects for claims, receipts, proof bundles, global-log proofs, STHs, anchors, and portable single-file proofs.
- Ed25519 client/server/registry signatures with key generation, key inspection, registry registration, revocation, and key listing commands.
- File claim creation with SHA-256 content hashing and optional local object-store copy.
- WAL-backed ingest path with configurable fsync mode: `strict`, `group`, or `batch`.
- HTTP ingest server with bounded queues, worker pool, health endpoint, Prometheus metrics, and graceful shutdown.
- Optional gRPC server for SDK/service-to-service access, using TrustDB CBOR payloads over unary gRPC calls.
- Batch Merkle worker that emits `ProofBundle` data and server-side record indexes.
- Global Transparency Log that appends committed batch roots, persists STHs, and serves inclusion/consistency proofs.
- L5 anchor pipeline that anchors only `SignedTreeHead` / global roots, not per-batch roots.
- Anchor sinks for `off`, `noop`, local file output, and OpenTimestamps, with OTS upgrade worker support.
- Proofstore backends for local file storage, Pebble, and optional TiKV (shared PD/TiKV cluster) so multiple compute nodes can share durable proof data for storage-compute separation.
- Paginated server-side record and root APIs with cursor/range-oriented access paths.
- Portable `.tdbackup` create, verify, and resumable restore flow.
- Public Go SDK for claim signing, server calls, proof export, and local verification.
- Desktop client built with Wails + Vue that supports onboarding, local identity, server settings, file attestation, record management, proof refresh, local verification, and `.sproof` export.
- Desktop local record storage backed by Pebble with indexes for list/search/filter paths.

## Proof Levels

TrustDB proof levels

TrustDB uses layered proof semantics:


| Level | Meaning                                                                     | Main artifact                          |
| ----- | --------------------------------------------------------------------------- | -------------------------------------- |
| L1    | Client signs a file claim containing content hash and metadata.             | `SignedClaim` / `.tdclaim`             |
| L2    | Server validates the claim and accepts it into WAL.                         | `AcceptedReceipt`                      |
| L3    | Claim is committed into a batch Merkle tree.                                | `ProofBundle` / `.tdproof`             |
| L4    | The batch root is included in the Global Transparency Log and a target STH. | `GlobalLogProof` / `.tdgproof`         |
| L5    | The corresponding STH/global root is externally anchored.                   | `STHAnchorResult` / `.tdanchor-result` |


For exchange and desktop verification, `.sproof` is the main single-file proof format. It can contain the L3 `ProofBundle`, optional L4 `GlobalLogProof`, and optional L5 `STHAnchorResult` together. The stable v1 format is documented in [formats/SPROOF_V1.md](formats/SPROOF_V1.md).

## Architecture

TrustDB's default deployment is single-node, while the TiKV proofstore backend supports a distributed storage-compute separated topology. In that mode, multiple TrustDB compute nodes can ingest, batch, serve proofs, and expose HTTP/gRPC APIs against the same TiKV-backed proofstore, with `node_id` / `log_id` metadata preserving source identity.

- Client path: CLI or desktop computes the file hash, signs a claim, and submits it to the HTTP server or local CLI flow.
- Ingest path: server validates signatures and key status, records durable acceptance in WAL, and returns an accepted receipt.
- Batch path: committed records are grouped into Merkle batches and stored as proof bundles plus record indexes.
- Global log path: committed batch roots are appended into a global transparency log, producing persisted STHs and global proofs.
- Anchor path: STH/global roots are queued and published by the anchor worker when an anchor sink is configured.
- Storage path: proof data is stored in file, Pebble, or TiKV proofstore backends; TiKV is the shared storage layer for horizontally scaled compute nodes.
- Backup path: proofstore data can be exported to `.tdbackup`, verified, and restored with resume checkpoints.
- Observability path: `/metrics` exposes ingest, batch, global log, anchor, WAL, backup, and storage metrics.

## HTTP API

Implemented HTTP endpoints include:


| Endpoint                                   | Purpose                                       |
| ------------------------------------------ | --------------------------------------------- |
| `GET /healthz`                             | Health check.                                 |
| `POST /v1/claims`                          | Submit a signed claim.                        |
| `GET /v1/records`                          | Paginated record list and search.             |
| `GET /v1/records/{record_id}`              | Read record index details.                    |
| `GET /v1/proofs/{record_id}`               | Fetch L3 proof bundle.                        |
| `GET /v1/roots`                            | List batch roots.                             |
| `GET /v1/roots/latest`                     | Fetch latest batch root.                      |
| `GET /v1/sth/latest`                       | Fetch latest SignedTreeHead.                  |
| `GET /v1/sth/{tree_size}`                  | Fetch a specific STH.                         |
| `GET /v1/global-log/inclusion/{batch_id}`  | Fetch global-log inclusion proof for a batch. |
| `GET /v1/global-log/consistency?from=&to=` | Fetch global-log consistency proof.           |
| `GET /v1/anchors/sth/{tree_size}`          | Fetch STH anchor status/result.               |
| `GET /metrics`                             | Prometheus metrics.                           |


## gRPC API

The server can also expose a gRPC listener with `--grpc-listen` or `server.grpc_listen`. The first implementation keeps TrustDB's existing deterministic CBOR model as the gRPC payload codec, so SDK callers can use HTTP or gRPC without changing proof object semantics.

The gRPC service covers the same core SDK paths currently used by the desktop client: health, submit claim, record list/detail, proof bundle, roots/latest root, STH, global proof, anchor state, and metrics. It also registers the standard `grpc.health.v1.Health` service for infrastructure probes.

## CLI Quick Guide

Generate keys:

```powershell
go run ./cmd/trustdb keygen --out .trustdb-dev --prefix client
go run ./cmd/trustdb keygen --out .trustdb-dev --prefix server
```

Start a local development server with direct client public-key trust:

```powershell
go run ./cmd/trustdb serve `
  --config configs/development.yaml `
  --server-private-key .trustdb-dev/server.key `
  --client-public-key .trustdb-dev/client.pub `
  --listen 127.0.0.1:8080
```

Create and sign a file claim:

```powershell
go run ./cmd/trustdb claim-file `
  --file .\example.txt `
  --private-key .trustdb-dev/client.key `
  --tenant default `
  --client local-client `
  --key-id client-key `
  --out .trustdb-dev/example.tdclaim
```

Commit a signed claim locally into a proof bundle:

```powershell
go run ./cmd/trustdb commit `
  --claim .trustdb-dev/example.tdclaim `
  --server-private-key .trustdb-dev/server.key `
  --client-public-key .trustdb-dev/client.pub `
  --out .trustdb-dev/example.tdproof
```

Verify a local file and proof bundle:

```powershell
go run ./cmd/trustdb verify `
  --file .\example.txt `
  --proof .trustdb-dev/example.tdproof `
  --server-public-key .trustdb-dev/server.pub `
  --client-public-key .trustdb-dev/client.pub
```

Verify a local file with the recommended single-file proof:

```powershell
go run ./cmd/trustdb verify `
  --file .\example.txt `
  --sproof .trustdb-dev/example.sproof `
  --server-public-key .trustdb-dev/server.pub `
  --client-public-key .trustdb-dev/client.pub
```

Inspect Global Log data:

```powershell
go run ./cmd/trustdb global-log sth latest --metastore file --metastore-path .trustdb-dev/proofs
go run ./cmd/trustdb global-log proof inclusion --batch-id batch-000001 --metastore file --metastore-path .trustdb-dev/proofs
```

Create and verify a portable backup:

```powershell
go run ./cmd/trustdb backup create `
  --metastore file `
  --metastore-path .trustdb-dev/proofs `
  --out .trustdb-dev/trustdb.tdbackup

go run ./cmd/trustdb backup verify --file .trustdb-dev/trustdb.tdbackup
```

## Configuration

Three labelled templates are included (see `configs/README.md` for `run_profile` semantics and startup hints):


| File                       | `run_profile`              | Intended use                                                                                                                                                                                            |
| -------------------------- | -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `configs/development.yaml` | `development`              | Local development and demos. Uses file proofstore and `noop` anchor sink.                                                                                                                               |
| `configs/production.yaml`  | `single_node_production`   | Baseline production profile. Uses Pebble proofstore, directory WAL, group fsync, global log, and OTS anchor sink; switch the proofstore to TiKV for shared storage and horizontal compute-node scaling. |
| `configs/benchmark.yaml`   | `benchmark`                | Load and ingest benchmarks: Pebble proofstore, `wal.fsync_mode: batch`, larger queues, async batch proofs, `noop` anchor — **not** for production audit semantics.                                       |


Important configuration groups:


| Group                          | Purpose                                                                                                                                                                              |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `run_profile`                  | Optional operator label (`development`, `single_node_production`, `benchmark`); `trustdb serve` logs profile guidance only — it does not alter behavior.                           |
| `paths`                        | Data, key registry, WAL, object, and proof directories.                                                                                                                              |
| `metastore` / `metastore_path` | Proofstore backend selection: `file`, `pebble`, or `tikv`; for TiKV, `metastore_path` may provide comma-separated PD endpoints for the shared storage cluster.                       |
| `proofstore.tikv_namespace`    | Application-level TiKV key namespace. Multiple compute nodes that should share one proofstore must use the same namespace; independent tenants/logs should use different namespaces. |
| `wal`                          | Fsync strategy and group-commit interval.                                                                                                                                            |
| `server`                       | HTTP listen address, optional gRPC listen address, queue size, workers, and timeouts.                                                                                                |
| `batch`                        | Batch queue, max records, and max delay.                                                                                                                                             |
| `global_log`                   | Enables the global transparency log and configures the node-local `log_id`.                                                                                                          |
| `anchor`                       | L5 anchor scope, sink, max delay, OTS calendars, and OTS upgrader.                                                                                                                   |
| `history`                      | Global-log tile size and hot window.                                                                                                                                                 |
| `backup`                       | Backup compression.                                                                                                                                                                  |
| `log`                          | Structured logging, rotation, and async logging.                                                                                                                                     |
| `keys`                         | Client, server, and registry key file paths.                                                                                                                                         |


Most config values can also be overridden with `TRUSTDB_*` environment variables or command flags. Run `go run ./cmd/trustdb serve --help` for the currently implemented server flags.

TiKV keys are stored under a TrustDB-managed namespace prefix. Older pre-namespace TiKV data written with bare Pebble-compatible keys can be copied into a namespace with:

```powershell
go run ./cmd/trustdb metastore tikv-migrate-legacy --pd-endpoints=127.0.0.1:2379 --namespace=default
```

## Desktop Client

The desktop app lives in `clients/desktop` and uses Wails + Vue. It currently supports:

- onboarding with local identity generation/import and server configuration;
- file attestation against a TrustDB HTTP server;
- paginated local record list, search, filters, details, deletion, and proof status refresh;
- `.sproof` as the primary export format;
- advanced export of `.tdproof`, `.tdgproof`, and `.tdanchor-result`;
- local proof verification, with optional L4/L5 artifacts;
- local record persistence in a Pebble-backed index store instead of one large JSON file.

Build checks used by this repository:

```powershell
cd clients/desktop
go test ./...
cd frontend
npm run build
```

## Go SDK

The public Go SDK lives in `sdk` and is imported as:

```go
import "github.com/ryan-wong-coder/trustdb/sdk"
```

Current SDK capabilities:

- build and sign file claims with Ed25519;
- submit signed claims to `POST /v1/claims`;
- query records, proof bundles, STHs, GlobalLogProof, and STH anchor state;
- use either the default HTTP transport or the gRPC transport via `sdk.NewGRPCClient`;
- use client-side multi-endpoint failover or round-robin via `sdk.NewLoadBalancedClient`;
- export the recommended `.sproof` single-file proof;
- verify `.sproof` and split proof artifacts locally.

Minimal example:

```go
client, err := sdk.NewClient("http://127.0.0.1:8080")
if err != nil {
    return err
}
proof, err := client.ExportSingleProof(ctx, recordID)
if err != nil {
    return err
}
return sdk.WriteSingleProofFile("example.sproof", proof)
```

Use gRPC transport instead of HTTP:

```go
client, err := sdk.NewGRPCClient("127.0.0.1:9090")
```

Use SDK-side failover across independent TrustDB nodes:

```go
client, err := sdk.NewLoadBalancedClient(
    []string{"http://10.0.0.10:8080", "http://10.0.0.11:8080"},
    sdk.LoadBalanceOptions{Mode: sdk.LoadBalanceFailover},
)
```

## Development Checks

Pull requests run the GitHub Actions workflow in `.github/workflows/ci.yml`. It covers repository hygiene, backend unit/race/integration/e2e tests, desktop Go tests, and the desktop frontend build.

Common checks:

```powershell
go test ./...
go test -tags=integration ./...
go test -tags=e2e ./...
go test -race ./...
cd clients/desktop; go test ./...
cd clients/desktop/frontend; npm run build
```

### Local TiKV via Docker (integration tests)

The repo ships a minimal **single-node** PD+TiKV stack for host-run Go tests (not a production layout):

- Compose: [`docker-compose.tikv.yml`](docker-compose.tikv.yml) (project name `trustdb-tikv-dev`, PD `127.0.0.1:2379`, TiKV `127.0.0.1:20160`). Override PD/TiKV images with `TRUSTDB_TIKV_PD_IMAGE` / `TRUSTDB_TIKV_TIKV_IMAGE` (environment or repo-root `.env`; see [`scripts/tikv-dev.env.example`](scripts/tikv-dev.env.example)).
- **Linux / macOS**: `./scripts/tikv-dev.sh up` then `./scripts/tikv-dev.sh test` (optional `./scripts/tikv-dev.sh test-legacy` for legacy migration tests). `./scripts/tikv-dev.sh reset` removes volumes for a clean cluster. Backend: `TRUSTDB_TIKV_COMPOSE_VIA` = `auto` (default), `native`, `podman`, or `docker-api`. **auto** uses `podman compose` when it is available on Unix-like systems (Git Bash / MSYS on Windows uses **native** to match the PowerShell script); otherwise it uses a **Podman-only native path** (`podman pod` + `podman run`, same images/ports as the compose file—no Docker CLI). **native** always uses that path. **podman** forces `podman compose` (honours `TRUSTDB_TIKV_COMPOSE_FILE`). **docker-api** is optional: `docker compose` with `DOCKER_HOST` (e.g. Podman’s npipe on Windows); install the Docker CLI only if you use this mode.
- **Windows (PowerShell)**: `.\scripts\tikv-dev.ps1 up` then `.\scripts\tikv-dev.ps1 test` (optional `test-legacy`). `.\scripts\tikv-dev.ps1 reset` removes volumes. Default **auto** on this script is the same as **native**: Podman only (`podman pod` + named volumes + the same PD/TiKV images), so **podman compose** and Docker CLI are not required. Use `TRUSTDB_TIKV_COMPOSE_VIA=podman` if you want compose (and a custom `TRUSTDB_TIKV_COMPOSE_FILE`); use `docker-api` only with Docker CLI + `TRUSTDB_TIKV_DOCKER_HOST` if you intentionally target a Docker-compatible API.
- **CI**: the [TiKV integration workflow](.github/workflows/tikv-integration.yml) starts `docker-compose.tikv.yml` on GitHub-hosted runners and runs `go test -tags=integration ./internal/proofstore/tikv` when TiKV-related paths change, or when you run it manually (**Actions → TiKV integration → Run workflow**).

Manual equivalent:

```powershell
docker compose -f docker-compose.tikv.yml --project-name trustdb-tikv-dev up -d
$env:TRUSTDB_TIKV_PD_ENDPOINTS = "127.0.0.1:2379"
go test -count=1 -tags=integration ./internal/proofstore/tikv
```

Real TiKV conformance and shared-namespace tests are enabled by setting `TRUSTDB_TIKV_PD_ENDPOINTS` before running integration tests. The legacy-key migration tests additionally require `TRUSTDB_TIKV_RUN_LEGACY_MIGRATION_TEST=1` because they intentionally read bare legacy TiKV key ranges.

The repository includes tests for deterministic CBOR, claims, WAL, batch proofs, global log proofs, anchors, backup/restore, HTTP API behavior, and desktop-local storage/verification paths.

## Contributing

TrustDB uses an Issue-first development flow. Before starting code, documentation, refactoring, or operations work, create or link a GitHub Issue with scope, acceptance criteria, and a test plan.

Use the GitHub Issue templates for bug reports, feature requests, and engineering tasks. Pull requests should use the repository PR template and must reference the linked Issue.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full contribution workflow, branch conventions, commit/PR expectations, CI quality gates, test matrix, documentation rules, and security notes.
