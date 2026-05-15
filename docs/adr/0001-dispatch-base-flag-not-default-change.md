# `pb dispatch --base` is an opt-in flag, not a default behavior change

`pb dispatch` currently roots every run at the repo default branch (origin/HEAD → main/master), ignoring the user's current branch except as a last-resort fallback. To let users dispatch on top of feature-branch work, we add an opt-in `--base <commit-ish>` flag. We deliberately do not change the default to "follow current HEAD" — that would silently change destination semantics for the `direct` branch strategy, where issue commits merge straight into the base.

## Considered options

- **Change default to current HEAD.** Most ergonomic, but anyone running `git checkout some-experiment && pb dispatch` with `branch_strategy: direct` would suddenly merge autonomous-agent commits into `some-experiment` instead of `main`. Footgun outweighs convenience.
- **Restrict `--base` to `integration` strategy only.** Paternalistic — `direct` is already an opt-in strategy; if the user chose it, they can wield it. Existing dirty-tree check and HEAD-restore in `MergeBack` apply equally.
- **Add a `pipeline.base_branch` config knob in addition to the flag.** Rejected for v1: a persistent config value would make the "integration branch already exists, `--base` ignored" warning fire on every re-run with no clean way to suppress. Flag-only keeps that warning meaningful. Non-breaking to add later.

## Consequences

- `--base` is symmetric across strategies: for `integration` it forks `<user>/<slug>` off the supplied ref; for `direct` it merges issue branches into the supplied ref.
- Accepts any commit-ish (branch, remote-tracking ref, tag, SHA) — validated via `git rev-parse --verify <base>^{commit}`.
- When `<user>/<slug>` already exists from a prior run, `--base` is honored only at creation. If the flag is explicitly passed on a re-run, dispatch warns and continues with the existing branch rather than re-forking (which would destroy committed work).
- `--base` bypasses the detached-HEAD error path in `defaultBranch()` as a side effect.
