---
name: etctd-task-management
description: Task management for AI agents across context windows using etctd. Use when agents need to track work, log progress, model dependencies, and hand off state between sessions.
---

# etctd - Task Management for AI Agents

## Overview

`etctd` is an etcd-backed CLI for tracking tasks and preserving agent context across sessions.

**Core capability:** run `etctd usage` to get current session state and actionable next steps.

## Quick Start

### Session Start (Every Time)

```bash
etctd usage --new-session
```

Output tells you your current session identity and workflow commands.

### Single-Issue Workflow

```bash
etctd start <issue-id>
etctd log <issue-id> "OAuth callback implemented"
etctd handoff <issue-id> --done "..." --remaining "..."
etctd review <issue-id>
```

### Dependency-Aware Workflow

```bash
etctd list --ready
etctd dep add <issue-id> <depends-on-id>
etctd dep list <issue-id>
```

`etctd start` is blocked while dependencies remain open.

## Key Workflows

### Workflow 1: Starting New Work

```bash
etctd usage
etctd list --ready
etctd start <id>
etctd log <id> "Started implementation"
```

### Workflow 2: Handing Off Work

```bash
etctd handoff <id> \
  --done "OAuth flow, token storage" \
  --remaining "Refresh token rotation, error handling" \
  --decisions "Using JWT for stateless auth" \
  --uncertain "Should tokens expire on password change?"
```

Keys:
- `--done` - what is complete
- `--remaining` - what is left
- `--decisions` - why decisions were made
- `--uncertain` - unresolved questions

### Workflow 3: Reviewing Work

```bash
etctd list --reviewable
etctd show <id>
etctd approve <id>
# or
etctd reject <id> --reason "Missing error handling"
```

You cannot approve work you implemented unless the issue is marked minor (`--minor`).

## Commands by Category

### Checking Status
- `etctd usage`
- `etctd usage -q`
- `etctd current`
- `etctd list --ready`
- `etctd list --reviewable`

### Working on Issues
- `etctd start <id>`
- `etctd log <id> "msg"`
- `etctd show <id>`
- `etctd list [filters]`

### Handoffs
- `etctd handoff <id> --done "..." --remaining "..."`

### Reviews
- `etctd review <id>`
- `etctd approve <id>`
- `etctd reject <id> --reason "..."`

### Creating and Dependencies
- `etctd create "title" --type task --priority P2`
- `etctd dep add <id> <depends-on-id>`
- `etctd dep list <id>`
- `etctd dep remove <id> <depends-on-id>`

See `references/quick_reference.md` for a concise command reference.

## For AI Agents

Always start with:

```bash
etctd usage --new-session
```

Before ending work, always run `etctd handoff` on in-progress issues.
