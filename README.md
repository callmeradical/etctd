# etctd

`etctd` is a Smith-aligned task and handoff CLI backed by etcd.

This implementation intentionally targets core workflow parity first:

- session identity scoped by branch + agent
- issue lifecycle (`open -> in_progress -> in_review -> closed`)
- progress logs and structured handoffs
- review/approval guardrails (self-approval blocked unless `--minor`)

## Why etcd

Smith already uses etcd for distributed coordination and state transitions. Reusing etcd for core task workflow keeps consistency guarantees aligned with the control plane:

- optimistic concurrency with compare-and-swap (revision checks)
- deterministic keyspace under `/smith/v1/etctd/projects/<project-id>/...`
- no extra persistence system in the first phase

## Environment

- `SMITH_ETCD_ENDPOINTS` (default: `http://127.0.0.1:2379`)
- `SMITH_ETCD_DIAL_TIMEOUT` (default: `5s`)
- `SMITH_AGENT_TYPE` (optional override for session identity)

## Build

```bash
go build ./cmd/etctd
```

## Commands

```text
etctd init
etctd usage [-q] [--new-session]
etctd create [--type task] [--priority P2] [--description text] [--minor] "title"
etctd list [--status open] [--mine] [--all] [--limit N]
etctd show <id>
etctd start <id>
etctd log <id> <message>
etctd handoff <id> --done a,b --remaining c,d
etctd review <id>
etctd approve <id>
etctd reject <id> [--reason text]
etctd current
etctd dep add <id> <depends-on-id>
etctd dep list <id>
etctd dep remove <id> <depends-on-id>
```

## Agent skill

For smith-replica usage, an agent skill is included at:

- `skills/etctd-install/SKILL.md`
- `skills/etctd-task-management/SKILL.md`

`etctd-install` sets up AGENTS.md instructions.
`etctd-task-management` codifies the day-to-day agent task workflow.

```bash
bash skills/etctd-install/scripts/write_agents_md.sh
```

A ready-to-paste `AGENT.md` snippet for smith-replica is available at:

- `docs/smith-replica-agent-md-snippet.md`

## Current scope notes

This is intentionally focused on Smith replica workflows (task lifecycle, dependency edges, handoffs), and is designed for incremental extension.

## CI

GitHub Actions workflow at `.github/workflows/ci.yml` runs:

- unit/integration tests via `go test ./...`
- k3s-based integration job that deploys etcd from `test/k8s/etcd-single.yaml`
- end-to-end CLI action scenarios in `test/scenarios/cli_actions_test.go`

### Local workflow testing with act

Use `act` to run CI jobs locally:

```bash
act pull_request -j unit-tests
act pull_request -j act-local-integration
```

Notes:

- `.actrc` is included with a compatible runner image mapping.
- `k3s-integration` is skipped under `act`; `act-local-integration` runs scenario tests against a local etcd service container instead.
