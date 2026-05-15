# Linear sync publishes full plan content one-way; the plan JSON stays canonical

`pb sync linear push` previously sent only an issue's `name` and `outcome` to Linear, and created the Linear project with nothing but a name — so projects landed empty and tickets were a single sentence. We widen the sync to publish the full plan: a structured markdown body for every issue and the project, plus labels and estimate. The sync stays deliberately asymmetric — push treats the on-disk plan JSON as canonical and overwrites Linear body content wholesale; pull mirrors back only the three fields a human naturally edits in Linear (`status`, `priority`, `assignee`).

## Considered options

- **Bidirectional sync with conflict detection.** Track per-field synced hashes, refuse to push or pull when the other side has diverged, surface merge UX. Rejected: plan-bender's model is "an agent authors the plan JSON, Linear is where humans watch progress." A planning tool does not need a merge engine; the code and UX cost dwarfs the benefit.
- **Linear as source of truth.** Once published, Linear edits to title/description/labels win and pull rewrites the JSON. Rejected: the plan JSON is the artifact agents read to do work; a rendered markdown body cannot be reliably parsed back into structured fields, so Linear-wins would corrupt the canonical form.
- **Hash-based change detection on re-push.** Skip the `issueUpdate` call when nothing changed, to avoid bumping `updatedAt` on every Linear issue each sync. Deferred, not rejected: it needs a persisted hash (cleanest spot is a map on the PRD's `linear` ref), which is a schema change that ripples into validation and `pb migrate`. That belongs in its own change, not riding along here.
- **Issue relations (`blocked_by`/`blocking`) as Linear issue relations.** Deferred to v2: relation reconciliation has real idempotency questions (detecting a relation removed locally and removing it remotely) that warrant their own pass.

## Consequences

- **Push overwrites Linear body content on every sync**, including a new `projectUpdate` mutation that refreshes the project `description` and `content`. A teammate who edits an issue body or project doc in Linear's UI will have it stomped on the next push. The rendered body carries a footer disclaimer saying exactly this, pointing at the source JSON path.
- **Pull stays thin** — only `status`, `priority`, `assignee` flow Linear → local. Title, description, and labels edited in Linear are not pulled back.
- **Labels are auto-created** in Linear (team-scoped) on first use, with case-insensitive lookup to avoid near-duplicate labels from plan-file typos.
- **Estimate is a direct `points → estimate` passthrough**, skipped entirely when the Linear team has estimation disabled (detected via `issueEstimationType` on the existing team query). No snap-to-scale; an off-scale value surfaces as a normal per-issue `SyncError`.
- **`assignee` is never pushed** — not even on create. Linear owns it; pull carries it back. A local `assignee` set by a plan-bender agent mid-implementation will not propagate to Linear; reconciling that is left to a future decision.
- **Every re-push bumps `updatedAt`** on every linked Linear issue, so issue watchers get a notification on each no-op sync. Accepted for v1; revisit if it becomes a complaint (see the deferred change-detection option).
