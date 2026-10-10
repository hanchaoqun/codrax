package orchestrator

import "github.com/hanchaoqun/codrax/internal/loginput"

func (o *Orchestrator) replaceAttachedLog(body string) {
	o.attachedLog = body
	o.attachedLogCatalog = nil
}

// SetAttachedLogCatalog follows SetAttachedLog. Only a live preparation can
// supply this handle; a saved preview has no complete-source authority.
func (o *Orchestrator) SetAttachedLogCatalog(catalog *loginput.Catalog) {
	o.attachedLogCatalog = catalog
}

func (o *Orchestrator) AttachedLogCatalog() *loginput.Catalog {
	return o.attachedLogCatalog
}

func (o *Orchestrator) bindRuntimeAttachments() {
	o.busCtx.AttachedLog = o.attachedLog
	o.busCtx.AttachedLogCatalog = o.attachedLogCatalog
	o.busCtx.AttachedHitrace = o.attachedHitrace
	o.busCtx.AttachedTraceMaterial = o.attachedTraceMaterial
	o.busCtx.AttachedHitraceSource = o.attachedHitraceSource
}
