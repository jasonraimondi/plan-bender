package status

// allowed enumerates every legal (current → target) edge. Sourced from a
// survey of every status-write call site:
//
//	todo → in-progress: bender-implement-issue marks an issue in-progress on entry.
//	todo → in-review:   cli/complete on a todo issue (sub-agent skipped in-progress).
//	todo → blocked:     dispatcher setup failures and subprocess failure paths.
//	todo → canceled:    human cancellation.
//	in-progress → in-review: cli/complete on the typical path.
//	in-progress → blocked:   subprocess failure paths and dispatcher setup failures.
//	in-progress → canceled:  human cancellation.
//	backlog → in-review:     cli/complete supports backlog as a from-state.
//	in-review → done:        dispatcher merge-back after a clean merge.
//	in-review → blocked:     dispatcher merge-back on merge conflict.
//	blocked → todo:          cli/retry resets a blocked issue.
//	blocked → in-progress:   resume after a manual unblock.
//
// The table is intentionally hardcoded — the PRD documents that workflow
// transitions are semantic invariants, not configuration.
var allowed = map[Status][]Status{
	StatusTodo:       {StatusInProgress, StatusInReview, StatusBlocked, StatusCanceled},
	StatusInProgress: {StatusInReview, StatusBlocked, StatusCanceled},
	StatusBacklog:    {StatusInReview},
	StatusInReview:   {StatusDone, StatusBlocked},
	StatusBlocked:    {StatusTodo, StatusInProgress},
}

func isAllowed(from, to Status) bool {
	for _, s := range allowed[from] {
		if s == to {
			return true
		}
	}
	return false
}
