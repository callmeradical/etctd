# etctd AI Agent Workflows

## 1. Start of Session

```bash
etctd usage --new-session
etctd list --ready
```

Pick one ready issue and begin:

```bash
etctd start <id>
```

## 2. Progress Logging

```bash
etctd log <id> "implemented API adapter"
etctd log <id> "added retry policy"
```

Log checkpoints after meaningful steps, not every minor edit.

## 3. Dependency-Driven Work

When an issue depends on another:

```bash
etctd dep add <id> <depends-on-id>
```

Use `etctd list --ready` to find work that is unblocked.

## 4. Handoff Before Context Ends

```bash
etctd handoff <id> \
  --done "implemented parser and tests" \
  --remaining "wire parser into controller" \
  --decisions "kept parser pure for easier testing" \
  --uncertain "need confirmation on edge-case format"
```

Always hand off before stopping mid-implementation.

## 5. Review Lifecycle

```bash
etctd review <id>
etctd list --reviewable
etctd approve <id>
```

If review fails:

```bash
etctd reject <id> --reason "missing rollback handling"
```
