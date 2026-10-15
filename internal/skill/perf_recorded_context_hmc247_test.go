package skill

import (
	"strings"
	"testing"
)

func TestHMC247PerfTeachingKeepsRecordedContextAndTypedExecutionBoundary(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("explore-skill")
	if err != nil {
		t.Fatal(err)
	}
	var body string
	for _, item := range sk.WorkflowTierB {
		if strings.HasPrefix(item.Body, "TRACE QUERY:") {
			body = item.Body
			if !item.ShouldRender(AppliesToContext{HasTrace: true}) || item.ShouldRender(AppliesToContext{}) {
				t.Fatal("sampling context teaching must retain the existing trace-only gate")
			}
		}
	}
	if body == "" {
		t.Fatal("registered explorer lost TRACE QUERY teaching")
	}
	for _, want := range []string{
		"recorded code-location and stack context",
		"sample_kind=unknown leaves execution state unestablished even when a CPU is recorded",
		"sample_kind=off_cpu records an off-CPU sample, not a measured blocking interval",
		"sample_cpu_scope=unknown or cpu_known=false leaves the CPU/core unavailable",
		"sample_kind=on_cpu supports only the recorded on-CPU sample points",
		"neither kind, CPU coordinate, nor event weight alone proves continuous execution or elapsed CPU time",
		"For independently established running or compute-supply causes",
		"same-source/thread/interval perf_contexts as code-location and stack support",
		"recorded symbols, modules, and stack paths within their own source/window/denominator",
		"on_chain_dependency",
		"scheduler interval overlap, chain relevance, binder peer state, D-state/IO, CPU/core/frequency/affinity, and supply pressure as the causal basis",
		"business_span_mention families (verbatim span + trio)",
		"positive CAP/compute-supply deficit and otherwise remains context_only",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("registered teaching missing %q", want)
		}
	}
	for _, retired := range []string{
		"Treat perf sample output as code-execution context",
		"explain what code consumed sampled CPU time",
		"top_symbols/top_dso/top_callchains explain what sampled threads were doing",
	} {
		if strings.Contains(body, retired) {
			t.Errorf("registered teaching still promotes generic sample records: %q", retired)
		}
	}
}
