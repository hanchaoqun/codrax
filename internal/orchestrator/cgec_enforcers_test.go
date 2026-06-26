package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	progressItem := types.EvidenceItem{
		Kind:      types.EvidenceConcrete,
		Subject:   "foo",
		Predicate: "p",
		Object:    "v",
		Source:    "f",
		LineStart: 1,
		LineEnd:   1,
		Scope:     types.ScopeLine,
	}
	progressItem.ID = types.StableEvidenceID(progressItem)
	o.busCtx.Mutable.AppendEvidence([]types.EvidenceItem{progressItem})
	if o.detectStallAndAct() {
		t.Errorf("progress between rounds must not be a stall")
	}
	if o.busCtx.Mutable.IsInvestigationComplete() {
		t.Errorf("progress must not force-complete")
	}
}

func TestHashChainTerminals_UsesTypedChainPayloadNotSummary(t *testing.T) {
	base := []types.EvidenceItem{{
		Kind:      types.EvidenceDataflowPath,
		Predicate: "resolution_chain",
		Subject:   "`Register()` binds `NewFoo()` → `Foo.Name()` returns \"foo\"",
		Summary:   "`Register()` binds `NewFoo()` → `Foo.Name()` returns \"old-summary\"",
	}}
	sameTypedDifferentSummary := []types.EvidenceItem{{
		Kind:      types.EvidenceDataflowPath,
		Predicate: "resolution_chain",
		Subject:   "`Register()` binds `NewFoo()` → `Foo.Name()` returns \"foo\"",
		Summary:   "`Register()` binds `NewFoo()` → `Foo.Name()` returns \"different-summary\"",
	}}
	differentTypedSameSummary := []types.EvidenceItem{{
		Kind:      types.EvidenceDataflowPath,
		Predicate: "resolution_chain",
		Subject:   "`Register()` binds `NewBar()` → `Bar.Name()` returns \"bar\"",
		Summary:   "`Register()` binds `NewFoo()` → `Foo.Name()` returns \"old-summary\"",
	}}

	if got, want := hashChainTerminals(sameTypedDifferentSummary), hashChainTerminals(base); got != want {
		t.Fatalf("rendered Summary changes must not affect chain terminal hash: got %d want %d", got, want)
	}
	if got, same := hashChainTerminals(differentTypedSameSummary), hashChainTerminals(base); got == same {
		t.Fatalf("typed chain payload changes must affect chain terminal hash; both were %d", got)
	}
}

func TestHashChainTerminals_IgnoresSummaryOnlyChainText(t *testing.T) {
	items := []types.EvidenceItem{{
		Kind:      types.EvidenceDataflowPath,
		Predicate: "resolution_chain",
		Summary:   "`Register()` binds `NewFoo()` → `Foo.Name()` returns \"foo\"",
	}}
	if got := hashChainTerminals(items); got != 0 {
		t.Fatalf("summary-only chain text must not drive stall fingerprint, got hash %d", got)
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

func TestRunForcedReads_AcceptedClosureDropsAdvisoryPendingReads(t *testing.T) {
	o := newTestOrch(t)
	closure := o.busCtx.Mutable.EvidenceClosure()
	o.busCtx.Mutable.SetInvestigationComplete("accepted closure already covers the principal answer")
	o.busCtx.Mutable.ResetInvestigationComplete()
	closure.AddPendingRead(types.PendingRead{
		File:       "missing/support.go",
		Origin:     "chain_promotion.bridge_literal",
		Rationale:  "support-chain line after accepted closure",
		LineRanges: []types.LineRange{{Start: 10, End: 12}},
	})

	if got := o.runForcedReads(); got != 0 {
		t.Fatalf("advisory support debt must not trigger forced reads after accepted closure; got %d", got)
	}
	if pending := closure.PendingReads(); len(pending) != 0 {
		t.Fatalf("advisory support debt should be cleared after accepted closure, got %+v", pending)
	}
	if got := o.busCtx.Mutable.DispatchToolResults(); len(got) != 0 {
		t.Fatalf("advisory support debt should not execute read_file, got %+v", got)
	}
}

func TestRunForcedReads_AcceptedClosureKeepsLoadBearingPendingReads(t *testing.T) {
	o := newTestOrch(t)
	repoRoot := o.busCtx.RepoRoot
	relPath := "anchor.txt"
	if err := os.WriteFile(filepath.Join(repoRoot, relPath), []byte("load-bearing anchor\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	closure := o.busCtx.Mutable.EvidenceClosure()
	o.busCtx.Mutable.SetInvestigationComplete("accepted closure still has a precise anchor repair")
	o.busCtx.Mutable.ResetInvestigationComplete()
	closure.AddPendingRead(types.PendingRead{
		File:      relPath,
		Origin:    "pre_complete.primary_anchor",
		Rationale: "load-bearing anchor remains unread",
	})

	if got := o.runForcedReads(); got != 1 {
		t.Fatalf("load-bearing anchor debt should still be force-read, got %d", got)
	}
	if pending := closure.PendingReads(); len(pending) != 0 {
		t.Fatalf("load-bearing pending read should be drained after successful forced read, got %+v", pending)
	}
}

// TestRunForcedReads_BudgetCap: even with many PendingReads, the
// per-round cap (cgecForcedReadsPerRound) is respected.
func TestRunForcedReads_BudgetCap(t *testing.T) {
	o := newTestOrch(t)
	closure := o.busCtx.Mutable.EvidenceClosure()
	// Queue more than the cap using real current-checkout files. Missing
	// paths are cleared by the final qualification boundary before they can
	// consume the read budget, so cap behavior must be pinned with executable
	// forced-read seeds.
	for i := 0; i < cgecForcedReadsPerRound+5; i++ {
		rel := fmt.Sprintf("budget/file_%02d.go", i)
		if err := os.MkdirAll(filepath.Join(o.busCtx.RepoRoot, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(o.busCtx.RepoRoot, rel), []byte("package budget\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		closure.AddPendingRead(types.PendingRead{
			File:      rel,
			Rationale: "test",
			Origin:    "test",
		})
	}
	if got := o.runForcedReads(); got != cgecForcedReadsPerRound {
		t.Fatalf("forced reads = %d, want cap %d", got, cgecForcedReadsPerRound)
	}
	remaining := closure.PendingReads()
	if len(remaining) < 5 {
		t.Errorf("over-cap entries should remain unread; got %d remaining (started with %d)",
			len(remaining), cgecForcedReadsPerRound+5)
	}
}

// TestRunForcedReads_CancellationStopsSurgicalLoop pins the
// /cancel responsiveness contract: when the orchestrator's
// cancellation context fires while runForcedReads is iterating
// surgical chunks, the loop must bail out immediately at the next
// chunk boundary rather than completing every demanded range.
// Without this, a wide L4 SmallFileFull demand or a surgical
// PendingRead with many ranges would hold /cancel for up to ~1s
// of synthesised IO before responding.
func TestRunForcedReads_CancellationStopsSurgicalLoop(t *testing.T) {
	o := newTestOrch(t)
	repoRoot := o.busCtx.RepoRoot
	relPath := "cancel_target.go"
	abs := filepath.Join(repoRoot, relPath)
	var content strings.Builder
	for i := 1; i <= 600; i++ {
		content.WriteString("L")
		content.WriteString(testIntToString(i))
		content.WriteString("\n")
	}
	if err := os.WriteFile(abs, []byte(content.String()), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Plug in a cancelled context so the very first chunk-boundary
	// check fires.
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	o.busCtx.Ctx = cancelled

	closure := o.busCtx.Mutable.EvidenceClosure()
	closure.AddPendingRead(types.PendingRead{
		File:       relPath,
		Origin:     "test.cancel",
		Rationale:  "wide demand cancelled mid-flight",
		LineRanges: []types.LineRange{{Start: 1, End: 600}},
	})

	got := o.runForcedReads()
	if got != 0 {
		t.Errorf("cancelled context must abort runForcedReads with 0 successful drains; got %d", got)
	}
	results := o.busCtx.Mutable.DispatchToolResults()
	if len(results) != 0 {
		t.Errorf("no surgical reads should have completed under cancellation; got %d results", len(results))
	}
}

// TestRunForcedReads_SurgicalChunksLargeRange pins the chunking
// contract: a demand range larger than surgicalReadChunkLines must
// be split into consecutive sub-reads within the same
// runForcedReads pass so the closure ends up with the FULL
// demanded range covered in one stall round. Without chunking, a
// long-line file or wide L4 SmallFileFull demand would trip
// read_file's inline truncation, the closure would record only a
// truncated head range, the gate would re-raise the same demand,
// and runForcedReads would issue the same truncated read again —
// stalling for 3 rounds before I4 force-completes.
func TestRunForcedReads_SurgicalChunksLargeRange(t *testing.T) {
	o := newTestOrch(t)
	repoRoot := o.busCtx.RepoRoot
	relPath := "wide_demand.go"
	abs := filepath.Join(repoRoot, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// 600-line file so the demand spans 3 × surgicalReadChunkLines
	// (200) chunks.
	var content strings.Builder
	for i := 1; i <= 600; i++ {
		content.WriteString("L")
		content.WriteString(testIntToString(i))
		content.WriteString("\n")
	}
	if err := os.WriteFile(abs, []byte(content.String()), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	closure := o.busCtx.Mutable.EvidenceClosure()
	closure.AddPendingRead(types.PendingRead{
		File:       relPath,
		Rationale:  "wide surgical demand spanning multiple chunk budgets",
		Origin:     "pre_complete.multi_path_anchor",
		LineRanges: []types.LineRange{{Start: 1, End: 600}}, // 600 lines = 3 × 200
	})

	got := o.runForcedReads()
	if got != 1 {
		t.Fatalf("expected 1 successful PendingRead drain; got %d", got)
	}
	results := o.busCtx.Mutable.DispatchToolResults()
	if len(results) != 3 {
		t.Fatalf("expected 3 chunked surgical ToolResults (600 lines / 200 per chunk); got %d", len(results))
	}
	expectedRanges := []string{
		"showing lines 1-200",
		"showing lines 201-400",
		"showing lines 401-600",
	}
	for i, want := range expectedRanges {
		if !strings.Contains(results[i].Summary, want) {
			t.Errorf("result[%d] banner does not match expected chunk; want substring %q; got: %s",
				i, want, firstLineOf(results[i].Summary))
		}
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

// TestRunForcedReads_EmptyFile_LegacyPath_ConvergesViaFullyRead
// pins the empty-file integration: a 0-byte file demanded via the
// legacy full-file path is read once, the closure records the
// read_file banner's "showing all 1 lines (0 bytes)" or "showing
// lines 1-1 of 1 total" shape with totalLines=1, and HasFullyRead
// short-circuits any subsequent gate pass. No infinite loop.
func TestRunForcedReads_EmptyFile_LegacyPath_ConvergesViaFullyRead(t *testing.T) {
	o := newTestOrch(t)
	repoRoot := o.busCtx.RepoRoot
	relPath := "empty.txt"
	if err := os.WriteFile(filepath.Join(repoRoot, relPath), nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	closure := o.busCtx.Mutable.EvidenceClosure()
	closure.AddPendingRead(types.PendingRead{
		File:      relPath,
		Origin:    "test.empty",
		Rationale: "demand triggered against empty file",
	})

	if got := o.runForcedReads(); got != 1 {
		t.Fatalf("empty-file forced-read should still report success (read_file does not fail on 0 bytes); got %d", got)
	}
	results := o.busCtx.Mutable.DispatchToolResults()
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 forced-read result for the empty file, got %d", len(results))
	}
	// PendingRead is cleared (verified by ClearPendingReadFor in
	// runForcedReads) so subsequent gate passes see no demand.
	if got := closure.PendingReads(); len(got) != 0 {
		t.Errorf("PendingReads must be empty after successful empty-file forced-read; got %+v", got)
	}
}

// TestRunForcedReads_NotExist_AbandonsAndAdvises pins the
// permission/missing-file abandonment contract: a forced-read on a
// path that does not exist must NOT loop forever (legacy bounded
// behaviour was 3 stall iterations). The new abandon path detects
// os.IsNotExist via os.Stat probe and clears the PendingRead +
// emits a RepairExpandSearch advisory so the LLM is informed and
// the gate budget is freed for productive work.
func TestRunForcedReads_NotExist_AbandonsAndAdvises(t *testing.T) {
	o := newTestOrch(t)
	closure := o.busCtx.Mutable.EvidenceClosure()
	const relPath = "definitely/does/not/exist.go"
	closure.AddPendingRead(types.PendingRead{
		File:       relPath,
		Origin:     "test.missing",
		Rationale:  "demand against ENOENT path",
		LineRanges: []types.LineRange{{Start: 100, End: 130}}, // surgical demand
	})

	got := o.runForcedReads()
	if got != 0 {
		t.Errorf("ENOENT forced-read must NOT count as success; got %d", got)
	}
	if pending := closure.PendingReads(); len(pending) != 0 {
		t.Errorf("ENOENT path's PendingRead must be cleared by abandon path; got %+v", pending)
	}
	advisoryFound := false
	for _, r := range closure.PendingRepairs() {
		if r.Origin == "forced_read.unqualified.test.missing" && r.Kind == types.RepairExpandSearch {
			advisoryFound = true
			if !strings.Contains(r.Rationale, "skipped forced-read seed") {
				t.Errorf("advisory rationale should explain pre-execution skip; got: %s", r.Rationale)
			}
			if !strings.Contains(r.Rationale, "file not found") {
				t.Errorf("advisory rationale should classify the failure (file not found); got: %s", r.Rationale)
			}
		}
	}
	if !advisoryFound {
		t.Fatalf("expected RepairExpandSearch advisory with origin forced_read.unqualified.test.missing; got %+v", closure.PendingRepairs())
	}
}

// TestRunForcedReads_PermissionDenied_AbandonsWithCorrectRationale
// pins the EACCES branch of the abandon contract: a real file the
// process has no read permission for is abandoned with the failure
// classified as "permission denied" in the LLM-facing advisory.
//
// Skipped on Windows / when the test runs as root because chmod 0
// semantics differ.
func TestRunForcedReads_PermissionDenied_AbandonsWithCorrectRationale(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-denied semantics differ on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("test runs as root; chmod 0 does not deny read")
	}
	o := newTestOrch(t)
	repoRoot := o.busCtx.RepoRoot
	relPath := "no_perms.go"
	abs := filepath.Join(repoRoot, relPath)
	if err := os.WriteFile(abs, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(abs, 0o000); err != nil {
		t.Fatalf("chmod 000: %v", err)
	}
	defer os.Chmod(abs, 0o644) // restore so t.TempDir cleanup works

	closure := o.busCtx.Mutable.EvidenceClosure()
	closure.AddPendingRead(types.PendingRead{
		File:      relPath,
		Origin:    "test.eacces",
		Rationale: "demand against unreadable file",
	})

	o.runForcedReads()
	if pending := closure.PendingReads(); len(pending) != 0 {
		t.Errorf("EACCES path PendingRead must be cleared; got %+v", pending)
	}
	advisoryFound := false
	for _, r := range closure.ActiveRepairs() {
		if r.Origin != "forced_read.unrecoverable.test.eacces" {
			continue
		}
		advisoryFound = true
		if !strings.Contains(r.Rationale, "permission denied") {
			t.Errorf("permission-denied rationale should classify the failure; got: %s", r.Rationale)
		}
	}
	if !advisoryFound {
		t.Fatalf("expected advisory with origin forced_read.unrecoverable.test.eacces; got %+v", closure.ActiveRepairs())
	}
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

func TestSeedRequiredFileHintForcedReadsBeforeExplore_UsesSharedCurrentSourceContract(t *testing.T) {
	o := newTestOrch(t)
	for _, rel := range []string{
		"internal/llm/openai.go",
		"internal/orchestrator/read_stage_retry.go",
	} {
		if err := os.MkdirAll(filepath.Join(o.busCtx.RepoRoot, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(filepath.Join(o.busCtx.RepoRoot, rel), []byte("package p\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	o.busCtx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain,
		LogTriage: &types.LogBundle{
			Errors: []types.LogError{{Type: "first_byte_timeout"}},
		},
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic:        true,
			CurrentVersionCheck: true,
		},
		AnalyzerHints: types.AnalyzerHints{RequiredFileHints: []types.RequiredFileHint{
			{Path: "./internal/llm/openai.go", Confidence: 0.9},
			{Path: "internal/orchestrator/read_stage_retry.go", Confidence: 0.8},
			{Path: "internal/render/status_messages.go", Confidence: 0.79},
		}},
	}}

	if got := o.seedRequiredFileHintForcedReadsBeforeExplore(); got != 2 {
		t.Fatalf("seeded required-file pending reads = %d, want 2 high-confidence hints", got)
	}
	pending := o.busCtx.Mutable.EvidenceClosure().PendingReads()
	if len(pending) != 2 {
		t.Fatalf("pending reads = %+v, want 2", pending)
	}
	if pending[0].File != "internal/llm/openai.go" {
		t.Errorf("first pending file = %q, want canonical internal/llm/openai.go", pending[0].File)
	}
	if pending[0].Origin != "pre_dispatch.required_file_hint_unread" {
		t.Errorf("pending origin = %q", pending[0].Origin)
	}
	if again := o.seedRequiredFileHintForcedReadsBeforeExplore(); again != 0 {
		t.Fatalf("second seed should dedupe existing pending reads, got %d", again)
	}
}

func TestSeedRequiredFileHintForcedReadsBeforeExplore_SkipsPhantomSingleRepoPath(t *testing.T) {
	o := newTestOrch(t)
	real := "internal/llm/openai.go"
	if err := os.MkdirAll(filepath.Join(o.busCtx.RepoRoot, filepath.Dir(real)), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(o.busCtx.RepoRoot, real), []byte("package p\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	o.busCtx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain,
		LogTriage: &types.LogBundle{
			Errors: []types.LogError{{Type: "first_byte_timeout"}},
		},
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic:        true,
			CurrentVersionCheck: true,
		},
		AnalyzerHints: types.AnalyzerHints{RequiredFileHints: []types.RequiredFileHint{
			{Path: "history/old/removed_file.go", Confidence: 0.95},
			{Path: real, Confidence: 0.9},
		}},
	}}

	if got := o.seedRequiredFileHintForcedReadsBeforeExplore(); got != 1 {
		t.Fatalf("seeded required-file pending reads = %d, want only the existing current file", got)
	}
	pending := o.busCtx.Mutable.EvidenceClosure().PendingReads()
	if len(pending) != 1 || pending[0].File != real {
		t.Fatalf("pending reads should exclude phantom history path, got %+v", pending)
	}
}

func TestSeedRequiredFileHintForcedReadsBeforeExplore_ObservationOnlySkips(t *testing.T) {
	o := newTestOrch(t)
	o.busCtx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain,
		LogTriage: &types.LogBundle{
			Errors: []types.LogError{{Type: "first_byte_timeout"}},
		},
	}}

	if got := o.seedRequiredFileHintForcedReadsBeforeExplore(); got != 0 {
		t.Fatalf("observation-only artifact should not seed current-source reads, got %d", got)
	}
}

func TestRunForcedReads_PreDispatchRequiredFilesUseSharedCoverageCap(t *testing.T) {
	o := newTestOrch(t)
	closure := o.busCtx.Mutable.EvidenceClosure()
	for i, rel := range []string{"a.go", "b.go", "c.go", "d.go"} {
		if err := os.WriteFile(filepath.Join(o.busCtx.RepoRoot, rel), []byte("package p\n"), 0o644); err != nil {
			t.Fatalf("write fixture %d: %v", i, err)
		}
		closure.AddPendingRead(types.PendingRead{
			File:      rel,
			Origin:    "pre_dispatch.required_file_hint_unread",
			Rationale: "required-file preseed",
			Stage:     string(types.StageExplore),
		})
	}

	if got := o.runForcedReads(); got != types.RequiredFileHintCoverageMax {
		t.Fatalf("forced reads = %d, want shared required-file cap %d", got, types.RequiredFileHintCoverageMax)
	}
	if pending := closure.PendingReads(); len(pending) != 0 {
		t.Fatalf("pre-dispatch required-file reads should drain in one pass, got %+v", pending)
	}
}

func TestSeedRequiredFileHintForcedReadsBeforeExplore_SourceInventoryUsesInventoryCap(t *testing.T) {
	o := newTestOrch(t)
	var hints []types.RequiredFileHint
	for i := 0; i < types.SourceInventoryRequiredFileHintCoverageMax+2; i++ {
		rel := fmt.Sprintf("src/file_%02d.ets", i)
		if err := os.MkdirAll(filepath.Join(o.busCtx.RepoRoot, "src"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(o.busCtx.RepoRoot, rel), []byte("@Entry\n"), 0o644); err != nil {
			t.Fatalf("write fixture %d: %v", i, err)
		}
		hints = append(hints, types.RequiredFileHint{
			Path:       rel,
			Confidence: 0.9,
		})
	}
	o.busCtx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		AnalyzerHints: types.AnalyzerHints{
			Kind:              string(types.ReqEnumeration),
			RequiredFileHints: hints,
		},
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		},
	}}

	if got := o.seedRequiredFileHintForcedReadsBeforeExplore(); got != types.SourceInventoryRequiredFileHintCoverageMax {
		t.Fatalf("queued=%d, want source inventory cap %d", got, types.SourceInventoryRequiredFileHintCoverageMax)
	}
	if pending := o.busCtx.Mutable.EvidenceClosure().PendingReads(); len(pending) != types.SourceInventoryRequiredFileHintCoverageMax {
		t.Fatalf("pending=%d, want source inventory cap %d: %+v", len(pending), types.SourceInventoryRequiredFileHintCoverageMax, pending)
	}
}

// TestRunForcedReads_PathIsDirectory_AbandonsAcrossPlatforms pins
// the IsDir cross-platform branch: a PendingRead pointing at a
// directory (broken classification, race, manifest-included path
// that's actually a folder) is abandoned uniformly on Linux /
// macOS / Windows because os.FileInfo.IsDir() is OS-agnostic.
func TestRunForcedReads_PathIsDirectory_AbandonsAcrossPlatforms(t *testing.T) {
	o := newTestOrch(t)
	repoRoot := o.busCtx.RepoRoot
	relPath := "actually_a_dir"
	if err := os.MkdirAll(filepath.Join(repoRoot, relPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	closure := o.busCtx.Mutable.EvidenceClosure()
	closure.AddPendingRead(types.PendingRead{
		File:      relPath,
		Origin:    "test.isdir",
		Rationale: "demand against directory",
	})

	o.runForcedReads()
	if pending := closure.PendingReads(); len(pending) != 0 {
		t.Errorf("directory PendingRead must be cleared on every OS; got %+v", pending)
	}
	advisoryFound := false
	for _, r := range closure.PendingRepairs() {
		if r.Origin == "forced_read.unqualified.test.isdir" {
			advisoryFound = true
			if !strings.Contains(r.Rationale, "directory") {
				t.Errorf("directory advisory should classify the failure as 'directory'; got: %s", r.Rationale)
			}
		}
	}
	if !advisoryFound {
		t.Fatalf("expected directory advisory; got %+v", closure.PendingRepairs())
	}
}

func TestRunForcedReads_SkipsUnqualifiedHistoricalAndExternalPendingReads(t *testing.T) {
	o := newTestOrch(t)
	repoRoot := o.busCtx.RepoRoot
	real := "internal/agent/real.go"
	if err := os.MkdirAll(filepath.Join(repoRoot, filepath.Dir(real)), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, real), []byte("package agent\n"), 0o644); err != nil {
		t.Fatalf("write real: %v", err)
	}
	external := filepath.Join(t.TempDir(), "external.go")
	if err := os.WriteFile(external, []byte("package external\n"), 0o644); err != nil {
		t.Fatalf("write external: %v", err)
	}
	closure := o.busCtx.Mutable.EvidenceClosure()
	for _, pending := range []types.PendingRead{
		{File: "history/old/removed.go", Origin: "test.missing", Rationale: "historical ranked path"},
		{File: "../outside.go", Origin: "test.traversal", Rationale: "parent traversal"},
		{File: external, Origin: "test.external", Rationale: "repo-external absolute path"},
		{File: real, Origin: "test.real", Rationale: "current checkout path"},
	} {
		closure.AddPendingRead(pending)
	}

	if got := o.runForcedReads(); got != 1 {
		t.Fatalf("forced reads = %d, want only the current checkout file", got)
	}
	if pending := closure.PendingReads(); len(pending) != 0 {
		t.Fatalf("all executable/unqualified pending reads should be drained or cleared, got %+v", pending)
	}
	if !closure.HasRead(real) {
		t.Fatalf("real current-checkout file should be read")
	}
	repairs := closure.PendingRepairs()
	for _, origin := range []string{
		"forced_read.unqualified.test.missing",
		"forced_read.unqualified.test.traversal",
		"forced_read.unqualified.test.external",
	} {
		found := false
		for _, repair := range repairs {
			if repair.Origin == origin && repair.Advisory {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing advisory repair for %s in %+v", origin, repairs)
		}
	}
}

// TestSummarizeReadFailure_CrossPlatformClassification verifies the
// classifier handles the ergonomic branches each platform actually
// produces. We construct the standard error wrappers Go uses for
// IsNotExist / IsPermission so this test runs on every host OS
// (no syscall.Errno construction needed — fs.ErrNotExist /
// fs.ErrPermission are platform-neutral and IsNotExist /
// IsPermission accept them by design).
func TestSummarizeReadFailure_CrossPlatformClassification(t *testing.T) {
	cases := []struct {
		name    string
		statErr error
		isDir   bool
		ioErr   error
		summary string
		want    string
	}{
		{name: "isDir wins", isDir: true, statErr: nil, want: "path is a directory, not a file"},
		{name: "stat ENOENT", statErr: os.ErrNotExist, want: "file not found"},
		{name: "stat EACCES", statErr: os.ErrPermission, want: "permission denied"},
		{name: "io ENOENT fallback", ioErr: os.ErrNotExist, want: "file not found"},
		{name: "io EACCES fallback", ioErr: os.ErrPermission, want: "permission denied"},
		{name: "summary parse", summary: "read failed: open /tmp/x: no such file or directory", want: "open /tmp/x: no such file or directory"},
		{name: "all empty", want: "framework read failed for an unknown reason"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := summarizeReadFailure(tc.statErr, tc.isDir, tc.ioErr, tc.summary)
			if got != tc.want {
				t.Errorf("summarize = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestClassifyForcedReadFailure_VerbatimWinsWhenJoinedFails: when
// the verbatim path resolves to a real file and the joined path
// does not, the classifier must NOT abandon — the tool would have
// succeeded on the verbatim path. This guards against a regression
// that would lose PendingReads any time RepoRoot was misconfigured
// while the verbatim path was correct.
func TestClassifyForcedReadFailure_VerbatimWinsWhenJoinedFails(t *testing.T) {
	dir := t.TempDir()
	verbatim := filepath.Join(dir, "real.go")
	if err := os.WriteFile(verbatim, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	bus := &types.BusContext{RepoRoot: filepath.Join(dir, "wrong_root")}

	verdict, _, _ := classifyForcedReadFailure(bus, verbatim, os.ErrNotExist)
	if verdict {
		t.Errorf("verbatim path resolves cleanly; classifier must NOT abandon even when ioErr is misleading")
	}
}

// TestRunForcedReads_TransientFailure_KeepsPendingForRetry: when the
// forced-read fails but the file actually exists and is readable
// (transient failure — read raced with delete then restore, NFS
// blip), the abandon path must NOT fire. The PendingRead stays so
// the next runForcedReads call can retry; the existing I4 stall
// detection bounds the worst case at 3 retries before force-complete.
//
// We simulate this by: (a) creating a real readable file, (b)
// constructing a synthetic ToolResult{Success:false} that mimics the
// "transient" pattern, and (c) checking the abandon decision.
// Direct-call style: tests the helper, not the runForcedReads loop,
// so we don't have to fake a transient os.ReadFile failure.
func TestRunForcedReads_TransientFailure_KeepsPendingForRetry(t *testing.T) {
	o := newTestOrch(t)
	repoRoot := o.busCtx.RepoRoot
	relPath := "exists.go"
	if err := os.WriteFile(filepath.Join(repoRoot, relPath), []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// File exists + is readable. isStructurallyUnrecoverableReadFailure
	// must return false even when the caller passes a transient err.
	transientErr := os.ErrInvalid
	if isStructurallyUnrecoverableReadFailure(o.busCtx, relPath, transientErr) {
		t.Errorf("transient failure on a readable file must NOT be treated as unrecoverable")
	}
}
