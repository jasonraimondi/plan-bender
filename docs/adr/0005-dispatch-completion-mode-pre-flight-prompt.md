# Skill prompts for completion mode upfront, executes it after dispatch returns 0

`bender-implement-prd` and standalone `bender-implement-issue` previously always pushed and opened a PR at the tail — fine for combined feature work, friction for the common case of "merge directly into the branch I'm on." We replace the unconditional PR step with a three-mode prompt — `merge` / `branch` / `pr` — asked **before** dispatch runs, executed only on exit 0.

`merge` lands the integration branch (`<user>/<slug>`) or per-issue branch onto the **landing branch** (the parent repo's HEAD when the skill was invoked) with `--no-ff`, then `git branch -d`s the source. `branch` is a no-op — leaves the branch local, no push. `pr` is the prior behavior — push and `gh pr create`.

## Considered options

- **Ask after dispatch returns.** Outcome-informed but breaks the AFK ergonomics — dispatch can run for tens of minutes, and a tail-end blocking prompt forces the operator to be present at completion. The pre-flight choice is "what kind of work is this", not "how did the work go" — outcome rarely changes the operator's mind.
- **Add a `.plan-bender.json` knob (`completion_mode: merge|branch|pr|ask`).** Rejected for v1. The prompt is ~2 seconds; a config-skip is a footgun if the operator forgets they set `merge` while on a protected landing branch. Non-breaking to add later.
- **Auto-flip `in-review → done` on `merge`.** Blocked by the transition table (`in-progress → done` is forbidden) and there is no CLI for `in-review → done` outside `dispatch.MergeBack`. Adding `pba finalize` was deferred — the standalone PR path already leaves issues at `in-review` indefinitely (pre-existing gap, not introduced here).
- **Refuse `merge` when landing branch is the default branch.** Rejected per ADR 0001's spirit: the wariness there was about *silent* defaults, not explicit prompted actions. Operator picked `merge` knowingly; honor it.

## Consequences

- **Parent HEAD is no longer invariant post-dispatch.** ADR 0003 established that *dispatch* never moves parent HEAD. `merge` mode runs after dispatch returns and does move HEAD (`git merge --no-ff <user>/<slug>` while on the landing branch). The invariant applies only during the dispatch run.
- **Pre-flight validation shapes the menu.** When the landing tree is dirty or HEAD is detached, `merge` is hidden from the option list with a one-line reason. `pr` and `branch` always available on exit 0.
- **Choice fires only on exit 0.** Exit 2 (HITL pending) and other non-zero exits ignore the upfront choice and surface the existing diagnostic. The operator's choice is "captured" upfront but conditional on clean completion.
- **Symmetric across `bender-implement-prd` and standalone `bender-implement-issue`.** Same three modes; only the branch being acted on differs (integration branch vs per-issue branch). Worktree-mode invocations of `implement-issue` (under dispatch) skip the prompt entirely — dispatch owns the merge-back.
- **`bender-implement-hitl` is out of scope** for v1. Its per-issue PR step has the same shape but a different concern (resolving human-gated work, not deciding integration). Extend later if friction is reported.
