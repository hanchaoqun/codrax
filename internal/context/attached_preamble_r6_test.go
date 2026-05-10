// Package context — attached_preamble_r6_test.go (2026-05-10).
//
// R6 audit on the attachedLogPreamble / attachedTracePreamble
// strings introduced by b8e8490. These flow directly into the
// LLM-facing prompt as the lead-in to the raw runtime artifact
// section, so internal pipeline jargon (stage names, downstream
// architecture references) is forbidden.
//
// Forensic anchor: 2026-05-10 audit found
// "before downstream agents consume it" leaked into the perf-trace
// producer preamble; reworded to user-domain language. This pins
// the cleanliness so a future edit cannot regress.
package context

import (
	"strings"
	"testing"
)

func TestAttachedLogPreamble_R6(t *testing.T) {
	for _, state := range []attachedRuntimeTriageState{
		attachedTriageProducer,
		attachedTriageStructured,
		attachedTriageUnavailable,
	} {
		out := attachedLogPreamble(state)
		auditR6(t, "log/state="+humanState(state), out)
	}
}

func TestAttachedTracePreamble_R6(t *testing.T) {
	for _, state := range []attachedRuntimeTriageState{
		attachedTriageProducer,
		attachedTriageStructured,
		attachedTriageUnavailable,
	} {
		out := attachedTracePreamble(state)
		auditR6(t, "trace/state="+humanState(state), out)
	}
}

func auditR6(t *testing.T, label, out string) {
	t.Helper()
	for _, banned := range []string{
		// Pipeline stage names
		"explorer agent", "extractor agent", "finalizer agent", "analyzer agent",
		"log-triage producer", "perf-triage producer",
		"log_triage", "perf_triage", "pre-stage",
		// Pipeline architecture references that the LLM has no business knowing
		"downstream agents",
		"downstream stages",
		"downstream consumers",
		"downstream",
		"answer pipeline",
		// Internal Go type / package names
		"BusContext", "MutableState", "AnalysisIR",
		"AnalyzerHints", "AnswerDocumentV2",
		// Internal layer designators
		"L1 gate", "L2 gate", "L3 gate", "L4 gate",
	} {
		if strings.Contains(out, banned) {
			t.Errorf("[%s] R6 leak: preamble contains internal term %q: %q", label, banned, out)
		}
	}
}

func humanState(s attachedRuntimeTriageState) string {
	switch s {
	case attachedTriageProducer:
		return "producer"
	case attachedTriageStructured:
		return "structured"
	default:
		return "unavailable"
	}
}
