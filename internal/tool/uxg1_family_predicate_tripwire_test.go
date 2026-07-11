package tool

// uxg1_family_predicate_tripwire_test.go — UXG-1 M4 (ledger §29.38 收敛机制
// "family 谓词强制"; matrix spec §④): source-scan tripwire against LOCAL
// re-enumeration of two adjudicated token families —
//
//   - the priority-inversion row-type family
//     {priority_inversion_candidate, priority_inversion_runnable_wait}
//     (single points: tracequery rootCauseTypeIsPriorityInversion, display
//     runtimeTracePriorityInversionCandidateType — interlocked below);
//   - the cross-thread aggregate row family (registry-derived: RowToken ∧
//     Additivity==cross_thread_cpu_ms; single points:
//     rootCauseAggregateMetricTypes ↔ runtimeTraceProjCrossThreadAggregateType,
//     already interlocked by semantic_ruling_pins_test.go).
//
// Mechanics: scan every non-test .go file of the engine/display packages,
// collect quoted family-token occurrences (comment lines dropped), and merge
// occurrences ≤3 lines apart into clusters. A cluster naming ≥2 DISTINCT
// family members is a family enumeration site. Every site must be declared
// in the allowlist below WITH its justification; a NEW site (someone
// hand-spelling the family instead of calling the predicate) reddens, and a
// REMOVED site reddens too (ghost allowlist rows die loudly — the allowlist
// is a commitment surface).
//
// Precision note: the scan reads quoted literals only, so prompt prose and
// comments never trip; LLM-facing packages (internal/skill, internal/agent)
// are deliberately out of scope (prompt files are zero-touch for UXG-1).

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

var uxg1ScanDirs = []string{".", "../tracequery", "../types", "../preview", "../render"}

// uxg1InversionFamily — the two inversion row-type tokens (spelled here ONCE,
// in test data, as the scan needle).
var uxg1InversionFamily = []string{"priority_inversion_candidate", "priority_inversion_runnable_wait"}

// uxg1AggregateFamily derives the cross-thread aggregate row family from the
// causal-token registry (the single source of truth for token lanes).
func uxg1AggregateFamily() []string {
	var out []string
	for _, token := range tracequery.CausalTokenUniverse() {
		spec, _ := tracequery.CausalTokenSpecFor(token)
		if spec.RowToken && spec.Additivity == tracequery.CausalAdditivityCrossThreadCPUms {
			out = append(out, token)
		}
	}
	sort.Strings(out)
	return out
}

// Allowlists: file base name → expected family-enumeration cluster count.
// EVERY entry documents why the site is not a family-predicate bypass.
// F4 counting blind spot (documented): a same-file swap — retiring one
// allowed cluster while adding one new re-enumeration — keeps the count
// unchanged and is not caught (漏报可能,报红必真); the family predicates'
// interlock pins are the second net.
var uxg1InversionAllowedClusters = map[string]int{
	// Registry rows: per-token lane declarations (one token per row; adjacency
	// is alphabetical listing, not enumeration-for-treatment).
	"causal_token_registry.go": 1,
	// THE engine family single point rootCauseTypeIsPriorityInversion.
	"query.go": 1,
	// Typed recon rule "causal_inversion_window": the two tokens play
	// DIFFERENT roles (absorber vs absorbed) — a per-token rule table, not
	// same-treatment enumeration.
	"rank_cross_type_recon.go": 1,
	// THE display family single point runtimeTracePriorityInversionCandidateType.
	"answer_document_mutation_runtime.go": 1,
	// Per-token zh label table (distinct label per token — cannot collapse
	// into a predicate).
	"answer_document_mutation_runtime_typelabels.go": 1,
}

var uxg1AggregateAllowedClusters = map[string]int{
	// Registry rows (per-token lane declarations; three alphabetical
	// adjacency clusters).
	"causal_token_registry.go": 3,
	// query.go, four clusters: ① compute-supply verdict-enum collision
	// ("cpu_pressure" as a ComputeSupply.Verdict value beside the
	// "compute_supply" type mint — different lane, token name collision);
	// ② rootCauseAggregateMetricTypes — THE engine consumer set (interlocked
	// with the display set by semantic_ruling_pins_test.go); ③ the
	// participation-caliber zero closed set (deliberately NOT the family:
	// irq_burst is excluded there, membership difference is load-bearing);
	// ④ rootCauseTypeWeight per-token weight rows (distinct values).
	"query.go": 4,
	// rcr.go, three clusters: the §24.3 impact-form token→family mapping —
	// itself a pinned single-source table (form words, not the aggregate
	// family).
	"answer_document_mutation_runtime_rcr.go": 3,
	// tree.go, three clusters: ① runtimeTraceProjCrossThreadAggregateType —
	// THE display consumer set; ② queue-depth density-wording sub-family
	// (cpu_pressure/supply_pressure); ③ interrupt concurrency-wording
	// sub-family (irq_burst/irq_activity/ipi_activity) — both typed wording
	// forks declared as their own closed sub-sets.
	"answer_document_mutation_runtime_tree.go": 3,
	// Per-token zh label table (distinct labels).
	"answer_document_mutation_runtime_typelabels.go": 1,
	// Per-token typed observation producer calls (each token feeds its own
	// stats slice).
	"trace_query.go": 1,
	// NKR registry rows: family COLUMN values ("supply_pressure",
	// "compute_supply" as note families), not row-token enumeration.
	"trace_note_keys.go": 1,
	// Resource-pressure PREDICATE closed set (includes softirq etc. — a
	// different, wider semantic set than the aggregate row family).
	"trace_observation_coverage.go": 1,
}

var uxg1QuotedToken = regexp.MustCompile(`"([a-z_]+)"`)

// uxg1StripComments removes Go line comments: whole-line comments vanish,
// trailing comments are cut at the first whitespace-preceded "//" (shared by
// the info-contract census scans).
func uxg1StripComments(src string) string {
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			lines[i] = ""
			continue
		}
		if at := strings.Index(line, "\t//"); at >= 0 {
			lines[i] = line[:at]
		} else if at := strings.Index(line, " //"); at >= 0 {
			lines[i] = line[:at]
		}
	}
	return strings.Join(lines, "\n")
}

// uxg1FamilyClusters returns the family-enumeration cluster count per file
// path for one family token set.
func uxg1FamilyClusters(t *testing.T, family []string) map[string]int {
	t.Helper()
	members := map[string]bool{}
	for _, token := range family {
		members[token] = true
	}
	counts := map[string]int{}
	for _, dir := range uxg1ScanDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(dir, name)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			type occurrence struct {
				line  int
				token string
			}
			var occ []occurrence
			for i, line := range strings.Split(string(raw), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "//") {
					continue
				}
				for _, match := range uxg1QuotedToken.FindAllStringSubmatch(line, -1) {
					if members[match[1]] {
						occ = append(occ, occurrence{i + 1, match[1]})
					}
				}
			}
			clusterTokens := map[string]bool{}
			flush := func() {
				if len(clusterTokens) >= 2 {
					counts[path]++
				}
				clusterTokens = map[string]bool{}
			}
			last := -10
			for _, o := range occ {
				if o.line-last > 3 {
					flush()
				}
				clusterTokens[o.token] = true
				last = o.line
			}
			flush()
		}
	}
	return counts
}

func uxg1AssertClusters(t *testing.T, familyName string, got map[string]int, allowed map[string]int) {
	t.Helper()
	seen := map[string]bool{}
	for path, count := range got {
		base := filepath.Base(path)
		seen[base] = true
		want, ok := allowed[base]
		if !ok {
			t.Errorf("%s family: NEW enumeration site in %s (%d cluster(s)) — call the family predicate instead of re-spelling the token set (UXG-1 M4)", familyName, path, count)
			continue
		}
		if count != want {
			t.Errorf("%s family: %s has %d enumeration cluster(s), allowlist says %d — a new local re-enumeration (or a stale allowlist row); reconcile with the family predicate first", familyName, path, count, want)
		}
	}
	for base := range allowed {
		if !seen[base] {
			t.Errorf("%s family: allowlist row %q is a ghost (no cluster found) — delete the row so the commitment surface stays honest", familyName, base)
		}
	}
}

func TestUXG1InversionFamilyNoLocalReEnumeration(t *testing.T) {
	uxg1AssertClusters(t, "inversion", uxg1FamilyClusters(t, uxg1InversionFamily), uxg1InversionAllowedClusters)
}

func TestUXG1CrossThreadAggregateFamilyNoLocalReEnumeration(t *testing.T) {
	family := uxg1AggregateFamily()
	if len(family) < 5 {
		t.Fatalf("registry-derived aggregate family suspiciously small: %v", family)
	}
	uxg1AssertClusters(t, "cross-thread-aggregate", uxg1FamilyClusters(t, family), uxg1AggregateAllowedClusters)
}

// TestUXG1InversionFamilyPredicatesInterlocked pins the engine and display
// single points to each other over the full registry universe plus
// non-members — the two-predicate topology can never fork silently.
func TestUXG1InversionFamilyPredicatesInterlocked(t *testing.T) {
	probes := append([]string{}, tracequery.CausalTokenUniverse()...)
	probes = append(probes, "blocking_span", "not_a_token", "")
	for _, token := range probes {
		if got, want := runtimeTracePriorityInversionCandidateType(token), tracequery.RootCauseTypeIsPriorityInversion(token); got != want {
			t.Errorf("inversion family fork on %q: display=%v engine=%v", token, got, want)
		}
	}
	if !tracequery.RootCauseTypeIsPriorityInversion("priority_inversion_candidate") ||
		!tracequery.RootCauseTypeIsPriorityInversion("priority_inversion_runnable_wait") {
		t.Fatalf("engine inversion family predicate lost a member")
	}
	if fmt.Sprintf("%v", uxg1InversionFamily) != "[priority_inversion_candidate priority_inversion_runnable_wait]" {
		t.Fatalf("scan needle drifted from the adjudicated family: %v", uxg1InversionFamily)
	}
}
