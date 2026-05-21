# Changelog

All notable changes to plan-bender are documented here. Format loosely follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); plan-bender is pre-1.0 so breaking changes ship in patch releases until v1.

## v0.0.52

### Added

- Published JSON Schema for `.plan-bender.json` at `schema/plan-bender.schema.json`. Editors that honor `$schema` now offer validation and autocomplete for the config.
- `pb setup` writes a `$schema` key into scaffolded `.plan-bender.json` files, pointing at the published schema so new configs get editor validation out of the box.

### Changed

- Starter config no longer enables the `pi` agent by default; freshly scaffolded configs ship with only `claude-code` enabled.

## v0.0.51

### BREAKING

- **`after_batch` hook cwd changed.** Previously the hook ran with cwd set to the parent repo root. It now runs inside the per-slug integration worktree at `{worktree_base}/{repoName}-wt/{slug}/_integration`, with the just-merged commits checked out. Hooks that resolve paths relative to cwd (e.g. `./scripts/post-merge.sh`, `pnpm test`) will now see the integration worktree's tree rather than the parent repo's. If a hook needs the parent repo root, capture it explicitly before dispatch (e.g. `PB_REPO_ROOT=$(git -C "$(git rev-parse --show-toplevel)" rev-parse --show-toplevel)` is no longer a no-op from inside the worktree — pass it through your environment instead). See [ADR-0003](docs/adr/0003-merge-in-dedicated-integration-worktree.md).

### Added

- `worktree_base` top-level config: relocate dispatch worktrees under any directory. Accepts absolute paths, `~`-prefixed paths, and repo-relative paths (`./...`). Default (`""`) preserves the legacy `{repoParent}/{repoName}-wt/...` layout, so existing users see no path change. See [docs/configuration.md](docs/configuration.md).
- Per-slug dispatch lock at `{plans_dir}/{slug}/.dispatch.lock`. A second `pba dispatch` against the same slug fails fast with the holding pid and lock path. Different slugs in the same repo can now run concurrently.

### Changed

- Merge-back runs entirely inside a dedicated per-slug integration worktree. The parent repo's HEAD is no longer captured, modified, or restored by dispatch, and dispatch no longer refuses to run on a dirty parent working tree. `pba worktree gc <slug>` cleans up the integration worktree along with merged issue worktrees; an in-flight HITL-only exit preserves it for resumption. See [ADR-0003](docs/adr/0003-merge-in-dedicated-integration-worktree.md).
