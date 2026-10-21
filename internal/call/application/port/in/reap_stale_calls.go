package in

import "context"

// ReapStaleCallsCommand is the hard-crash backstop: a media pod that dies
// without ever publishing call.<id>.ended leaves its calls stuck 'active'
// forever. Run periodically, it treats a live call whose node is no longer
// in the registry (or whose room the node no longer has) as dead and
// closes it server-side. Carries no fields: it sweeps every live call.
type ReapStaleCallsCommand struct{}

// ReapStaleCallsUseCase handles ReapStaleCallsCommand, returning the
// number of calls it ended.
type ReapStaleCallsUseCase interface {
	Handle(ctx context.Context, cmd ReapStaleCallsCommand) (int, error)
}
