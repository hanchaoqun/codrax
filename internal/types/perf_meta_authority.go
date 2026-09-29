package types

// HasAuthoritativeMeta separates schema-valid model estimates from measured
// metadata. BugClasses have independent registry provenance and are unaffected.
func (b *PerfBundle) HasAuthoritativeMeta() bool {
	return b != nil && b.Meta.Authority == PerfObservationAuthorityDeterministicValidator
}

// AuthoritativeSignals never turns free-form pre-triage labels into evidence.
// Independently verified records still contribute their own semantics even if
// their containing metadata was model-extracted or came from a legacy bundle.
func (b *PerfBundle) AuthoritativeSignals() []string {
	if b == nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	if b.HasAuthoritativeMeta() {
		for _, s := range b.Meta.Signals {
			add(s)
		}
	}
	if b.HasAuthoritativeJankVerdict() {
		add("jank")
	}
	for _, s := range b.Stalls {
		if !s.IsNavigationOnly() && s.DurationMs >= PerfMainThreadStallMs {
			add("main-thread-stall")
		}
	}
	if b.HasAuthoritativeStartup() && b.Startup.Mode == "cold" && b.Startup.AppLaunchMs > PerfStartupSlowColdMs {
		add("cold-start-slow")
	}
	return out
}
