# CLI Action Scenarios

`test/scenarios/cli_actions_test.go` runs end-to-end scenarios for every supported `etctd` action against a real etcd endpoint.

Covered actions:

- `usage` (`--new-session`, `-q`)
- `init`
- `create`
- `list` (search, ready, reviewable)
- `show`
- `start`
- `current`
- `log`
- `handoff`
- `dep add`
- `dep list`
- `dep remove`
- `review`
- `approve`
- `reject`

## Run locally

Start etcd and set endpoint:

```bash
export SMITH_ETCD_ENDPOINTS=http://127.0.0.1:2379
export ETCTD_SCENARIOS=1
go test ./test/scenarios -v
```

Without `ETCTD_SCENARIOS=1`, these tests are skipped.
