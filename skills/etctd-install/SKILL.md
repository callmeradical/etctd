---
name: etctd-install
description: Install etctd and configure AGENTS.md with the managed etctd usage protocol for smith-replica style agent workflows.
---

# etctd Install

## Overview

Use this skill to set up `etctd` usage in a repo and stamp the standardized usage instructions into `AGENTS.md`.

## What this skill sets up

1. Build/install `etctd`
2. Add/update managed `AGENTS.md` usage block
3. Ensure agents run `etctd usage --new-session` at conversation start

## Install etctd

From this repository:

```bash
go build ./cmd/etctd
```

Optional global install:

```bash
go install ./cmd/etctd
```

## Configure AGENTS.md

Run the bundled script:

```bash
bash skills/etctd-install/scripts/write_agents_md.sh
```

Or target a specific file:

```bash
bash skills/etctd-install/scripts/write_agents_md.sh --path services/smith-replica/AGENTS.md
```

The script manages this marker block:

- `<!-- etctd-usage:start -->`
- `<!-- etctd-usage:end -->`

It is idempotent and replaces existing managed content in place.
