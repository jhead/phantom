package exec

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
)

type SingleThreadExecutor struct {
	actionChan chan actionWithContext
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewSingleThread() SingleThreadExecutor {
	ctx, cancel := context.WithCancel(context.Background())

	exec := SingleThreadExecutor{
		make(chan actionWithContext),
		ctx,
		cancel,
	}

	go exec.actionLoop()

	return exec
}

func (exec SingleThreadExecutor) Execute(ctx context.Context, actions ...Action) {
	for _, action := range actions {
		exec.actionChan <- actionWithContext{action, ctx}
	}
}

func (exec SingleThreadExecutor) Close() {
	exec.cancel()
}

func (exec SingleThreadExecutor) actionLoop() {
	for {
		select {
		case action := <-exec.actionChan:
			if err := action.Execute(action.ctx); err != nil {
				log.Error().Msgf("Executor encountered error: %v", err)
			}
		case <-exec.ctx.Done():
			fmt.Println("Executor stopped")
			return
		}
	}
}
