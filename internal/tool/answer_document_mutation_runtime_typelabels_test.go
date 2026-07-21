package tool

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// §7.30.3 D2 anti-drift guard: every type token enumerated by the
// deterministic rootCauseTypeWeight ranking switch MUST have a concise zh tree
// label — a newly added root-cause type cannot silently render untranslated on
// the zh tree. The scan reads the tracequery source so the two case sets can
// never diverge without this test noticing.
func TestRootCauseTypeZHLabelCoversWeightUniverse(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "tracequery", "query.go"))
	if err != nil {
		t.Fatalf("read tracequery source: %v", err)
	}
	body := string(src)
	start := strings.Index(body, "func rootCauseTypeWeight(")
	if start < 0 {
		t.Fatalf("rootCauseTypeWeight not found in tracequery/query.go")
	}
	end := strings.Index(body[start:], "\n}")
	if end < 0 {
		t.Fatalf("rootCauseTypeWeight body end not found")
	}
	fn := body[start : start+end]
	caseLine := regexp.MustCompile(`(?m)^\s*case (.+):`)
	token := regexp.MustCompile(`"([^"]+)"`)
	found := 0
	for _, m := range caseLine.FindAllStringSubmatch(fn, -1) {
		for _, tok := range token.FindAllStringSubmatch(m[1], -1) {
			found++
			if runtimeTraceRootCauseTypeZHLabel(tok[1]) == "" {
				t.Errorf("rootCauseTypeWeight token %q has no zh tree label (D2) — add it to runtimeTraceRootCauseTypeZHLabel", tok[1])
			}
			// RULE3-1 件8 (§29.182②): the EN verdict-word table mirrors the
			// zh universe case-for-case — a token can never gain a zh word
			// while its EN face regresses to snake_case.
			if runtimeTraceRootCauseTypeENLabel(tok[1]) == "" {
				t.Errorf("rootCauseTypeWeight token %q has no EN verdict label (§29.182②) — add it to runtimeTraceRootCauseTypeENLabel", tok[1])
			}
		}
	}
	if found < 30 {
		t.Fatalf("weight-universe scan looks broken: only %d tokens extracted", found)
	}
	// Default-weight producers that never appear in the weight switch but do
	// publish tree rows.
	for _, tok := range []string{"io_latency", "sleep_wait", "ipi_activity"} {
		if runtimeTraceRootCauseTypeZHLabel(tok) == "" {
			t.Errorf("default-weight producer token %q has no zh tree label", tok)
		}
		if runtimeTraceRootCauseTypeENLabel(tok) == "" {
			t.Errorf("default-weight producer token %q has no EN verdict label (§29.182②)", tok)
		}
	}
	// Unmapped tokens must fall back to the raw form — never fabricate.
	if got := runtimeTraceCausalProjectionDisplayCauseName("runnable_delay", true); got != "runnable_delay" {
		t.Fatalf("unmapped token must render raw, got %q", got)
	}
	if got := runtimeTraceCausalProjectionNarrativeCauseName("runnable_delay", true); got != "runnable_delay" {
		t.Fatalf("unmapped narrative token must render raw, got %q", got)
	}
	// EVOLUTION RECORD (RULE3-1 件8, §29.182② verbatim 「②维持族判词 EN 化…
	// ◎/树判词面用词,wire token 留证据引用键位」, 2026-07-21): the former
	// 「EN surfaces keep raw tokens」 arms are RETIRED — the EN display lane
	// now consumes runtimeTraceRootCauseTypeENLabel (io_wait keeps its
	// identity word iowait), the EN narrative lane speaks the D4 combined
	// form, and UNMAPPED tokens still render raw (never fabricate).
	if got := runtimeTraceCausalProjectionDisplayCauseName("io_wait", false); got != "iowait" {
		t.Fatalf("EN tree lane must speak the mapped verdict word, got %q", got)
	}
	if got := runtimeTraceCausalProjectionDisplayCauseName("page_cache_churn", false); got != "page-cache churn" {
		t.Fatalf("EN tree lane must speak the §29.182② ruled word, got %q", got)
	}
	if got := runtimeTraceCausalProjectionNarrativeCauseName("priority_inversion_candidate", false); got != "priority inversion (candidate) (priority_inversion_candidate)" {
		t.Fatalf("EN narrative lane must use the label (token) combined format, got %q", got)
	}
	if got := runtimeTraceCausalProjectionDisplayCauseName("runnable_delay", false); got != "runnable_delay" {
		t.Fatalf("EN unmapped token must render raw, got %q", got)
	}
	// D4 combined format on the zh narrative lane.
	if got := runtimeTraceCausalProjectionNarrativeCauseName("priority_inversion_candidate", true); got != "优先级反转候选（priority_inversion_candidate）" {
		t.Fatalf("zh narrative lane must use the 中文（token） combined format, got %q", got)
	}
}

func TestSemanticOptimizationCustomerRuledLabels(t *testing.T) {
	for _, tc := range []struct {
		token  string
		want   string
		wantEN string
	}{
		{token: "texture_upload", want: "纹理上传", wantEN: "texture upload"},
		{token: "jit_compile", want: "JIT编译", wantEN: "JIT compilation"},
	} {
		if got := runtimeTraceRootCauseTypeZHLabel(tc.token); got != tc.want {
			t.Fatalf("ZH semantic label %s = %q, want %q", tc.token, got, tc.want)
		}
		if got := runtimeTraceCausalProjectionDisplayCauseName(tc.token, true); got != tc.want {
			t.Fatalf("ZH display label %s = %q, want %q", tc.token, got, tc.want)
		}
		// EVOLUTION RECORD (RULE3-1 件8, §29.182②, 2026-07-21): the EN
		// display lane speaks the reader word; the canonical token keeps its
		// audit seats on the detail 类型 column / wire keys only.
		if got := runtimeTraceCausalProjectionDisplayCauseName(tc.token, false); got != tc.wantEN {
			t.Fatalf("EN display lane %s = %q, want %q", tc.token, got, tc.wantEN)
		}
	}
}

func TestAggregatePressureDisplayNamesAreActionable(t *testing.T) {
	supply := types.TraceCausalProjectionNode{SubjectKind: types.TraceCausalSubjectKindAggregateMetric, Object: "supply_pressure"}
	if got := runtimeTraceCausalProjectionAggregateMetricName(supply, true); got != "调度压力(需求积压)·聚合" {
		t.Fatalf("supply aggregate must use the ruled demand-backlog label, got %q", got)
	}
	if got := runtimeTraceCausalProjectionAggregateMetricName(supply, false); got != "scheduling pressure (demand backlog) · aggregate" {
		t.Fatalf("EN supply aggregate label drifted: %q", got)
	}
}
