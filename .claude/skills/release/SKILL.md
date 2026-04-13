---
name: release
description: Tag a new semver release, wait for the GitHub Actions release workflow to succeed, and confirm the Go module is available.
disable-model-invocation: true
---

# Release

Perform the full release flow for this Go module. Steps:

1. **Determine the next version**: Find the latest tag with `git tag --sort=-v:refname | head -1`, then review the git log since that tag (`git log <latest-tag>..HEAD --oneline`) to understand what changed. If there are no new commits since the last tag, stop and inform the user there is nothing to release. Otherwise, choose the appropriate semver bump (major, minor, or patch) based on the changes. Present the proposed version and changelog to the user for confirmation before proceeding.
2. **Tag and push**: Create the new tag on the current commit and push it to origin.
3. **Wait for the release workflow**: Use `gh run list --workflow=release.yml` to find the Actions run triggered by the tag push, then `gh run watch <id>` to wait for it to complete. If it fails, report the error and stop.
4. **Confirm**: Verify the release was created with `gh release view <tag>` and print a summary.
