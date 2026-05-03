#!/usr/bin/env bash
# Bump version in all rival skill SKILL.md files (embedded + project-level).
# Usage: ./scripts/bump-skill-versions.sh 3.7.0

set -euo pipefail

VERSION="${1:?Usage: $0 <version>}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

SKILL_DIRS=(
  "$ROOT/rival/internal/skills/rival-codex-only"
  "$ROOT/rival/internal/skills/rival-gemini-only"
  "$ROOT/rival/internal/skills/rival-claude-only"
  "$ROOT/rival/internal/skills/rival-review"
  "$ROOT/.claude/skills/rival-codex-only"
  "$ROOT/.claude/skills/rival-gemini-only"
  "$ROOT/.claude/skills/rival-claude-only"
  "$ROOT/.claude/skills/rival-review"
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
