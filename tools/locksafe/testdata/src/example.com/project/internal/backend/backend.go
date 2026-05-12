package backend

import "context"

type Backend interface {
	PullProject(context.Context, string) error
}
