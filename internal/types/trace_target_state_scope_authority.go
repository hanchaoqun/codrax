package types

import "strings"

// TraceTargetStateScopeAuthority is the wording boundary carried by one
// compiled target_window_states account. Every duration is scoped to the
// target thread's own wall-clock state partition; none is a CPU-wide
// utilization or saturation measurement.
type TraceTargetStateScopeAuthority struct {
	ArtifactLabel string
	Subject       string
	WindowStartTs float64
	WindowEndTs   float64
	RunningMS     float64
	RunnableMS    float64
	SleepMS       float64
	DStateMS      float64
	TotalMS       float64
	EvidenceID    string
}

// BuildTraceTargetStateScopeAuthorities compiles the target-thread scope
// authorities from the already-selected projection accounts. It deliberately
// consumes the compiled projection rather than all raw target-state records so
// explicit-window election and supplemental-window separation remain owned by
// the existing projection compiler.
func BuildTraceTargetStateScopeAuthorities(set TraceCausalProjectionSet) []TraceTargetStateScopeAuthority {
	out := make([]TraceTargetStateScopeAuthority, 0, len(set.Projections))
	seen := map[string]bool{}
	for _, projection := range set.Projections {
		account := projection.TargetStateAccount
		if account == nil || strings.TrimSpace(account.Subject) == "" || account.TotalMS <= 0 {
			continue
		}
		key := strings.Join([]string{
			strings.TrimSpace(projection.ArtifactPath),
			strings.TrimSpace(account.Subject),
			strings.TrimSpace(account.EvidenceID),
		}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, TraceTargetStateScopeAuthority{
			ArtifactLabel: strings.TrimSpace(projection.ArtifactLabel),
			Subject:       strings.TrimSpace(account.Subject),
			WindowStartTs: account.WindowStartTs,
			WindowEndTs:   account.WindowEndTs,
			RunningMS:     account.RunningMS,
			RunnableMS:    account.RunnableMS,
			SleepMS:       account.SleepMS,
			DStateMS:      account.DStateMS,
			TotalMS:       account.TotalMS,
			EvidenceID:    strings.TrimSpace(account.EvidenceID),
		})
	}
	return out
}
