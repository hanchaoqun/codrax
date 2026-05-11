package types

import "strings"

// ObservationKind is the typed string enum describing one normalized
// observation channel that fed an ArtifactObservationProfile. Producers
// MUST use these constants (or the constructors below) instead of
// literal strings so a typo is a compile error and downstream routing
// switches can rely on a closed-or-prefixed set.
//
// Two encoding tiers:
//
//   - Fixed kinds — exact constant values produced by request
//     diagnostic intent, log-bundle direct evidence (error_type /
//     error_message), and perf-bundle direct evidence (perf_frame /
//     perf_jank / perf_stall / perf_startup).
//   - Prefixed kinds — produced by MakeLogSignalObservationKind and
//     MakePerfSignalObservationKind. Carry the underlying signal name
//     after a `signal:` / `perf_signal:` prefix. The log-signal lane
//     is itself a typed LogSignal enum; the perf-signal lane is still
//     a free-form []string at source (PerfBundle.Meta.Signals).
//   - Bundle observation kinds — produced by
//     ObservationKindFromLogObservation, which lifts the typed
//     LogObservationKind enum into ObservationKind so the two lists
//     can share routing.
//
// Wire format is preserved: ObservationKind is a string subtype, so
// JSON marshaling produces the same flat array as the previous
// []string field.
type ObservationKind string

const (
	// Diagnostic intent derived from RequestModel.DiagnosticProfile.
	ObservationKindRequestDiagnostic         ObservationKind = "request_diagnostic"
	ObservationKindDiagnosticQuestion        ObservationKind = "diagnostic_question"
	ObservationKindCurrentRiskCheck          ObservationKind = "current_risk_check"
	ObservationKindHistoricalRegressionCheck ObservationKind = "historical_regression_check"
	ObservationKindCurrentVersionCheck       ObservationKind = "current_version_check"

	// Log bundle direct evidence.
	ObservationKindErrorType    ObservationKind = "error_type"
	ObservationKindErrorMessage ObservationKind = "error_message"

	// Perf bundle direct evidence.
	ObservationKindPerfFrame   ObservationKind = "perf_frame"
	ObservationKindPerfJank    ObservationKind = "perf_jank"
	ObservationKindPerfStall   ObservationKind = "perf_stall"
	ObservationKindPerfStartup ObservationKind = "perf_startup"
)

const (
	observationKindLogSignalPrefix  = "signal:"
	observationKindPerfSignalPrefix = "perf_signal:"
)

// MakeLogSignalObservationKind tags a typed LogSignal as an
// observation kind. Empty / whitespace-only signals return "".
func MakeLogSignalObservationKind(s LogSignal) ObservationKind {
	trimmed := strings.TrimSpace(string(s))
	if trimmed == "" {
		return ""
	}
	return ObservationKind(observationKindLogSignalPrefix + trimmed)
}

// MakePerfSignalObservationKind tags a perf-bundle signal name as an
// observation kind. PerfBundle stores raw []string signals (no typed
// enum upstream), so this helper trims + namespaces.
func MakePerfSignalObservationKind(signal string) ObservationKind {
	signal = strings.TrimSpace(signal)
	if signal == "" {
		return ""
	}
	return ObservationKind(observationKindPerfSignalPrefix + signal)
}

// ObservationKindFromLogObservation lifts a typed LogObservationKind
// into ObservationKind. Empty input returns "".
func ObservationKindFromLogObservation(k LogObservationKind) ObservationKind {
	trimmed := strings.TrimSpace(string(k))
	if trimmed == "" {
		return ""
	}
	return ObservationKind(trimmed)
}

// ObservationKindStrings flattens ObservationKind slices to plain
// strings for rendering (strings.Join, prompt prose, etc.).
func ObservationKindStrings(in []ObservationKind) []string {
	out := make([]string, 0, len(in))
	for _, k := range in {
		s := strings.TrimSpace(string(k))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// ObservationKindsContain reports whether want appears in haystack.
// Used by tests; production code routes via switch on ObservationKind.
func ObservationKindsContain(haystack []ObservationKind, want ObservationKind) bool {
	for _, k := range haystack {
		if k == want {
			return true
		}
	}
	return false
}

func dedupeObservationKinds(in []ObservationKind) []ObservationKind {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[ObservationKind]bool, len(in))
	out := make([]ObservationKind, 0, len(in))
	for _, k := range in {
		s := ObservationKind(strings.TrimSpace(string(k)))
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
