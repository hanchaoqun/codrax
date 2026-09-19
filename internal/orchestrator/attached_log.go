package orchestrator

// SetAttachedLog stores a runtime log excerpt (panic, exception stack,
// sanitizer diagnostic, traceback) that every subsequent Run() should
// attach to BusContext.AttachedLog so the log_triage pre-stage
// can extract stack-frame anchors.
//
// REPL sticky lifetime: the REPL's /log command sets this once and it
// persists across turns until the REPL's /log clear command passes
// an empty string to reset it. CLI single-shot mode calls it at most
// once with the --log / --log-text payload before the single Run().
// Empty string clears any previously attached log.
func (o *Orchestrator) SetAttachedLog(log string) {
	o.attachedLog = log
}
