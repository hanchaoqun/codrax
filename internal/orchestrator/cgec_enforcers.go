package orchestrator

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// cgec_enforcers.go bundles the orchestrator-side CGEC (Citation-
// Grounded Evidence Closure) enforcers for invariants I3 (pre-
// complete contract check; partial — emit_investigation_complete
// owns the per-tool half) and I4 (cross-round convergence detector +
// Lazy Auto-Read). The producers / consumers live in:
//
//   - chain promotion (I1): internal/agent/explorer.go::applyChainPromotion
//   - grounder repair (I2): internal/tool/emit_answer_document.go grounder
//   - pre-complete (I3):    internal/tool/emit_investigation_complete.go
//   - convergence (I4):     this file
//
// Both functions here mutate state via the per-Run EvidenceClosure
// (set up by NewOrchestrator at task entry through the Mutable
// lifecycle).

// CGEC tunables. All overridable from codrax.yaml via the
// cgec_* keys; cmd/root.go calls SetCGECPolicy after
// LoadRuntimeSettings to apply any operator overrides. The
// defaults here are the values the unit + e2e tests pin against.
//
// Why mutable vars and not constants: same reason BlobLimits and
// AnalysisLimits are mutable — operators tune them per deployment
// without recompiling.
var (
	// cgecForcedReadsPerRound is the per-explore-round cap on Lazy
	// Auto-Read invocations. Three is enough to compensate for one
	// or two LLM oversights without turning the framework into a
	// parallel reader; an over-eager cap would break ReAct autonomy
	// by reading far ahead of what the LLM has been told to
	// investigate.
	cgecForcedReadsPerRound = 3

	// cgecStallThresholdSoft is the fingerprint-equality threshold
	// at which the convergence detector raises a forced-read repair
	// directive. Two consecutive identical rounds = "the LLM made
	// no progress on the third try"; we step in before retry budget
	// is exhausted.
	cgecStallThresholdSoft = 2

	// cgecStallThresholdHard is the threshold at which the
	// convergence detector force-completes the investigation.
	// Three identical rounds in a row is an unrecoverable pattern;
	// we let the finalizer ship a best-effort answer instead of
	// looping until budget exhaustion.
	cgecStallThresholdHard = 3
)

// SetCGECPolicy overrides the CGEC tunables. cmd/root.go calls this
// after LoadRuntimeSettings with any non-nil overrides.
//
// Disable semantics:
//
//	cgec_forced_reads_per_round = 0  → Lazy Auto-Read disabled
//	                                   ("LLM is on its own"). Mirrors
//	                                   analysis_max_prescan_rounds: 0.
//	cgec_stall_threshold_soft   = 0  → soft stall detection off
//	cgec_stall_threshold_hard   = 0  → hard stall (force-complete) off
//
// Negative values are programming errors and are ignored — operators
// that mistype "-1" should not silently zero out the policy.
func SetCGECPolicy(forcedReadsPerRound, stallSoft, stallHard int) {
	if forcedReadsPerRound >= 0 {
		cgecForcedReadsPerRound = forcedReadsPerRound
	}
	if stallSoft >= 0 {
		cgecStallThresholdSoft = stallSoft
	}
	if stallHard >= 0 {
		cgecStallThresholdHard = stallHard
	}
}

// CGECPolicy returns the current tunable values. Used by tests and
// debug log lines.
func CGECPolicy() (forcedReadsPerRound, stallSoft, stallHard int) {
	return cgecForcedReadsPerRound, cgecStallThresholdSoft, cgecStallThresholdHard
}

// runForcedReads is the Lazy Auto-Read enforcer (CGEC E2). When the
// closure has queued PendingReads (raised by chain promotion or by
// emit_answer_document grounder rejects) and the LLM has not picked
// them up in its own read_file calls, the orchestrator reads them on
// the LLM's behalf so the readSet closes against the contract. The
// synthesized tool result is tagged `[forced_read]` so the operator
// can grep the trace for framework-driven reads vs. LLM-driven reads.
//
// Returns the number of files actually forced-read this round.
func (o *Orchestrator) runForcedReads() int {
	if o.busCtx == nil || o.busCtx.Mutable == nil {
		return 0
	}
	// Lazy Auto-Read disabled by configuration. Mirrors the
	// analysis_max_prescan_rounds: 0 disable convention.
	if cgecForcedReadsPerRound == 0 {
		return 0
	}
	closure := o.busCtx.Mutable.EvidenceClosure()
	readSet := closure.ReadSet()

	// A1 bridge: grounder-raised RepairReadFile directives are
	// mirrored into PendingReads at AddRepair time, so this single
	// loop covers BOTH chain-promotion pending reads AND grounder
	// reject pending reads. No need for a second PEEK over Repairs.
	var toRead []types.PendingRead
	for _, p := range closure.PendingReads() {
		if readSet[p.File] {
			closure.ClearPendingReadFor(p.File)
			continue
		}
		toRead = append(toRead, p)
		if len(toRead) >= cgecForcedReadsPerRound {
			break
		}
	}
	if len(toRead) == 0 {
		return 0
	}

	rf := &tool.ReadFile{}
	success := 0
	for _, p := range toRead {
		// Try the path verbatim first; fall back to RepoRoot-joined
		// if the verbatim read fails. Most repos run codrax from the
		// repo root so the relative path resolves; the join is the
		// belt-and-suspenders.
		params, _ := json.Marshal(map[string]any{"path": p.File})
		result, err := rf.Execute(o.busCtx, params)
		if err != nil || !result.Success {
			if o.busCtx.RepoRoot != "" {
				params, _ = json.Marshal(map[string]any{"path": filepath.Join(o.busCtx.RepoRoot, p.File)})
				result, err = rf.Execute(o.busCtx, params)
			}
		}
		if err != nil || !result.Success {
			logging.Warning("[CGEC] E2 forced-read failed: file=%s err=%v summary=%s", p.File, err, result.Summary)
			continue
		}
		// Tag the synthesized result so trace consumers can tell it
		// from a real LLM-driven read.
		result.Summary = "[forced_read] " + result.Summary
		// Hook into the same channels real reads flow through:
		//   - DispatchToolResults: the per-dispatch buffer the
		//     extractFileCoverage helper consults
		//   - TurnAArtifacts.ReadFiles + ToolResults: the canonical
		//     channel the finalizer's grounder uses to whitelist
		//     citations
		//   - BusContext.ToolResults: the cumulative history
		o.busCtx.Mutable.AppendDispatchToolResult(result)
		o.busCtx.ToolResults = append(o.busCtx.ToolResults, result)
		artifacts := o.busCtx.Mutable.TurnAArtifacts()
		if artifacts == nil {
			artifacts = &types.TurnAArtifacts{
				UserQuestion: o.busCtx.Mutable.Objective(),
			}
		}
		if artifacts.UserQuestion == "" && o.busCtx.Mutable.Objective() != "" {
			artifacts.UserQuestion = o.busCtx.Mutable.Objective()
		}
		artifacts.ReadFiles = appendUniqueString(artifacts.ReadFiles, p.File)
		artifacts.ToolResults = append(artifacts.ToolResults, result)
		o.busCtx.Mutable.SetTurnAArtifacts(*artifacts)

		readSet[p.File] = true
		closure.ClearPendingReadFor(p.File)
		success++
		logging.Info("[CGEC] E2 forced-read: file=%s origin=%s rationale=%s", p.File, p.Origin, p.Rationale)
	}
	if success > 0 {
		closure.SetReadSet(readSet)
		closure.BumpForcedReads(success)
		o.emit(render.Event{
			Kind:      render.EventAgentReasoning,
			Timestamp: time.Now(),
			Agent:     "orchestrator",
			Reasoning: fmt.Sprintf("⟳ Forced-read %d file(s) the LLM skipped (CGEC E2)", success),
		})
	}
	return success
}

// detectStallAndAct is the CGEC I4 convergence detector. Computes a
// ClosureFingerprint over (ReadSet, EvidenceCount, ChainTerminalSet)
// and compares against the rolling history. Two identical
// fingerprints in a row → raise a force-read repair (and call
// runForcedReads with elevated cap). Three in a row → mark
// investigation complete to force the loop to exit.
//
// Returns true when a hard stall was detected and the caller should
// break out of the explore loop.
func (o *Orchestrator) detectStallAndAct() bool {
	if o.busCtx == nil || o.busCtx.Mutable == nil {
		return false
	}
	closure := o.busCtx.Mutable.EvidenceClosure()
	fp := types.ClosureFingerprint{
		ReadSetHash:   types.HashFileSet(closure.ReadSet()),
		EvidenceHash:  hashEvidenceIDs(o.busCtx.Mutable.EmittedEvidence()),
		ChainTermSet:  hashChainTerminals(o.busCtx.Mutable.EmittedEvidence()),
		CitedRefsHash: hashCitedRefs(closure.CitedRefs()),
	}
	closure.AppendFingerprint(fp)
	hist := closure.Fingerprints()
	// Soft threshold = 0 disables the entire detector (no
	// observability either; for that, set hard high but keep
	// observability via Stats()).
	if cgecStallThresholdSoft == 0 {
		return false
	}
	if len(hist) < cgecStallThresholdSoft {
		return false
	}
	// Soft threshold: last cgecStallThresholdSoft consecutive
	// fingerprints must all be equal. Generic over the threshold so
	// operators can tighten / loosen via codrax.yaml without code
	// change.
	if !lastNFingerprintsEqual(hist, cgecStallThresholdSoft) {
		return false
	}
	logging.Warning("[orchestrator] CGEC I4: convergence stall detected (round %d, soft threshold %d)", len(hist), cgecStallThresholdSoft)
	closure.BumpStallSoftHits(1)
	// Try to break the stall by force-reading anything that's been
	// queued but not satisfied yet.
	read := o.runForcedReads()
	if read > 0 {
		// At least one file was read; the next round may converge.
		// Do not declare hard stall yet — give the closure a chance
		// to record progress.
		return false
	}
	// Hard threshold: cgecStallThresholdHard consecutive identical
	// fingerprints AND no forced read fired. Threshold = 0 disables
	// the hard exit (operators that want soft observability without
	// force-complete set hard=0).
	if cgecStallThresholdHard > 0 && len(hist) >= cgecStallThresholdHard {
		if lastNFingerprintsEqual(hist, cgecStallThresholdHard) {
			logging.Error("[orchestrator] CGEC I4: hard stall — force-completing investigation to ship best-effort answer")
			closure.BumpStallHardHits(1)
			closure.AddRepair(types.RepairDirective{
				Kind:      types.RepairForceCompleteDowngrade,
				Rationale: fmt.Sprintf("no progress detected across %d consecutive explore rounds", cgecStallThresholdHard),
				Origin:    "convergence_detector",
			})
			o.busCtx.Mutable.SetInvestigationComplete(fmt.Sprintf("CGEC I4 hard stall: %d identical fingerprints", cgecStallThresholdHard))
			o.emit(render.Event{
				Kind:      render.EventAgentReasoning,
				Timestamp: time.Now(),
				Agent:     "orchestrator",
				Reasoning: "⚠️ CGEC I4: convergence stall — force-completing with current evidence",
			})
			return true
		}
	}
	return false
}

// lastNFingerprintsEqual reports whether the last n entries of hist
// are all pairwise equal. Returns false when len(hist) < n. Helper
// shared by the soft + hard threshold checks so the comparison
// logic stays uniform.
func lastNFingerprintsEqual(hist []types.ClosureFingerprint, n int) bool {
	if n < 2 || len(hist) < n {
		return false
	}
	for i := len(hist) - n; i < len(hist)-1; i++ {
		if !fingerprintsEqual(hist[i], hist[i+1]) {
			return false
		}
	}
	return true
}

// appendUniqueString appends s to slice when it is not already present.
// Order-preserving; O(n) per call which is fine for the small TurnA
// ReadFiles slice.
func appendUniqueString(slice []string, s string) []string {
	for _, existing := range slice {
		if existing == s {
			return slice
		}
	}
	return append(slice, s)
}

// hashEvidenceIDs computes a stable hash over the EvidenceItem.ID
// strings so the convergence detector can detect "no new evidence
// emitted this round". ID is the canonical EvidenceItem identity
// (StableEvidenceID) so two identical pieces of evidence collide on
// the hash regardless of buffer growth.
func hashEvidenceIDs(items []types.EvidenceItem) uint32 {
	if len(items) == 0 {
		return 0
	}
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, string(it.ID))
	}
	return types.HashStringSet(ids)
}

// hashChainTerminals computes a stable hash over the unique
// resolution-chain terminal tokens in the evidence buffer. Detects
// the case where the LLM keeps surfacing the same chain endings
// across retries — distinct from "no new evidence" because the
// evidence count may grow while the answer-relevant chain set
// stays static.
func hashChainTerminals(items []types.EvidenceItem) uint32 {
	if len(items) == 0 {
		return 0
	}
	seen := make(map[string]bool)
	var terms []string
	for _, it := range items {
		if it.Kind != types.EvidenceDataflowPath || it.Predicate != "resolution_chain" {
			continue
		}
		// Take the substring after the last "→" as the terminal.
		s := it.Summary
		if i := lastIndexArrow(s); i >= 0 {
			s = s[i+len("→"):]
		}
		s = trimChainTerminal(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		terms = append(terms, s)
	}
	if len(terms) == 0 {
		return 0
	}
	sort.Strings(terms)
	return types.HashStringSet(terms)
}

// lastIndexArrow returns the byte index of the rightmost "→" in s.
// Local helper instead of strings.LastIndex so this file's chain
// hash stays self-contained.
func lastIndexArrow(s string) int {
	const arrow = "→"
	idx := -1
	for i := 0; i+len(arrow) <= len(s); i++ {
		if s[i:i+len(arrow)] == arrow {
			idx = i
		}
	}
	return idx
}

// trimChainTerminal trims surrounding whitespace, backticks, and
// quotes from a chain terminal token. Used by the chain-terminal
// hash so wording variants ("`Foo.Name()`" vs `Foo.Name() `) collapse.
func trimChainTerminal(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '`' || s[0] == '"' || s[0] == '\'' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '`' || s[len(s)-1] == '"' || s[len(s)-1] == '\'' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

// fingerprintsEqual compares two ClosureFingerprint values across
// all four dimensions.
func fingerprintsEqual(a, b types.ClosureFingerprint) bool {
	return a.ReadSetHash == b.ReadSetHash &&
		a.EvidenceHash == b.EvidenceHash &&
		a.ChainTermSet == b.ChainTermSet &&
		a.CitedRefsHash == b.CitedRefsHash
}

// hashCitedRefs computes a stable hash over the (file, line) pairs
// in the citation pool. Detects "the LLM keeps citing the same
// references across retries" stalls — a different signal from
// "no new evidence" because the LLM may emit new evidence yet
// re-cite the same lines in finalizer.
func hashCitedRefs(refs map[string][]int) uint32 {
	if len(refs) == 0 {
		return 0
	}
	files := make([]string, 0, len(refs))
	for f := range refs {
		files = append(files, f)
	}
	sort.Strings(files)
	var entries []string
	for _, f := range files {
		lines := append([]int(nil), refs[f]...)
		sort.Ints(lines)
		for _, l := range lines {
			entries = append(entries, fmt.Sprintf("%s:%d", f, l))
		}
	}
	return types.HashStringSet(entries)
}
