#!/usr/bin/env bash
set -euo pipefail

TARGET_PATH="AGENTS.md"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --path)
      if [[ $# -lt 2 ]]; then
        echo "Error: --path requires a value" >&2
        exit 1
      fi
      TARGET_PATH="$2"
      shift 2
      ;;
    -h|--help)
      cat <<'EOF'
Usage: write_agents_md.sh [--path <AGENTS.md path>]

Writes or updates a managed etctd usage section in AGENTS.md.

Examples:
  write_agents_md.sh
  write_agents_md.sh --path services/smith-replica/AGENTS.md
EOF
      exit 0
      ;;
    *)
      echo "Error: unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

mkdir -p "$(dirname "$TARGET_PATH")"
if [[ ! -f "$TARGET_PATH" ]]; then
  : > "$TARGET_PATH"
fi

python3 - "$TARGET_PATH" <<'PY'
from pathlib import Path
import re
import sys

path = Path(sys.argv[1])
text = path.read_text(encoding="utf-8")

legacy_pattern = re.compile(
    r"## MANDATORY: Use td for Task Management\s*\n"
    r"\s*You must run td usage --new-session at conversation start \(or after /clear\) to see current work\.\s*\n"
    r"\s*Use td usage -q for subsequent reads\.\s*\n?",
    re.MULTILINE,
)
text = re.sub(legacy_pattern, "", text)

start = "<!-- etctd-usage:start -->"
end = "<!-- etctd-usage:end -->"

block = """<!-- etctd-usage:start -->
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
"""

if start in text and end in text:
    start_idx = text.index(start)
    end_idx = text.index(end) + len(end)
    prefix = text[:start_idx].rstrip()
    suffix = text[end_idx:].lstrip("\n")

    parts = []
    if prefix:
        parts.append(prefix)
    parts.append(block.strip())
    if suffix:
        parts.append(suffix.rstrip())
    out = "\n\n".join(parts) + "\n"
else:
    stripped = text.rstrip()
    if stripped:
        out = stripped + "\n\n" + block.strip() + "\n"
    else:
        out = block.strip() + "\n"

path.write_text(out, encoding="utf-8")
print(f"Updated {path}")
PY
