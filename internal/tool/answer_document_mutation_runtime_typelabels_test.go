package tool

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
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
	}
	// Unmapped tokens must fall back to the raw form — never fabricate.
	if got := runtimeTraceCausalProjectionDisplayCauseName("runnable_delay", true); got != "runnable_delay" {
		t.Fatalf("unmapped token must render raw, got %q", got)
	}
	if got := runtimeTraceCausalProjectionNarrativeCauseName("runnable_delay", true); got != "runnable_delay" {
		t.Fatalf("unmapped narrative token must render raw, got %q", got)
	}
	// EN surfaces keep raw tokens on both lanes.
	if got := runtimeTraceCausalProjectionDisplayCauseName("io_wait", false); got != "io_wait" {
		t.Fatalf("EN tree lane must keep the raw token, got %q", got)
	}
	if got := runtimeTraceCausalProjectionNarrativeCauseName("io_wait", false); got != "io_wait" {
		t.Fatalf("EN narrative lane must keep the raw token, got %q", got)
	}
	// D4 combined format on the zh narrative lane.
	if got := runtimeTraceCausalProjectionNarrativeCauseName("priority_inversion_candidate", true); got != "优先级反转候选（priority_inversion_candidate）" {
		t.Fatalf("zh narrative lane must use the 中文（token） combined format, got %q", got)
	}
}
