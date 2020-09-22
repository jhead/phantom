package exec

import (
	"context"
)

// Credit: https://rodaine.com/2018/08/x-files-sync-golang/

// An Action performs a single arbitrary task.
type Action interface {
	// Execute performs the work of an Action. This method should make a best
	// effort to be cancelled if the provided ctx is cancelled.
	Execute(ctx context.Context) error
}

// An Executor performs a set of Actions. It is up to the implementing type
// to define the concurrency and open/closed failure behavior of the actions.
type Executor interface {
	// Execute performs all provided actions by calling their Execute method.
	// This method should make a best-effort to cancel outstanding actions if the
	// provided ctx is cancelled.
	Execute(ctx context.Context, actions ...Action)
}

// An action with an associated context packged for convenience when sending it
// over a channel.
type actionWithContext struct {
	Action
	ctx context.Context
}
