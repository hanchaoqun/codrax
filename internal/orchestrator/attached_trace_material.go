package orchestrator

import (
	"context"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// SetAttachedHitrace replaces the sticky trace preview. A new plain-text
// attachment cannot inherit a prior file's full-material receipt.
func (o *Orchestrator) SetAttachedHitrace(trace string) {
	o.attachedHitrace = trace
	o.attachedTraceMaterial = nil
}

func (o *Orchestrator) AttachedHitrace() string                { return o.attachedHitrace }
func (o *Orchestrator) SetAttachedHitraceSource(source string) { o.attachedHitraceSource = source }
func (o *Orchestrator) AttachedHitraceSource() string          { return o.attachedHitraceSource }

// SetAttachedTraceMaterial must follow SetAttachedHitrace. Run validates its
// immutable preview and all physical generations before any investigation.
func (o *Orchestrator) SetAttachedTraceMaterial(material *attachment.TraceMaterial) {
	o.attachedTraceMaterial = material
}
func (o *Orchestrator) AttachedTraceMaterial() *attachment.TraceMaterial {
	return o.attachedTraceMaterial
}

func (o *Orchestrator) validateAttachedTraceInputs(ctx context.Context) error {
	if err := validateRuntimeTraceInputsBeforeInvestigation(ctx, o.attachedHitrace); err != nil {
		return err
	}
	if o.attachedTraceMaterial == nil {
		return nil
	}
	if err := o.attachedTraceMaterial.Validate(ctx, o.attachedHitrace); err != nil {
		return err
	}
	return tracequery.ValidateTraceInputPath(ctx, o.attachedTraceMaterial.QueryPath())
}
