# smith-replica AGENT.md snippet

Use this snippet in `smith-replica`'s `AGENT.md` to standardize task and handoff behavior with the etcd-backed `etctd` CLI.

```md
## Task and Handoff Protocol (etcd-backed etctd)

At conversation start (or after `/clear`), run:

`etctd usage --new-session`

For subsequent checks in the same conversation, run:

`etctd usage -q`

Use `etctd` as the source of truth for task lifecycle:

- `etctd list --ready` to pick unblocked work
- `etctd start <id>` before implementation
- `etctd log <id> "message"` for progress checkpoints
- `etctd dep add <id> <depends-on-id>` for blocker edges
- `etctd handoff <id> --done ... --remaining ... --decisions ... --uncertain ...` before stop/switch
- `etctd review <id>` when ready for review
- `etctd list --reviewable` to find review queue
- `etctd approve <id>` to complete

Never stop mid-implementation without `etctd handoff`.
```
