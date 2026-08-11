package orchestrator

import "github.com/hanchaoqun/codrax/internal/agent"

// finalizerNoVisibleOutputFallback is retained only as the read-scheduler L1
// compatibility seam. Active streaming reasoning is model progress, so lack
// of visible answer text is not authority for the system to stop waiting or
// synthesize a replacement conclusion from earlier evidence. The built-in
// adapter therefore never emits that legacy timeout, and this seam refuses a
// provider-specific legacy occurrence fail-closed. Normal scheduler recovery
// may retry the model, recover a prior model-authored draft, or surface the
// transport failure; it must not manufacture an answer.
//
// Keep the call sites byte-stable: runReadSchedulerLoop is protected by the L1
// read-mode preservation contract.
func (o *Orchestrator) finalizerNoVisibleOutputFallback(_ error) *agent.StageOutput {
	return nil
}
