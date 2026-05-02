// Package status owns every issue-status write through a single CAS-checked,
// allowed-transitions-enforced Owner. Callers never touch the plan lock or the
// underlying store directly.
package status

// Status is the typed counterpart to schema.IssueYaml.Status. The string values
// match the schema field exactly so the boundary adapter in issue #2 is a pure
// string conversion with no translation table.
type Status string

const (
	StatusBacklog    Status = "backlog"
	StatusTodo       Status = "todo"
	StatusInProgress Status = "in-progress"
	StatusBlocked    Status = "blocked"
	StatusInReview   Status = "in-review"
	StatusDone       Status = "done"
	StatusCanceled   Status = "canceled"
)
