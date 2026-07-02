#!/usr/bin/env bash
# Bump version in all rival skill SKILL.md files (embedded + project-level).
# Usage: ./scripts/bump-skill-versions.sh 3.7.0

set -euo pipefail

VERSION="${1:?Usage: $0 <version>}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Source of truth: the embedded skills compiled into the binary. The repo-root
# .claude/skills/ copies were removed — `rival install` copies these out to
# ~/.claude/skills/ on install/update, so there is no second copy to keep in sync.
# gemini-only/claude-only are deprecated (removed on install) but still embedded,
# so their version stays synced too.
SKILL_DIRS=(
  "$ROOT/rival/internal/skills/rival-codex-only"
  "$ROOT/rival/internal/skills/rival-gemini-only"
  "$ROOT/rival/internal/skills/rival-claude-only"
  "$ROOT/rival/internal/skills/rival-antigravity-only"
  "$ROOT/rival/internal/skills/rival-plan-codex"
  "$ROOT/rival/internal/skills/rival-plan-fable"
  "$ROOT/rival/internal/skills/rival-fable-only"
  "$ROOT/rival/internal/skills/rival-review"
)

for dir in "${SKILL_DIRS[@]}"; do
  file="$dir/SKILL.md"
  if [[ -f "$file" ]]; then
    sed -i '' "s/^version: .*/version: $VERSION/" "$file"
    echo "  ✓ $file → $VERSION"
  else
    echo "  ✗ $file not found"
  fi
done

echo "Done."
