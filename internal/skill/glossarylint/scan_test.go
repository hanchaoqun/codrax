package glossarylint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMatch pins the one matcher every lint shares: short uppercase
// acronyms are whole-word, everything else is a case-sensitive
// substring (so the hyphenated codename family matches inside its
// shipped label form).
func TestMatch(t *testing.T) {
	cases := []struct {
		body, term string
		want       bool
	}{
		{"the TERMINAL state", "ERM", false},
		{"see ERM rule", "ERM", true},
		{"ERM", "ERM", true},
		{"LOW-MIND RULE: when the CURRENT request", "LOW-MIND", true},
		{"Low-mind precedence shortcut", "Low-mind", true},
		{"Preferred low-mind row contract", "low-mind", true},
		{"lowmind", "low-mind", false},
		{"AnswerDocumentV2Patch", "AnswerDocumentV2", true},
		{"", "AnalysisIR", false},
		{"AnalysisIR", "", false},
	}
	for _, c := range cases {
		got := Match(c.body, c.term) >= 0
		if got != c.want {
			t.Errorf("Match(%q, %q) = %v, want %v", c.body, c.term, got, c.want)
		}
	}
}

// TestScanTextWith_ExtrasUseSameMatcher pins that per-surface extras
// are matched by the same rule as glossary entries.
func TestScanTextWith_ExtrasUseSameMatcher(t *testing.T) {
	hits := ScanTextWith("x", "the finalizer uses ctx.Mutable here", "ctx.Mutable")
	var terms []string
	for _, h := range hits {
		terms = append(terms, h.Term)
	}
	if strings.Join(terms, ",") != "finalizer,ctx.Mutable" {
		t.Fatalf("expected glossary hit then extra hit, got %v", terms)
	}
	if got := ScanText("x", "plain prose"); len(got) != 0 {
		t.Fatalf("clean text must not hit: %v", got)
	}
}

// writeScratchPackage lays down a synthetic package so each structural
// exclusion is proved by a self-red: the injected token is found at the
// exact file:line unless the policy position excludes it.
func writeScratchPackage(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func hitLabels(hits []Hit) []string {
	var out []string
	for _, h := range hits {
		out = append(out, filepath.Base(h.Label)+"/"+h.Term)
	}
	return out
}

// TestScanGoDir_SelfRedPerShape proves every structural exclusion and
// every scanned position on a synthetic package. EVOLUTION RECORD
// (§40.52 fold-in, G6-jargon #0): the original case pinned only
// `fmt.Fprintf(os.Stderr, …)` as excluded, so the scanner's unconditional
// fmt.Fprint* exclusion — which also hid builder-directed
// `fmt.Fprintf(&b, …)` prompt writes — could never go red. The
// fmt.Fprint* lane is now keyed on the writer operand: host streams
// (os.Stdout / os.Stderr / io.Discard / a logger's Writer(), directly or
// through a single-assignment name) stay excluded; a strings.Builder,
// an io.Writer parameter and a field writer are scanned.
func TestScanGoDir_SelfRedPerShape(t *testing.T) {
	dir := writeScratchPackage(t, map[string]string{
		"a.go": `package scratch

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"github.com/hanchaoqun/codrax/internal/logging"
)

type row struct {
	F string ` + "`json:\"AnalysisIR\"`" + `
}

const enumValue = "finalizer_only"
const prose = "the TaskGraph is ready"

var auditOut = os.Stdout

func f(x string) string {
	logging.Debug("[CGEC] excluded %s", x)
	log.Printf("BusContext excluded")
	fmt.Fprintf(os.Stderr, "MutableState excluded")
	var s struct{ LastError string }
	s.LastError = "EvidencePlan excluded by policy"
	return "hit: RequestModel" + fmt.Sprintf("hit: %s", "AnswerContract")
}

func g() error { return fmt.Errorf("HypothesisSet stays in scope") }

type sink struct{ out io.Writer }

func h(w io.Writer, k *sink) string {
	var b strings.Builder
	fmt.Fprintf(&b, "hit: %s in EvidenceClosure", "x")
	fmt.Fprintln(w, "hit: ReadSet through an io.Writer parameter")
	fmt.Fprint(k.out, "hit: PendingReads through a field writer")
	errOut := os.Stderr
	fmt.Fprintf(errOut, "GroundingStatus excluded through a stream-bound name")
	fmt.Fprintf(auditOut, "AnchorKind excluded through a package-level stream name")
	fmt.Fprintf(log.Writer(), "RiskMatrix excluded through the logger writer")
	fmt.Fprintln(io.Discard, "QualityGate excluded through io.Discard")
	return b.String()
}
`,
		"a_test.go": `package scratch

const ignored = "AnalysisIR in a test file is never scanned"
`,
	})

	narrow, err := ScanGoDir(dir, Policy{})
	if err != nil {
		t.Fatalf("narrow scan: %v", err)
	}
	wantNarrow := []string{
		"a.go:16/finalizer",       // const enum value: in scope without SkipConstRHS
		"a.go:17/TaskGraph",       // const prose
		"a.go:26/EvidencePlan",    // *Error target: in scope without SkipErrorTargets
		"a.go:27/RequestModel",    // plain literal
		"a.go:27/AnswerContract",  // fmt.Sprintf argument is NOT a logger
		"a.go:30/HypothesisSet",   // fmt.Errorf argument is NOT a logger
		"a.go:36/EvidenceClosure", // fmt.Fprintf into a strings.Builder is a prompt write
		"a.go:37/ReadSet",         // fmt.Fprintln into an io.Writer parameter is scanned
		"a.go:38/PendingReads",    // fmt.Fprint into a field writer is scanned
	}
	if got := strings.Join(hitLabels(narrow), " "); got != strings.Join(wantNarrow, " ") {
		t.Fatalf("narrow policy hits = %v\nwant %v", hitLabels(narrow), wantNarrow)
	}

	wide, err := ScanGoDir(dir, Policy{SkipConstRHS: true, SkipErrorTargets: true})
	if err != nil {
		t.Fatalf("wide scan: %v", err)
	}
	wantWide := []string{
		"a.go:27/RequestModel", "a.go:27/AnswerContract", "a.go:30/HypothesisSet",
		"a.go:36/EvidenceClosure", "a.go:37/ReadSet", "a.go:38/PendingReads",
	}
	if got := strings.Join(hitLabels(wide), " "); got != strings.Join(wantWide, " ") {
		t.Fatalf("wide policy hits = %v\nwant %v", hitLabels(wide), wantWide)
	}
}

func TestScanGoDir_ExemptionRowsAreTyped(t *testing.T) {
	dir := writeScratchPackage(t, map[string]string{
		"table.go": "package scratch\n\nvar paths = []string{\"cmd/root.go\"}\n",
		"other.go": "package scratch\n\nvar x = \"clean\"\n",
	})
	if hits, err := ScanGoDir(dir, Policy{}); err != nil || len(hits) != 1 {
		t.Fatalf("expected exactly one data-table hit before exemption, got %v / %v", hits, err)
	}
	hits, err := ScanGoDir(dir, Policy{Exempt: []Exemption{{File: "table.go", Reason: ExemptDataTable}}})
	if err != nil || len(hits) != 0 {
		t.Fatalf("typed exemption must silence the data table: %v / %v", hits, err)
	}
	if _, err := ScanGoDir(dir, Policy{Exempt: []Exemption{{File: "gone.go", Reason: ExemptDataTable}}}); err == nil || !strings.Contains(err.Error(), "stale exemption row") {
		t.Fatalf("stale exemption row must fail loud, got %v", err)
	}
	if _, err := ScanGoDir(dir, Policy{Exempt: []Exemption{{File: "table.go", Reason: "because"}}}); err == nil || !strings.Contains(err.Error(), "outside the closed set") {
		t.Fatalf("open-set reason must fail loud, got %v", err)
	}
	if _, err := ScanGoDir(t.TempDir(), Policy{}); err == nil {
		t.Fatalf("a directory without non-test .go files must fail loud")
	}
}

func TestRunPackageScan_ReportsEveryHitThenFails(t *testing.T) {
	dir := writeScratchPackage(t, map[string]string{
		"a.go": "package scratch\n\nvar a = \"TaskGraph\"\nvar b = \"EvidencePlan\"\n",
	})
	rec := &recordingTB{TB: t}
	func() {
		defer func() { _ = recover() }()
		RunPackageScan(rec, dir, Policy{})
	}()
	if rec.errors != 2 || !rec.fatal {
		t.Fatalf("expected 2 Errorf lines then Fatalf, got errors=%d fatal=%v", rec.errors, rec.fatal)
	}
}

// recordingTB counts Errorf/Fatalf calls so the reporting contract
// (every hit listed, then one summary Fatalf) can be pinned.
type recordingTB struct {
	testing.TB
	errors int
	fatal  bool
}

func (r *recordingTB) Helper()                           {}
func (r *recordingTB) Errorf(string, ...any)             { r.errors++ }
func (r *recordingTB) Fatalf(format string, args ...any) { r.fatal = true; panic("fatal") }
