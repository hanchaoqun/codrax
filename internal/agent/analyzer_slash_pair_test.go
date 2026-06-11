package agent

import (
	"reflect"
	"testing"
)

// Slash-pair completion pins: the forensic shape plus every guard.
func TestCompleteSlashPairEntities(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		entities []string
		want     []string
	}{
		{
			name:     "forensic shape — missing half completed",
			raw:      "主要状态载体（例如 AnalysisIR、EvidenceItems、AnswerDocument、Mutable/BusContext）",
			entities: []string{"AnalysisIR", "BusContext"},
			want:     []string{"AnalysisIR", "BusContext", "Mutable"},
		},
		{
			name:     "reverse side anchored",
			raw:      "explain Mutable/BusContext handoff",
			entities: []string{"Mutable"},
			want:     []string{"Mutable", "BusContext"},
		},
		{
			name:     "both present — no duplicate",
			raw:      "Mutable/BusContext",
			entities: []string{"Mutable", "BusContext"},
			want:     []string{"Mutable", "BusContext"},
		},
		{
			name:     "both absent — nothing anchors the pair",
			raw:      "Mutable/BusContext",
			entities: []string{"AnalysisIR"},
			want:     []string{"AnalysisIR"},
		},
		{
			name:     "path segments never qualify",
			raw:      "look under internal/agent for the file",
			entities: []string{"internal"},
			want:     []string{"internal"},
		},
		{
			name:     "multi-segment path out of scope",
			raw:      "Read internal/agent/Analyzer for details",
			entities: []string{"Analyzer"},
			want:     []string{"Analyzer"},
		},
		{
			name:     "dotted file out of scope",
			raw:      "open pkg.go/Helper now",
			entities: []string{"Helper"},
			want:     []string{"Helper"},
		},
		{
			name:     "snake_case half qualifies",
			raw:      "check retry_budget/cap_limit pair",
			entities: []string{"cap_limit"},
			want:     []string{"cap_limit", "retry_budget"},
		},
		{
			name:     "no slash fast path",
			raw:      "plain question",
			entities: []string{"AnalysisIR"},
			want:     []string{"AnalysisIR"},
		},
	}
	for _, tc := range cases {
		got := completeSlashPairEntities(tc.raw, tc.entities)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
