package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestInterruptibleRequestBudgetBoundsOnlyOptedInNonResponsiveLeaf(t *testing.T) {
	release, entered, returned := make(chan struct{}), make(chan struct{}), make(chan struct{})
	adapter := &requestBudgetProbe{run: func(context.Context, ChatOptions) (Response, error) {
		close(entered)
		<-release
		defer close(returned)
		return Response{Content: "late response"}, nil
	}}
	done := make(chan error, 1)
	go func() {
		_, err := ChatWithInterruptibleRequestBudget(context.Background(), NewTelemetryAdapter(NewFallbackAdapter(adapter), func(RequestTelemetry) {}), nil, nil, ChatOptions{}, 20*time.Millisecond)
		done <- err
	}()
	<-entered
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("non-responsive leaf must remain bounded: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("explicit interruptible request wait did not return on its leaf budget")
	}
	// The executor cannot forcibly stop foreign code. Its buffered result
	// channel must nevertheless let a late adapter finish when that code exits.
	close(release)
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("late adapter did not exit after being released")
	}
}

func TestOrdinaryRequestBudgetPreservesSynchronousAdapterLifetime(t *testing.T) {
	release, sawCancellation := make(chan struct{}), make(chan struct{})
	adapter := &requestBudgetProbe{run: func(ctx context.Context, _ ChatOptions) (Response, error) {
		<-ctx.Done()
		close(sawCancellation)
		<-release
		return Response{}, ctx.Err()
	}}
	done := make(chan error, 1)
	go func() {
		_, err := ChatWithRequestBudget(context.Background(), NewTelemetryAdapter(NewFallbackAdapter(adapter), func(RequestTelemetry) {}), nil, nil, ChatOptions{}, 20*time.Millisecond)
		done <- err
	}()
	<-sawCancellation
	select {
	case err := <-done:
		t.Errorf("ordinary evaluator budget must not silently gain an asynchronous leaf lifetime: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("ordinary budget must retain adapter cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ordinary synchronous leaf did not finish after release")
	}
}
