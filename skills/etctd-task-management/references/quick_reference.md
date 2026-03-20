# etctd Quick Reference

## Session

```bash
etctd usage --new-session
etctd usage -q
```

## Create and List

```bash
etctd create --type task --priority P2 "Implement feature"
etctd list
etctd list --ready
etctd list --reviewable
etctd list --status in_progress --mine
etctd list --type bug --priority "<=P1" --search scheduler
```

## Work and Handoff

```bash
etctd start <id>
etctd log <id> "implemented parser"
etctd handoff <id> --done "..." --remaining "..." --decisions "..." --uncertain "..."
```

## Dependencies

```bash
etctd dep add <id> <depends-on-id>
etctd dep list <id>
etctd dep remove <id> <depends-on-id>
```

## Review

```bash
etctd review <id>
etctd approve <id>
etctd reject <id> --reason "..."
```
