package orchestrator

import (
	"context"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	orchestratorLocalWorkSlowAfter = 5 * time.Second
	orchestratorLocalWorkSlowEvery = 10 * time.Second
)

// startSchedulerLocalWork marks a local, CPU-bound scheduler phase as
// active and logs slow-phase breadcrumbs. It intentionally does not
// enforce a wall-clock deadline: large customer repositories can make
// legitimate validation passes slower than small local repos, and a
// fixed timeout would trade a diagnosability problem for correctness
// loss. Cancellation remains cooperative through CancelContext.
func (o *Orchestrator) startSchedulerLocalWork(stage types.PipelineStage, phase string) func() {
	if o != nil && o.emit != nil {
		o.emit(render.Event{
			Kind:      render.EventLocalWorkStart,
			Timestamp: time.Now(),
			Stage:     stage,
		})
	}
	start := time.Now()
	logging.Debug("[diag scheduler] phase=%s stage=%s start", phase, stage)

	ctx := context.Background()
	if o != nil {
		ctx = o.CancelContext()
	}
	ctxDone := ctx.Done()
	done := make(chan struct{})
	var once sync.Once
	go func() {
		timer := time.NewTimer(orchestratorLocalWorkSlowAfter)
		defer timer.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctxDone:
				logging.Warning("[diag scheduler] phase=%s stage=%s canceled while local work is still unwinding err=%v",
					phase, stage, ctx.Err())
				ctxDone = nil
			case <-timer.C:
				logging.Warning("[diag scheduler] phase=%s stage=%s still running elapsed=%s",
					phase, stage, time.Since(start).Round(time.Second))
				timer.Reset(orchestratorLocalWorkSlowEvery)
			}
		}
	}()

	return func() {
		once.Do(func() {
			close(done)
			if err := ctx.Err(); err != nil {
				logging.Warning("[diag scheduler] phase=%s stage=%s done after cancel elapsed=%s err=%v",
					phase, stage, time.Since(start).Round(time.Millisecond), err)
				return
			}
			logging.Debug("[diag scheduler] phase=%s stage=%s done elapsed=%s",
				phase, stage, time.Since(start).Round(time.Millisecond))
		})
	}
}

// finishSchedulerLocalWork closes the slow-phase logger and records a
// cooperative cancel as the current task error. Callers return from the
// scheduler loop when this returns true.
func (o *Orchestrator) finishSchedulerLocalWork(stop func(), phase string, iter int) bool {
	if stop != nil {
		stop()
	}
	if o == nil {
		return false
	}
	if cerr := o.checkCanceled("scheduler."+phase, iter); cerr != nil {
		if o.busCtx != nil {
			o.busCtx.TaskState.LastError = cerr.Error()
		}
		return true
	}
	return false
}
