<!-- etctd-usage:start -->
## MANDATORY: Use etctd for Task Management

At conversation start (or after `/clear`), run:

`etctd usage --new-session`

For subsequent reads in the same conversation, run:

`etctd usage -q`

Use `etctd` as the source of truth for task lifecycle and handoffs:

- `etctd list --ready` to pick unblocked work
- `etctd start <id>` before implementation
- `etctd log <id> "message"` for progress checkpoints
- `etctd dep add <id> <depends-on-id>` for blocker edges
- `etctd handoff <id> --done ... --remaining ... --decisions ... --uncertain ...` before stop/switch
- `etctd review <id>` when ready for review
- `etctd list --reviewable` to find review queue
- `etctd approve <id>` to complete

Never stop mid-implementation without `etctd handoff`.
<!-- etctd-usage:end -->
