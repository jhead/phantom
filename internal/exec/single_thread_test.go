package exec

import (
	"context"
	"testing"
)

type testAction struct {
	t               *testing.T
	expectedContext context.Context
	expectedResult  int
	success         chan int
}

func (a testAction) Execute(ctx context.Context) error {
	if ctx != a.expectedContext {
		a.t.Error("Context was not propagated to action")
	}

	a.success <- a.expectedResult

	return nil
}

func TestSingleThread(t *testing.T) {
	exec := NewSingleThread()
	defer exec.Close()

	ctx := context.Background()
	success := make(chan int)
	expectedResult := 123

	action := testAction{t, ctx, expectedResult, success}

	exec.Execute(ctx, action)

	if result := <-success; result != expectedResult {
		t.Errorf("Unexpected result: got %v expected %v", result, expectedResult)
	}
}
