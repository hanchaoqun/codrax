package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestDetectStallAndAct_NoFingerprintHistory_NoStall: a single
// fingerprint cannot be a stall. Returns false, no repair queued.
func TestDetectStallAndAct_NoFingerprintHistory_NoStall(t *testing.T) {
	o := newTestOrch(t)
	if o.detectStallAndAct() {
		t.Errorf("first call must not detect stall")
	}
	if reps := o.busCtx.Mutable.EvidenceClosure().PendingRepairs(); len(reps) > 0 {
		t.Errorf("expected no repairs queued, got %v", reps)
	}
}

// TestDetectStallAndAct_TwoIdenticalFingerprints_DoesNotForceComplete:
// two consecutive identical fingerprints triggers forced-read attempt
// but does NOT force-complete. The flag stays false.
func TestDetectStallAndAct_TwoIdenticalFingerprints_DoesNotForceComplete(t *testing.T) {
	o := newTestOrch(t)
	// Two identical rounds (no read, no evidence, no chains).
	o.detectStallAndAct()
	o.detectStallAndAct()
	if o.busCtx.Mutable.IsInvestigationComplete() {
		t.Errorf("two-fingerprint stall must not force-complete")
	}
}

// TestDetectStallAndAct_ThreeIdenticalFingerprints_HardStall:
// three consecutive identical fingerprints triggers hard stall →
// investigationComplete is forced and a force_complete_downgrade
// repair is queued.
func TestDetectStallAndAct_ThreeIdenticalFingerprints_HardStall(t *testing.T) {
	o := newTestOrch(t)
	o.detectStallAndAct()
	o.detectStallAndAct()
	hard := o.detectStallAndAct()
	if !hard {
		t.Errorf("third identical fingerprint should signal hard stall")
	}
	if !o.busCtx.Mutable.IsInvestigationComplete() {
		t.Errorf("hard stall must force-complete the investigation")
	}
	var found bool
	for _, r := range o.busCtx.Mutable.EvidenceClosure().PendingRepairs() {
		if r.Kind == types.RepairForceCompleteDowngrade {
			found = true
		}
	}
	if !found {
		t.Errorf("hard stall must queue RepairForceCompleteDowngrade repair")
	}
}

// TestDetectStallAndAct_ProgressBetweenRounds_NoStall: when a new
// piece of evidence is emitted between rounds, the fingerprint
// changes and no stall fires.
func TestDetectStallAndAct_ProgressBetweenRounds_NoStall(t *testing.T) {
	o := newTestOrch(t)
	o.detectStallAndAct()
	// Simulate progress: emit one new evidence item.
	o.busCtx.Mutable.AppendEvidence([]types.EvidenceItem{
		{
			ID:        types.StableEvidenceID(types.EvidenceConcrete, "foo", "p", "v", "", "f", 1, 1),
			Source:    "f",
			LineStart: 1,
		},
	})
	if o.detectStallAndAct() {
		t.Errorf("progress between rounds must not be a stall")
	}
	if o.busCtx.Mutable.IsInvestigationComplete() {
		t.Errorf("progress must not force-complete")
	}
}

// TestRunForcedReads_NoPending_Noop: with an empty PendingReads
// queue, runForcedReads returns 0 and changes nothing.
func TestRunForcedReads_NoPending_Noop(t *testing.T) {
	o := newTestOrch(t)
	if got := o.runForcedReads(); got != 0 {
		t.Errorf("expected 0 forced reads, got %d", got)
	}
}

// TestRunForcedReads_BudgetCap: even with many PendingReads, the
// per-round cap (cgecForcedReadsPerRound) is respected.
func TestRunForcedReads_BudgetCap(t *testing.T) {
	o := newTestOrch(t)
	closure := o.busCtx.Mutable.EvidenceClosure()
	// Queue more than the cap. Use a path that DOES NOT exist on
	// disk so the read fails — we just want to confirm the cap is
	// applied (the loop attempts only N before stopping).
	for i := 0; i < cgecForcedReadsPerRound+5; i++ {
		closure.AddPendingRead(types.PendingRead{
			File:      string(rune('a'+i)) + "/missing.go",
			Rationale: "test",
			Origin:    "test",
		})
	}
	// Calling runForcedReads with no real files just returns 0
	// successful reads. The important assertion is that PendingReads
	// is NOT entirely emptied — over-the-cap entries remain.
	o.runForcedReads()
	remaining := closure.PendingReads()
	if len(remaining) < 5 {
		t.Errorf("over-cap entries should remain unread; got %d remaining (started with %d)",
			len(remaining), cgecForcedReadsPerRound+5)
	}
}

// TestRunForcedReads_HonorsLineRanges_Surgical pins the load-bearing
// surgical-recovery contract: when a PendingRead carries non-empty
// LineRanges (populated by multipath.EvaluateAnchor's
// ActionDemandSurgicalRead path), runForcedReads must issue ONE
// read_file call per range with offset/limit instead of paginating
// the whole file. Without this, recovery on a 4 000-line file with
// three 30-line missing slivers would pull in the whole file —
// defeating the surgical-no-noise rewrite.
func TestRunForcedReads_HonorsLineRanges_Surgical(t *testing.T) {
	o := newTestOrch(t)
	// Materialise a real file in the test repo with enough lines to
	// distinguish surgical reads from full-file reads.
	repoRoot := o.busCtx.RepoRoot
	relPath := "internal/repl/repl.go"
	abs := filepath.Join(repoRoot, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var content strings.Builder
	for i := 1; i <= 500; i++ {
		// Each line is "L<n>" so the rendered banner content is
		// distinct per line and we can verify which lines were read.
		content.WriteString("L")
		content.WriteString(testIntToString(i))
		content.WriteString("\n")
	}
	if err := os.WriteFile(abs, []byte(content.String()), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	closure := o.busCtx.Mutable.EvidenceClosure()
	closure.AddPendingRead(types.PendingRead{
		File:      relPath,
		Rationale: "surgical demand from multi-path anchor gate",
		Origin:    "pre_complete.multi_path_anchor",
		LineRanges: []types.LineRange{
			{Start: 100, End: 130},
			{Start: 200, End: 230},
		},
	})

	got := o.runForcedReads()
	if got != 1 {
		t.Fatalf("expected 1 successful PendingRead drain, got %d", got)
	}
	// Two surgical reads must have landed in DispatchToolResults
	// (one per range), each with the [forced_read surgical] tag and
	// a banner reflecting the requested offset/limit.
	results := o.busCtx.Mutable.DispatchToolResults()
	if len(results) != 2 {
		t.Fatalf("expected 2 surgical ToolResults (one per LineRange), got %d", len(results))
	}
	for i, r := range results {
		if !strings.HasPrefix(r.Summary, "[forced_read surgical] ") {
			t.Errorf("result[%d] missing surgical tag: %q", i, r.Summary[:testMin(80, len(r.Summary))])
		}
	}
	// First range was [100, 130] → banner must say "showing lines 100-130".
	if !strings.Contains(results[0].Summary, "showing lines 100-130") {
		t.Errorf("result[0] banner does not match first surgical range; got: %s", firstLineOf(results[0].Summary))
	}
	if !strings.Contains(results[1].Summary, "showing lines 200-230") {
		t.Errorf("result[1] banner does not match second surgical range; got: %s", firstLineOf(results[1].Summary))
	}
}

// TestRunForcedReads_FullFileFallback_NoLineRanges pins the
// backward-compatibility contract: PendingReads emitted by older
// gates (chain promotion, grounder reject, phase1_unread,
// primary_anchor) leave LineRanges nil, and runForcedReads must
// continue to do a full-file read so those legacy paths keep
// working byte-identically.
func TestRunForcedReads_FullFileFallback_NoLineRanges(t *testing.T) {
	o := newTestOrch(t)
	repoRoot := o.busCtx.RepoRoot
	relPath := "small.txt"
	if err := os.WriteFile(filepath.Join(repoRoot, relPath), []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	closure := o.busCtx.Mutable.EvidenceClosure()
	closure.AddPendingRead(types.PendingRead{
		File:      relPath,
		Rationale: "legacy primary-anchor demand",
		Origin:    "pre_complete.primary_anchor",
		// LineRanges intentionally nil — old callers do not set it.
	})

	if got := o.runForcedReads(); got != 1 {
		t.Fatalf("expected 1 successful drain, got %d", got)
	}
	results := o.busCtx.Mutable.DispatchToolResults()
	if len(results) != 1 {
		t.Fatalf("expected 1 ToolResult (full-file read), got %d", len(results))
	}
	if !strings.HasPrefix(results[0].Summary, "[forced_read] ") {
		t.Errorf("legacy fallback must use the un-suffixed [forced_read] tag, got: %q",
			results[0].Summary[:testMin(80, len(results[0].Summary))])
	}
	if strings.Contains(results[0].Summary, "[forced_read surgical]") {
		t.Errorf("legacy nil-LineRanges path must NOT use the surgical tag")
	}
}

// testIntToString is a no-allocation int-to-string helper used by
// the test content generator above. Renamed from `itoa` to avoid
// colliding with the package-level helper of the same name in
// cancel.go.
func testIntToString(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// firstLineOf returns the first newline-delimited line of s, used in
// banner-assertion failure messages so test output stays compact.
func firstLineOf(s string) string {
	if idx := strings.Index(s, "\n"); idx >= 0 {
		return s[:idx]
	}
	return s
}

// testMin avoids the package-level `min` helper in plan_mode_e2e_test.go.
func testMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// newTestOrch builds a minimal Orchestrator instance with a freshly-
// initialized BusContext + MutableState. Sufficient for the CGEC
// enforcer unit tests; does NOT wire the full pipeline.
func newTestOrch(t *testing.T) *Orchestrator {
	t.Helper()
	mut := types.NewMutableState("test objective")
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
	}
	return &Orchestrator{busCtx: bus, emit: render.NopEmitter}
}
