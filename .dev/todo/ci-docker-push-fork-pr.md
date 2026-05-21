# Update GitHub CI Docker Image Push for Same-Repo PRs Only

mode: bug
state: review
root_git: ../substreams.worktrees/fix/ci-docker-push-fork-pr
worktree: ../substreams.worktrees/fix/ci-docker-push-fork-pr
branch: fix/ci-docker-push-fork-pr
target_branch: develop

> **Resume protocol:** read **Dev Feedback** and the **State Tracker** below first, then jump to the
> step marked `Current`. Ensure that you are in the correct worktree and branch according to preamble here. Update current with Developer feedback and update the tracker after every meaningful change.
> Do not mutate completed steps; append a new entry instead.

---

## Initial Description

Update GitHub CI so that Docker images are pushed only if pull request is made within same repo (essentially building the Docker image but omitting the push when running for forks PR).

## Dev Feedback

## Spec & Implementation

### Root Cause Analysis

The `.github/workflows/docker.yml` workflow already had a correct condition on the "Build and push Docker image" step (line 79):

```yaml
push: ${{ github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository }}
```

However, two steps were still running unconditionally for fork PRs:

1. **"Log in to the Container registry"** — The login is unnecessary for fork PRs since no push will occur. While the login itself might succeed with a read-only GITHUB_TOKEN, it is wasteful and could cause confusion.

2. **"Docker Scout"** — This step tries to pull the just-pushed image from the registry by SHA and scan it for CVEs. For fork PRs, no image was pushed, so Docker Scout would fail trying to find an image that doesn't exist in the registry.

### Fix Applied

Both steps now have the same condition guard as the push flag:

```yaml
if: ${{ github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository }}
```

This ensures:
- **Non-PR triggers** (push to `develop`, tag pushes): all steps run as before — login, build+push, Docker Scout.
- **Same-repo PRs**: all steps run — login, build+push, Docker Scout with PR comment writing.
- **Fork PRs**: login is skipped, image is built but not pushed, Docker Scout is skipped.

The `write-comment` condition on Docker Scout was already correct (only writes comments for same-repo PRs).

### Files Changed

- `.github/workflows/docker.yml` — Added `if:` conditions to "Log in to the Container registry" and "Docker Scout" steps.
- `docs/release-notes/change-log.md` — Added Unreleased section with the fix entry.

## State Tracker

**Last Updated:** 2026-05-21
**Current Step:** Step 2 — Implementation Complete, Ready for Review
**Status:** In review

### Step 1 — Begin Implementation (completed)
- Explored `.github/workflows/docker.yml`
- Identified that "Log in to the Container registry" and "Docker Scout" steps ran unconditionally for fork PRs
- The push condition on the build step was already correct

### Step 2 — Implementation Complete (current)
- Added `if:` condition to "Log in to the Container registry" step
- Added `if:` condition to "Docker Scout" step
- Updated `docs/release-notes/change-log.md` with Unreleased section
- Committed changes to `fix/ci-docker-push-fork-pr` branch
