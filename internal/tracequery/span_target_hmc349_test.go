package tracequery

import "testing"

func TestHMC349SharedSpanTargetMembershipKeepsEngineSemantics(t *testing.T) {
	span := TraceSpanSummary{Thread: ThreadRef{PID: 7, TGID: 100, Comm: "worker"}, SpanPID: 200, Name: "arbitrary work", StartTs: 1, EndTs: 2}
	for _, tc := range []struct {
		name   string
		target ThreadRef
		query  Query
		want   bool
	}{
		{"broad", ThreadRef{}, Query{}, true},
		{"exact_tid", ThreadRef{PID: 7}, Query{PID: 7}, true},
		{"tgid_is_not_tid", ThreadRef{PID: 100}, Query{PID: 100}, false},
		{"marker_pid_is_not_tid", ThreadRef{PID: 200}, Query{PID: 200}, false},
		{"process_tgid", ThreadRef{}, Query{PID: 100, TargetScope: TargetScopeProcess}, true},
		{"process_marker_namespace", ThreadRef{}, Query{PID: 200, TargetScope: TargetScopeProcess}, true},
		{"tid_is_not_process", ThreadRef{}, Query{PID: 7, TargetScope: TargetScopeProcess}, false},
		{"process_missing", ThreadRef{}, Query{TargetScope: TargetScopeProcess}, false},
		{"unknown_tid", ThreadRef{PID: 99}, Query{PID: 99}, false},
		{"engine_exact_name_unchanged", ThreadRef{Comm: "worker"}, Query{Thread: "worker"}, true},
		{"partial_name_is_not_identity", ThreadRef{Comm: "work"}, Query{Thread: "work"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := TraceSpanMatchesTarget(span, tc.target, tc.query); got != tc.want {
				t.Fatalf("shared membership=%v want %v", got, tc.want)
			}
			if got := traceSpanMatchesQuery(span, tc.target, tc.query); got != tc.want {
				t.Fatal("native query target rule changed")
			}
		})
	}
	q := Query{PID: 7, SpanName: "not this business", TimeStart: 3, TimeEnd: 4}
	if !TraceSpanMatchesTarget(span, ThreadRef{PID: 7}, q) || traceSpanMatchesQuery(span, ThreadRef{PID: 7}, q) {
		t.Fatal("identity matching absorbed window/name filters")
	}
}
