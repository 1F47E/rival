---
name: rival-release
description: Release rival — commit, push, tag, goreleaser, update brew formula, reinstall, notify. Use when user says "release" or "commit push release".
argument-hint: "<version>"
allowed-tools: Bash, Read, Write, Edit, Glob, Grep
---

# Rival Release

Full release pipeline for the rival Go binary.

## Steps

1. **Bump skill versions** — run `./scripts/bump-skill-versions.sh <version>` to update all 6 SKILL.md files
2. **Commit** all pending changes (skills, code, README)
3. **Tag** `v<version>` on the release commit
4. **Push** `master` + tags to origin
5. **Goreleaser** — run with `GITHUB_TOKEN=$(gh auth token) GITLAB_TOKEN="" goreleaser release --clean` from `rival/` dir
   - Expected: homebrew cask step fails with 403 (no tap token) — this is normal
6. **Update brew formula** — read SHA256 from `rival/dist/*.tar.gz`, update `~/dev/homebrew-tap/rival.rb`, commit + push
7. **Brew reinstall** — `brew uninstall rival && brew install 1f47e/tap/rival`
8. **Install skills** — `rival install --force`
9. **Verify** — `rival version` should show the new version
10. **Notify** — send Telegram notification via `/notify` skill with version and changelog summary
