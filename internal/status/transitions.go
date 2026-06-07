package status

// allowed enumerates every legal (current → target) edge. Hardcoded because
// workflow transitions are semantic invariants, not configuration.
var allowed = map[Status][]Status{
	StatusTodo:       {StatusInProgress, StatusInReview, StatusBlocked, StatusCanceled},
	StatusInProgress: {StatusInReview, StatusBlocked, StatusCanceled, StatusNeedsInput},
	StatusBacklog:    {StatusInProgress, StatusBlocked, StatusInReview},
	StatusInReview:   {StatusDone, StatusBlocked},
	StatusBlocked:    {StatusTodo, StatusInProgress},
	StatusNeedsInput: {StatusTodo, StatusInProgress},
}

func isAllowed(from, to Status) bool {
	for _, s := range allowed[from] {
		if s == to {
			return true
		}
	}
	return false
}
