# Releasing Rival

Rival's GitHub Actions release workflow is the only publisher. A pushed `v*`
tag triggers GoReleaser, which creates the GitHub release, uploads four platform
archives plus `checksums.txt`, and updates `rival.rb` in
`1F47E/homebrew-tap`.

Do not run `goreleaser release` locally for the same tag. A second publisher
collides with the CI-created assets and can leave an otherwise valid release
workflow marked failed.

## Release checklist

Set the version without the `v` prefix:

```bash
VERSION=3.23.0
```

1. Update all embedded skill versions:

   ```bash
   ./scripts/bump-skill-versions.sh "$VERSION"
   ```

2. Run the release gate before creating a tag:

   ```bash
   cd rival
   make test
   cd ..
   ```

   `make test` runs lint, race-enabled Go tests, and a versioned build. The
   release workflow itself builds artifacts but does not duplicate this test
   gate.

3. Review and commit the complete release state, then create a lightweight tag
   on that commit:

   ```bash
   git status --short
   git diff --check
   git add -A
   git diff --cached --check
   git diff --cached --stat
   git commit -m "release: v${VERSION}"
   git tag "v${VERSION}"
   ```

   Do not proceed with unrelated worktree changes. `git add -A` is appropriate
   only after the status review confirms that the complete tree is intended for
   this release.

4. Push the release commit and tag:

   ```bash
   git push origin master
   git push origin "v${VERSION}"
   ```

5. Watch the `Release` workflow for that tag:

   ```bash
   gh run list --workflow Release --limit 5
   gh run watch <run-id>
   ```

   Select the run whose event/tag corresponds to `v${VERSION}`.

6. Verify the published release:

   ```bash
   gh release view "v${VERSION}"
   ```

   It must contain archives for Darwin and Linux on both amd64 and arm64, plus
   `checksums.txt`. Also confirm the latest `1F47E/homebrew-tap` commit updated
   the formula for the same tag.

7. Verify the user installation after the tap update:

   ```bash
   brew update
   brew uninstall rival
   brew install 1F47E/tap/rival
   rival install --force
   rival version
   ```

   If Rival was not already installed, omit the uninstall command. The reported
   version must match `v${VERSION}`. Restart or reload Claude Code after
   refreshing the embedded skills.
