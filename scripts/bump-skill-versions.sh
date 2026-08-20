#!/usr/bin/env bash
# Bump version in all rival skill SKILL.md files (embedded + project-level).
# Usage: ./scripts/bump-skill-versions.sh 3.7.0

set -euo pipefail

VERSION="${1:?Usage: $0 <version>}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Source of truth: the embedded skills compiled into the binary. The repo-root
# .claude/skills/ copies were removed — `rival install` copies these out to
# ~/.claude/skills/ on install/update, so there is no second copy to keep in sync.
# Deprecated skill directories are skipped because they are removed on install.
# Derived from embed.go's Names list rather than repeated here: a static copy
# silently skips a newly added skill, and a skill whose version never moves is
# never reinstalled, because `rival install` skips files whose versions match.
SKILLS_DIR="$ROOT/rival/internal/skills"
SKILL_DIRS=()
while IFS= read -r name; do
	SKILL_DIRS+=("$SKILLS_DIR/$name")
done < <(
	sed -n 's/^var Names = \[\]string{\(.*\)}$/\1/p' "$SKILLS_DIR/embed.go" |
		tr ',' '\n' | tr -d ' "'
)

if [ ${#SKILL_DIRS[@]} -eq 0 ]; then
	echo "error: could not read skill names from embed.go" >&2
	exit 1
fi

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
