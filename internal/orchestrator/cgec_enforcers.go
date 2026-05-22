package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
// surgicalReadChunkLines bounds each individual surgical read_file
// call inside runForcedReads so the rendered banner content never
// trips read_file's inline MaxInlineBytes (32 KB default) blob
// truncation. With ~80-100 bytes per line plus a 12-byte gutter, a
// 200-line chunk renders ~22 KB — comfortably under the budget for
// typical source code, with headroom for files with longer-than-
// average lines. Demand ranges larger than this are split into
// consecutive chunk reads within the same runForcedReads pass so
// the closure records the full demanded range in one stall round
// (without chunking, a truncated banner would leave the upper
// portion of the demand uncovered, the gate would re-raise the
// same demand, and the fingerprint would stay stable until the I4
// hard-stall fired three rounds later).
const surgicalReadChunkLines = 200

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
	// readSet is the canonical-key snapshot. Used at the success-
	// path bottom (line ~326 closure.SetReadSet(readSet)) to push
	// the post-forced-read state back to the closure in one shot.
	// 2026-05-10 path-canonicalization audit: the per-pending-read
	// loop check is now closure.HasRead(p.File) (canonicalises at
	// the boundary) instead of `readSet[p.File]` (raw lookup). The
	// local snapshot still receives `readSet[p.File] = true` after
	// each successful forced read so the SetReadSet writeback at
	// the bottom carries the new files.
	readSet := closure.ReadSet()

	// A1 bridge: grounder-raised RepairReadFile directives are
	// mirrored into PendingReads at AddRepair time, so this single
	// loop covers BOTH chain-promotion pending reads AND grounder
	// reject pending reads. No need for a second PEEK over Repairs.
	//
	// CGEC D4 (advisory): log a warning when a PendingRead file is
	// NOT in ScannedSet — it may be a ghost path but could also be
	// a legitimate new file the LLM discovered via mid-run grep.
	// Do not SKIP the read because we don't want to false-negative
	// legitimate late-discovered files; if it really is a ghost,
	// the read_file tool call will fail gracefully and the warning
	// at line 150 will fire. The advisory warning here gives the
	// operator a grep-able heads-up before the attempt.
	var toRead []types.PendingRead
	limit := cgecForcedReadsPerRound
	if hasPreDispatchRequiredFilePendingRead(closure) && limit < types.RequiredFileHintCoverageMax {
		limit = types.RequiredFileHintCoverageMax
	}
	for _, p := range closure.PendingReads() {
		// 2026-05-10 path-canonicalization audit: switched from
		// raw `readSet[p.File]` to closure.HasRead(p.File). The
		// readSet map keys are canonical (SetReadSet canonicalizes
		// at insert), and addPendingReadLocked now canonicalizes
		// p.File at insert too — but using HasRead bulletproofs
		// against a future regression where a non-canonical path
		// slips through the closure boundary.
		if closure.HasRead(p.File) {
			closure.ClearPendingReadFor(p.File)
			continue
		}
		if !closure.IsScanned(p.File) {
			logging.Warning("[CGEC] D4 runForcedReads: attempting read of file=%s origin=%s NOT in ScannedSet — may be ghost path", p.File, p.Origin)
		}
		toRead = append(toRead, p)
		if len(toRead) >= limit {
			break
		}
	}
	if len(toRead) == 0 {
		return 0
	}

	rf := &tool.ReadFile{}
	success := 0
	successByStage := map[string]int{}
	for _, p := range toRead {
		// Surgical multi-range path. When the demanding gate populated
		// PendingRead.LineRanges (e.g. multi-path symbol-anchored
		// gate's ActionDemandSurgicalRead), issue ONE read_file call
		// per [Start, End] range so the LLM only sees the slivers
		// the gate identified. Without this, recovery on a 4 000-line
		// file with three 30-line missing slivers would pull in the
		// whole file (defeating the whole "surgical, no noise"
		// rewrite from 2026-05-01). Empty LineRanges falls back to
		// the historical full-file read so older PendingRead emitters
		// (chain promotion, grounder reject, phase1_unread,
		// primary_anchor) keep working byte-identically.
		var perRangeResults []types.ToolResult
		var firstErr error
		var lastSummary string
		if len(p.LineRanges) > 0 {
			for _, r := range p.LineRanges {
				if r.End < r.Start || r.Start <= 0 {
					continue
				}
				// Cancellation checkpoint: a wide demand can produce
				// dozens of chunked reads, each ~5-10ms. Without this
				// check, /cancel during runForcedReads waits for the
				// whole loop to drain (~1s worst case for 90 reads).
				// Bail out promptly so the dock returns control to
				// the operator instead of stalling on synthesised IO.
				if forcedReadCancelled(o.busCtx) {
					return success
				}
				// Chunk the demand to stay under read_file's inline
				// MaxInlineBytes budget. Without chunking, a long-line
				// file or a wide L4 SmallFileFull demand would trip
				// read_file's blob-truncation path (banner reports a
				// shorter range than requested), the closure would
				// record only the truncated range, the gate would
				// re-raise the same demand on the next pass, and
				// runForcedReads would issue the same truncated read
				// again — fingerprint stable, I4 force-complete after
				// 3 wasted rounds. Chunking guarantees each individual
				// read fits inline so the closure records the full
				// demanded range across one stall round.
				chunkBudget := surgicalReadChunkLines
				cur := r
				for cur.End >= cur.Start {
					end := cur.End
					if end-cur.Start+1 > chunkBudget {
						end = cur.Start + chunkBudget - 1
					}
					offset := cur.Start - 1
					limit := end - cur.Start + 1
					rangeResult, rangeErr := executeSurgicalReadFile(o.busCtx, rf, p.File, offset, limit)
					if rangeErr != nil || !rangeResult.Success {
						if firstErr == nil {
							firstErr = rangeErr
						}
						lastSummary = rangeResult.Summary
						logging.Warning("[CGEC] E2 surgical forced-read chunk failed: file=%s offset=%d limit=%d err=%v summary=%s",
							p.File, offset, limit, rangeErr, rangeResult.Summary)
						break
					}
					rangeResult.Summary = "[forced_read surgical] " + rangeResult.Summary
					perRangeResults = append(perRangeResults, rangeResult)
					if end >= cur.End {
						break
					}
					cur = types.LineRange{Start: end + 1, End: cur.End}
				}
			}
			if len(perRangeResults) == 0 {
				if cause, abandoned := abandonUnrecoverableForcedRead(o.busCtx, closure, p.File, p.Origin, firstErr, lastSummary, "surgical"); abandoned {
					o.emitAbandonEvent(p.File, cause)
				}
				continue
			}
		} else {
			// Historical full-file fallback for PendingRead emitters
			// that did not populate LineRanges. Try the path verbatim
			// first; fall back to RepoRoot-joined if the verbatim
			// read fails (most repos run codrax from the repo root so
			// the relative path resolves; the join is the
			// belt-and-suspenders).
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
				if cause, abandoned := abandonUnrecoverableForcedRead(o.busCtx, closure, p.File, p.Origin, err, result.Summary, "full"); abandoned {
					o.emitAbandonEvent(p.File, cause)
				}
				continue
			}
			result.Summary = "[forced_read] " + result.Summary
			perRangeResults = []types.ToolResult{result}
		}

		// Hook every produced ToolResult into the same channels real
		// reads flow through. For surgical multi-range we append one
		// ToolResult per range so extractFileCoverage's banner walker
		// records each range independently — the closure's
		// MissingContextRegions then sees the union and the next gate
		// pass clears.
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
		for _, result := range perRangeResults {
			o.busCtx.Mutable.AppendDispatchToolResult(result)
			o.busCtx.ToolResults = append(o.busCtx.ToolResults, result)
			artifacts.ToolResults = append(artifacts.ToolResults, result)
		}
		o.busCtx.Mutable.SetTurnAArtifacts(*artifacts)

		// 2026-05-10 path-canonicalization audit: p.File is canonical
		// (addPendingReadLocked normalises at insert) so the lookup
		// key matches readSet's canonical-only contract.
		readSet[p.File] = true
		closure.ClearPendingReadFor(p.File)
		success++
		// A.1+E.1: per-stage attribution so PerStage[Stage].ForcedReads
		// is accurate without snapshot-time derivation. Empty p.Stage
		// (legacy emitters) flows into successByStage[""] and is folded
		// into the global counter alone via BumpForcedReads below.
		successByStage[p.Stage]++
		if len(p.LineRanges) > 0 {
			logging.Info("[CGEC] E2 surgical forced-read: file=%s origin=%s ranges=%d successful_reads=%d",
				p.File, p.Origin, len(p.LineRanges), len(perRangeResults))
		} else {
			logging.Info("[CGEC] E2 forced-read: file=%s origin=%s rationale=%s", p.File, p.Origin, p.Rationale)
		}
	}
	if success > 0 {
		closure.SetReadSet(readSet)
		// A.1+E.1: when any read carried explicit Stage attribution use
		// the per-stage bumper for those reads; legacy reads (Stage="")
		// fall back to the un-attributed global bumper. The total stays
		// consistent: sum of stage-attributed bumps + the empty-stage
		// global bump = success.
		legacyCount := 0
		for stage, n := range successByStage {
			if stage == "" {
				legacyCount += n
				continue
			}
			closure.BumpForcedReadsForStage(stage, n)
		}
		if legacyCount > 0 {
			closure.BumpForcedReads(legacyCount)
		}
		o.emit(render.Event{
			Kind:       render.EventOrchestratorNotice,
			Timestamp:  time.Now(),
			Agent:      "orchestrator",
			NoticeKind: render.NoticeForcedRead,
			Reasoning:  softForcedReadMessage(o.busCtx.Language, success),
		})
	}
	return success
}

func (o *Orchestrator) seedRequiredFileHintForcedReadsBeforeExplore() int {
	if o == nil || o.busCtx == nil || o.busCtx.AnalysisIR == nil || o.busCtx.Mutable == nil {
		return 0
	}
	rm := o.busCtx.AnalysisIR.RequestModel
	if !types.RequiredFileHintCurrentSourceCoverageApplies(rm) {
		return 0
	}
	closure := o.busCtx.Mutable.EvidenceClosure()
	if closure == nil {
		return 0
	}
	queued := 0
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		if hint.Confidence < 0.8 {
			continue
		}
		file, ok := types.QualifyForcedReadSeedPath(o.busCtx, hint.Path)
		if !ok {
			logging.Warning("[CGEC] pre-dispatch required_file_hint_unread: skipping phantom path file=%s",
				types.CanonicalRequiredFileHintPath(hint.Path, o.busCtx.RepoRoot))
			continue
		}
		if file == "" || closure.HasRead(file) || pendingReadQueuedForFile(closure, file) {
			continue
		}
		closure.AddPendingRead(types.PendingRead{
			File:      file,
			Rationale: "High-confidence current-source file from request analysis should be read before the first explore completion attempt",
			Origin:    "pre_dispatch.required_file_hint_unread",
			Stage:     string(types.StageExplore),
		})
		queued++
		logging.Info("[CGEC] pre-dispatch required_file_hint_unread: queued forced-read file=%s", file)
		if queued >= types.RequiredFileHintCoverageMax {
			break
		}
	}
	return queued
}

func pendingReadQueuedForFile(closure *types.EvidenceClosure, file string) bool {
	if closure == nil || file == "" {
		return false
	}
	for _, pending := range closure.PendingReads() {
		if pending.File == file {
			return true
		}
	}
	return false
}

func hasPreDispatchRequiredFilePendingRead(closure *types.EvidenceClosure) bool {
	if closure == nil {
		return false
	}
	for _, pending := range closure.PendingReads() {
		if pending.Origin == "pre_dispatch.required_file_hint_unread" {
			return true
		}
	}
	return false
}

// abandonUnrecoverableForcedRead drops a PendingRead the framework
// cannot read (typically because the path does not exist, the
// process lacks permission, the path is a directory / broken
// symlink, etc.) and replaces it with a non-blocking
// RepairExpandSearch advisory so the LLM is told the framework gave
// up but the gate still sees the demand absorbed (no infinite drain
// → re-raise loop burning the stall budget).
//
// Detection runs an os.Stat probe on both the verbatim path AND the
// RepoRoot-joined path so the diagnosis is independent of how the
// read_file tool wrapped the underlying OS error (it returns
// Success=false + a string-ified Summary with err nil-ed out, so
// the original *os.PathError is not available to classify on).
// Transient errors (the file genuinely exists + is readable but the
// read raced) deliberately fall through to the legacy "leave the
// PendingRead in place; let the I4 stall detector force-complete
// after K identical fingerprints" path so flaky filesystems don't
// lose unique demands on the first failure.
//
// Cross-platform: os.IsNotExist / os.IsPermission classify both
// POSIX errnos (ENOENT / EACCES on Linux + macOS) AND Windows
// errors (ERROR_FILE_NOT_FOUND / ERROR_PATH_NOT_FOUND /
// ERROR_ACCESS_DENIED) via the runtime's per-OS adapter — no
// build-tag branching needed in this code. IsDir() detects "path
// is a directory" uniformly on every OS.
func abandonUnrecoverableForcedRead(busCtx *types.BusContext, closure *types.EvidenceClosure, file, origin string, ioErr error, summary, mode string) (cause string, abandoned bool) {
	verdict, statErr, isDir := classifyForcedReadFailure(busCtx, file, ioErr)
	if !verdict {
		// Transient failure — keep the PendingRead, the next
		// runForcedReads invocation will retry.
		return "", false
	}
	cause = summarizeReadFailure(statErr, isDir, ioErr, summary)
	closure.ClearPendingReadFor(file)
	closure.AddRepair(types.RepairDirective{
		Kind:      types.RepairExpandSearch,
		Files:     []string{file},
		Rationale: fmt.Sprintf("framework cannot read primary anchor `%s` (%s mode): %s. Either grep for the question entities elsewhere if this anchor is essential, or judge it unrelated to the question and proceed with completion.", file, mode, cause),
		Origin:    "forced_read.unrecoverable." + origin,
	})
	logging.Error("[CGEC] E2 forced-read abandoned (unrecoverable) file=%s mode=%s origin=%s cause=%s ioErr=%v", file, mode, origin, cause, ioErr)
	return cause, true
}

// emitAbandonEvent emits the user-visible "skipping unreadable
// file" line on the orchestrator's render channel. Split out from
// abandonUnrecoverableForcedRead so the orchestrator-bound emitter
// can be looked up at the call site (closure-bound abandon helper
// has no o.* receiver). Caller passes the resolved cause so the
// operator-visible line and the LLM-facing rationale agree.
func (o *Orchestrator) emitAbandonEvent(file, cause string) {
	if o == nil || o.emit == nil {
		return
	}
	lang := ""
	if o.busCtx != nil {
		lang = o.busCtx.Language
	}
	o.emit(render.Event{
		Kind:       render.EventOrchestratorNotice,
		Timestamp:  time.Now(),
		Agent:      "orchestrator",
		NoticeKind: render.NoticeAbandonRead,
		Reasoning:  abandonForcedReadMessage(lang, file, cause),
	})
}

// classifyForcedReadFailure runs cross-platform readability probes
// and returns (unrecoverable, probeErr, isDir) so the caller can
// both decide WHETHER to abandon and produce a concrete cause for
// the LLM-facing rationale.
//
// Probe shape per path: Stat first (cheap, returns IsDir + the
// inode-level errors ENOENT / EACCES on dir lookup), then Open
// (catches "file exists in inode table but process lacks read
// permission" — the chmod 000 case where Stat succeeds because
// the parent dir is readable but the file itself is unreadable).
// File descriptors from Open are closed immediately.
//
// Path resolution mirrors read_file's own chain: try verbatim
// first, then RepoRoot-joined. Both probes must agree the path is
// structurally bad before we abandon — a single transient probe
// failure does not lose the PendingRead.
//
// Cross-platform notes:
//   - Linux:   os.IsNotExist matches ENOENT, os.IsPermission matches
//     EACCES / EPERM
//   - macOS:   POSIX-equivalent errnos; behaves identically
//   - Windows: ERROR_FILE_NOT_FOUND / ERROR_PATH_NOT_FOUND map to
//     IsNotExist; ERROR_ACCESS_DENIED maps to IsPermission;
//     os.FileInfo.IsDir works on NTFS / ReFS uniformly
//
// Returns the FIRST informative probe error for rationale
// (typically the verbatim probe's; falls back to the joined probe
// if verbatim succeeded but the joined didn't).
func classifyForcedReadFailure(busCtx *types.BusContext, file string, ioErr error) (unrecoverable bool, probeErr error, isDir bool) {
	if file == "" {
		return true, fmt.Errorf("empty file path"), false
	}
	verbatimVerdict, verbatimProbeErr, verbatimIsDir := probeReadability(file)
	if verbatimIsDir {
		return true, verbatimProbeErr, true
	}
	if verbatimVerdict == probeReadable {
		// Verbatim path is genuinely readable. ioErr was transient
		// (read raced with delete-then-restore, NFS hiccup). Keep
		// the PendingRead.
		return false, nil, false
	}
	// Verbatim probe says unreadable for one reason or another.
	// Cross-check the joined fallback before abandoning so a
	// misconfigured RepoRoot+repo-relative-path combo doesn't lose
	// PendingReads when the joined path actually resolves.
	if busCtx != nil && busCtx.RepoRoot != "" {
		joined := filepath.Join(busCtx.RepoRoot, file)
		joinedVerdict, joinedProbeErr, joinedIsDir := probeReadability(joined)
		if joinedIsDir {
			return true, joinedProbeErr, true
		}
		if joinedVerdict == probeReadable {
			// Joined fallback works. The tool's own resolution chain
			// would have succeeded — ioErr is transient.
			return false, nil, false
		}
		// Both unreadable. Pick the most informative error:
		// EACCES > IsDir-on-the-other-probe > ENOENT > generic.
		// Permission-denied is more diagnostic than not-found
		// because the LLM learns "file exists but unreadable" vs
		// "file does not exist" — actionable distinction.
		chosen := pickMostInformativeError(verbatimProbeErr, joinedProbeErr)
		if verbatimVerdict == probeStructurallyBad || joinedVerdict == probeStructurallyBad {
			return true, chosen, false
		}
		// Both probes returned an unclassified error (could be NFS
		// blip on both). Be conservative — don't abandon.
		return false, chosen, false
	}
	// No RepoRoot fallback available. Verbatim verdict is final.
	if verbatimVerdict == probeStructurallyBad {
		return true, verbatimProbeErr, false
	}
	return false, verbatimProbeErr, false
}

// probeVerdict labels the outcome of a readability probe.
type probeVerdict int

const (
	probeReadable        probeVerdict = iota // file exists, regular file, opens for read
	probeStructurallyBad                     // ENOENT / EACCES / EISDIR (handled separately) — won't recover on retry
	probeUnclassified                        // transient or unknown error — caller should not abandon
)

// pickMostInformativeError ranks two probe errors by diagnostic
// value for the LLM-facing rationale. Permission-denied beats
// not-found beats anything else, because "file exists but
// unreadable" is more actionable than "file does not exist" — the
// LLM learns to grep elsewhere or judge the anchor unrelated.
//
// Each input may be nil (one probe succeeded structurally but the
// outer classifier still picked a "both unreadable" branch via
// other state); nil ranks lowest.
func pickMostInformativeError(a, b error) error {
	score := func(e error) int {
		if e == nil {
			return 0
		}
		if os.IsPermission(e) {
			return 3
		}
		if os.IsNotExist(e) {
			return 2
		}
		return 1
	}
	if score(a) >= score(b) {
		return a
	}
	return b
}

// probeReadability runs the cross-platform Stat + Open probe pair
// for a single path. Returns the verdict, the most informative
// error for diagnosis, and an isDir flag.
//
// Open is needed in addition to Stat because chmod 000 (Linux/macOS)
// or NTFS ACL deny-read (Windows) leaves Stat able to read the
// inode metadata via a readable parent directory while os.ReadFile
// fails with permission denied. Stat alone would falsely classify
// such a file as "readable" and runForcedReads would loop.
func probeReadability(path string) (probeVerdict, error, bool) {
	info, statErr := os.Stat(path)
	if info != nil && info.IsDir() {
		return probeStructurallyBad, statErr, true
	}
	if statErr != nil {
		if os.IsNotExist(statErr) || os.IsPermission(statErr) {
			return probeStructurallyBad, statErr, false
		}
		return probeUnclassified, statErr, false
	}
	// Stat says regular file exists. Confirm read access via Open.
	f, openErr := os.Open(path)
	if openErr == nil {
		_ = f.Close()
		return probeReadable, nil, false
	}
	if os.IsNotExist(openErr) || os.IsPermission(openErr) {
		return probeStructurallyBad, openErr, false
	}
	return probeUnclassified, openErr, false
}

// isStructurallyUnrecoverableReadFailure is the boolean wrapper
// kept for the existing test surface that calls it directly.
// Internally delegates to classifyForcedReadFailure.
func isStructurallyUnrecoverableReadFailure(busCtx *types.BusContext, file string, ioErr error) bool {
	verdict, _, _ := classifyForcedReadFailure(busCtx, file, ioErr)
	return verdict
}

// summarizeReadFailure renders a one-line LLM-facing description of
// the failure for the advisory rationale. Prefers the stat-probe
// error (statErr) over the read_file tool's wrapped err (ioErr),
// because read_file converts every os.ReadFile failure into a
// string-ified Summary with err nil-ed out, losing the *os.PathError
// the os.IsNotExist / os.IsPermission classifiers operate on.
//
// Cross-platform classification uses os.IsNotExist /
// os.IsPermission which the Go runtime adapts per-OS:
//   - Linux:   ENOENT / EACCES / EPERM
//   - macOS:   ENOENT / EACCES / EPERM (POSIX-equivalent)
//   - Windows: ERROR_FILE_NOT_FOUND / ERROR_PATH_NOT_FOUND /
//     ERROR_ACCESS_DENIED
//
// Falls back to a generic message + the tool's wrapped summary
// (with the "read failed: " prefix stripped) when we can't classify.
func summarizeReadFailure(statErr error, isDir bool, ioErr error, summary string) string {
	if isDir {
		return "path is a directory, not a file"
	}
	for _, candidate := range []error{statErr, ioErr} {
		if candidate == nil {
			continue
		}
		switch {
		case os.IsNotExist(candidate):
			return "file not found"
		case os.IsPermission(candidate):
			return "permission denied"
		}
	}
	if ioErr != nil {
		return strings.TrimSpace(ioErr.Error())
	}
	if statErr != nil {
		return strings.TrimSpace(statErr.Error())
	}
	if s := strings.TrimSpace(summary); s != "" {
		// Tool error summaries already start with "read failed: ..."
		// — surface just the cause portion for the LLM.
		if idx := strings.Index(s, ": "); idx > 0 && idx < len(s)-2 {
			return s[idx+2:]
		}
		return s
	}
	return "framework read failed for an unknown reason"
}

// forcedReadCancelled reports whether the orchestrator's cancel
// token has fired so the surgical multi-range loop bails out
// promptly. Nil-safe on test fixtures that omit ctx.
func forcedReadCancelled(busCtx *types.BusContext) bool {
	if busCtx == nil {
		return false
	}
	ctx := busCtx.Context()
	if ctx == nil {
		return false
	}
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// executeSurgicalReadFile runs the read_file tool with explicit
// offset/limit, mirroring the verbatim → repoRoot-joined fallback
// the full-file path uses. Centralised so the surgical multi-range
// loop stays compact and the failure-mode handling matches the
// historical behaviour.
func executeSurgicalReadFile(busCtx *types.BusContext, rf *tool.ReadFile, file string, offset, limit int) (types.ToolResult, error) {
	params, _ := json.Marshal(map[string]any{
		"path":   file,
		"offset": offset,
		"limit":  limit,
	})
	result, err := rf.Execute(busCtx, params)
	if err == nil && result.Success {
		return result, nil
	}
	if busCtx == nil || busCtx.RepoRoot == "" {
		return result, err
	}
	joinedParams, _ := json.Marshal(map[string]any{
		"path":   filepath.Join(busCtx.RepoRoot, file),
		"offset": offset,
		"limit":  limit,
	})
	return rf.Execute(busCtx, joinedParams)
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
	logging.Warning("[CGEC] I4 stall soft: round=%d threshold=%d", len(hist), cgecStallThresholdSoft)
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
	// CGEC B1b: soft stall with NO files to force-read means the
	// ReadSet is saturated (or PendingReads is empty) but evidence /
	// chain terminals / citations are not growing — the LLM is
	// treading water on the same set of files. Emit
	// RepairExpandSearch so the next retry's prompt tells the LLM to
	// broaden its grep coverage instead of re-reading the same
	// files. Keywords empty → Render falls back to a generic
	// rationale; downstream Group B3 extractor can still surface the
	// prompt section.
	var kws []string
	if o.busCtx != nil && o.busCtx.AnalysisIR != nil {
		kws = append(kws, o.busCtx.AnalysisIR.RequestModel.AnalyzerHints.Keywords...)
	}
	closure.AddRepair(types.RepairDirective{
		Kind:      types.RepairExpandSearch,
		Keywords:  kws,
		Rationale: fmt.Sprintf("ReadSet saturated but %d consecutive rounds produced no new evidence/chain/citation — broaden grep coverage (stems / conceptual synonyms / sibling packages) instead of re-reading the same files", cgecStallThresholdSoft),
		Origin:    "convergence_detector.soft_stall",
	})
	logging.Info("[CGEC] B1b expand_search: origin=convergence_detector.soft_stall keywords=%d", len(kws))
	// Hard threshold: cgecStallThresholdHard consecutive identical
	// fingerprints AND no forced read fired. Threshold = 0 disables
	// the hard exit (operators that want soft observability without
	// force-complete set hard=0).
	if cgecStallThresholdHard > 0 && len(hist) >= cgecStallThresholdHard {
		if lastNFingerprintsEqual(hist, cgecStallThresholdHard) {
			logging.Error("[CGEC] I4 stall hard: force-completing investigation to ship best-effort answer")
			closure.BumpStallHardHits(1)
			closure.AddRepair(types.RepairDirective{
				Kind:      types.RepairForceCompleteDowngrade,
				Rationale: fmt.Sprintf("no progress detected across %d consecutive explore rounds", cgecStallThresholdHard),
				Origin:    "convergence_detector",
			})
			o.busCtx.Mutable.SetInvestigationComplete(fmt.Sprintf("convergence stall: %d consecutive explore rounds with no investigation progress", cgecStallThresholdHard))
			o.emit(render.Event{
				Kind:       render.EventOrchestratorNotice,
				Timestamp:  time.Now(),
				Agent:      "orchestrator",
				NoticeKind: render.NoticeConvergenceStall,
				Reasoning:  softConvergenceStallMessage(o.busCtx.Language),
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
