## Summary

-

## Linked Issue

Refs #

## Scope Control

- [ ] This PR only changes files needed for the linked Issue.
- [ ] README / user-facing docs only describe behavior that is already implemented.
- [ ] No local data, keys, logs, backups, build artifacts, `doc/`, or `docs/` files are included.

## Proof Semantics

- [ ] No L1/L2/L3/L4/L5 semantic change.
- [ ] L4/L5 behavior is unchanged: L4 is batch root in Global Log; L5 is STH/global root externally anchored.
- [ ] `.tdproof`, `.tdgproof`, `.tdanchor-result`, `.sproof`, and `.tdbackup` formats are unchanged.
- [ ] If any box above is false, the Issue and this PR explain compatibility and verification impact.

## Storage, Recovery, and Scale

- [ ] No production path introduces full-scan, full-load, or full-recompute behavior.
- [ ] WAL, proofstore, global log, anchor outbox, backup, and desktop local storage boundaries are unchanged.
- [ ] If storage/recovery behavior changes, tests or manual validation cover replay/retry/idempotency.

## Validation

- [ ] `go test ./...`
- [ ] `go test -race ./...`
- [ ] `go test -tags=integration ./...`
- [ ] `go test -tags=e2e ./...`
- [ ] `cd clients/desktop && go test ./...`
- [ ] `cd clients/desktop && go test -race ./...`
- [ ] `cd clients/desktop/frontend && npm run build`
- [ ] `cd clients/desktop && wails build`

Checks not run:

-

## Risk / Rollback

-
