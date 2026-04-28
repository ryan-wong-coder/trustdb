---
name: Engineering task
about: Track refactoring, quality, CI, docs, deployment, or maintenance work.
title: "[Task] "
labels: ""
assignees: ""
---

## Background

Explain why this task is needed now.

## Scope

- In scope:
- Out of scope:

## Implementation plan

1.
2.
3.

## Risk areas

- Proof semantics:
- Storage / recovery:
- Large-data path:
- Desktop behavior:
- Deployment / operations:

## Acceptance criteria

- [ ]
- [ ]

## Validation

- [ ] `go test ./...`
- [ ] `go test -race ./...`
- [ ] `go test -tags=integration ./...`
- [ ] `go test -tags=e2e ./...`
- [ ] `cd clients/desktop && go test ./...`
- [ ] `cd clients/desktop/frontend && npm run build`

If a check is not relevant or cannot run, explain why.
