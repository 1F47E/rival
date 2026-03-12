#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(dirname "$SCRIPT_DIR")"
TARGET_DIR="$HOME/.claude/skills"

mkdir -p "$TARGET_DIR"

for skill in rival-codex rival-gemini rival-megareview; do
    src="$REPO_DIR/.claude/skills/$skill"
    dst="$TARGET_DIR/$skill"

    if [ -L "$dst" ]; then
        echo "Updating symlink: $dst"
        rm "$dst"
    elif [ -d "$dst" ]; then
        echo "Warning: $dst exists and is not a symlink. Skipping."
        continue
    fi

    ln -s "$src" "$dst"
    echo "Installed: $dst -> $src"
done

echo ""
echo "Skills installed. Available commands:"
echo "  /rival-codex      — Run Codex via rival"
echo "  /rival-gemini     — Run Gemini via rival"
echo "  /rival-megareview — Run both Codex + Gemini in parallel"
