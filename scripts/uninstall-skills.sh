#!/usr/bin/env bash
set -euo pipefail

TARGET_DIR="$HOME/.claude/skills"

for skill in rival-codex rival-gemini; do
    dst="$TARGET_DIR/$skill"
    if [ -L "$dst" ]; then
        rm "$dst"
        echo "Removed: $dst"
    elif [ -d "$dst" ]; then
        echo "Warning: $dst is not a symlink. Remove manually if desired."
    else
        echo "Not found: $dst"
    fi
done

echo "Skills uninstalled."
