package orchestrator

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
	answertool "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type finalizeVisibleCarrierSnapshot struct {
	Document    *types.AnswerDocumentV2         `json:"document"`
	Attachments []types.AnswerDisplayAttachment `json:"attachments,omitempty"`
}

// finalizeRepairVisibleCarrierSnapshot captures the model-visible structured
// carrier without the later string-channel caveats. It is used only to avoid
// attaching the same first draft below itself after FRCAP restored that exact
// carrier and then appended a residual system note. Different documents or
// recovered model attachments remain distinct and are never suppressed.
func finalizeRepairVisibleCarrierSnapshot(mut *types.MutableState) []byte {
	if mut == nil {
		return nil
	}
	doc := mut.AnswerDocumentV2()
	attachments := mut.AnswerDisplayAttachments()
	if doc == nil && len(attachments) == 0 {
		return nil
	}
	raw, err := json.Marshal(finalizeVisibleCarrierSnapshot{
		Document:    doc,
		Attachments: attachments,
	})
	if err != nil {
		return nil
	}
	return raw
}

func finalizeRepairVisibleCarrierMatches(snapshot []byte, mut *types.MutableState) bool {
	return len(snapshot) > 0 && bytes.Equal(snapshot, finalizeRepairVisibleCarrierSnapshot(mut))
}

// finalize_repair_draft_ledger.go — FRCAP (§29.12,
// docs/design/real_trace_campaign_20260705.md, 2026-07-10).
//
// The finalize repair loop re-dispatches the finalizer LLM when the
// contract check fails; small models are unstable across rounds, so a
// LATER draft is not necessarily a BETTER draft. Pre-FRCAP, reaching
// the P6 hard cap shipped the CURRENT (latest) draft with a residual-
// concerns caveat. FRCAP retains every failing round's draft in this
// ledger and, at the cap, ships the draft with the FEWEST hard
// violations — ties go to the EARLIEST draft (user ruling: later
// rounds may be worse). Selection reads a PRECISE signal only: the
// typed actionable-root violation count of each round's contract
// result; no prose heuristics.
//
// Memory bound: drafts are recorded only on contract-failure rounds,
// and the P6 hard cap pre-empts dispatch once state.retryUsed reaches
// the cap, so the ledger holds at most cap+1 entries by construction;
// recordFinalizeRepairDraft enforces the same bound defensively.
//
// The ledger lives in scheduler-loop local state (one per task, like
// firstFinalizeDraft) — never on MutableState — so multi-task runs
// cannot drag drafts across a task boundary.

// finalizeRepairRoundsTelemetryFormat is the per-answer repair-budget
// telemetry line (§29.12 item 4). Format pinned by
// TestFinalizeRepairRoundsTelemetryFormatPinned — revisit transcripts
// grep for the literal "finalize repair budget used=".
//
// Wording (adversarial review P3c, 2026-07-10): the counter is
// state.retryUsed — the CROSS-STAGE scheduler retry budget the P6 cap
// compares (it also counts explore fact-retry, Tier-1 floor and SC
// requeues), so the label says "budget … (cross-stage)" instead of
// overclaiming a finalize-only round count. A finalize-only sub-counter
// is a ruling candidate, NOT changed here (cap semantics untouched).
const finalizeRepairRoundsTelemetryFormat = "[orchestrator] finalize repair budget used=%d cap=%d (cross-stage)"

// logFinalizeRepairRoundsTelemetry emits the §29.12 per-answer
// repair-budget audit line. Called once on every answer-shipping exit
// of the read scheduler loop; N is the exact counter the P6 hard cap
// compares (state.retryUsed), M the effective cap.
func (o *Orchestrator) logFinalizeRepairRoundsTelemetry(state *graphState) {
	if o == nil || state == nil {
		return
	}
	logging.Info(finalizeRepairRoundsTelemetryFormat, state.retryUsed, o.finalizeRepairHardCapValue())
}

// finalizeRepairDraftRecord is one retained failing draft: the
// rendered answer, the structured document (JSON deep copy, so later
// rounds cannot mutate it), and the PRECISE typed violation ledger of
// its own contract round.
type finalizeRepairDraftRecord struct {
	Round          int    // state.retryUsed when the draft failed its check
	Answer         string // rendered FinalAnswer at that round (pre-caveat)
	DocJSON        []byte // marshaled AnswerDocumentV2; nil when no doc
	HardViolations int    // len(actionable root retry violations) — precise signal
	Violations     []types.Violation
	Hash           string // sha256[:12] of Answer, for the audit log
	// SystemBlockKinds is the id → SystemGeneratedKind authority sidecar
	// captured from the SAME in-memory document DocJSON snapshots, at the
	// same moment (marker-stripping class root fix, audit 2026-07-10).
	// The marker is json:"-", so DocJSON alone loses it; restore
	// re-stamps via ReauthenticateSystemSnapshotBlockKinds so the
	// FRCAP-selected draft keeps its `##` runtime-trace chapters and its
	// system numerals stay on the prose-scalar evidence face. In-memory
	// only — the record never crosses a JSON boundary.
	SystemBlockKinds map[string]types.AnswerSystemGeneratedBlockKind
}

// recordFinalizeRepairDraft appends the current failing draft to the
// ledger. hardCap bounds the ledger size (cap+1 entries); a non-
// positive hardCap keeps the defensive default bound.
func recordFinalizeRepairDraft(ledger []finalizeRepairDraftRecord, out *agent.StageOutput, mut *types.MutableState, retryViolations []types.Violation, round, hardCap int) []finalizeRepairDraftRecord {
	if out == nil || strings.TrimSpace(out.FinalAnswer) == "" {
		return ledger
	}
	bound := hardCap + 1
	if hardCap <= 0 {
		bound = FinalizeRepairHardCapDefault + 1
	}
	if len(ledger) >= bound {
		return ledger
	}
	rec := finalizeRepairDraftRecord{
		Round:          round,
		Answer:         out.FinalAnswer,
		HardViolations: len(retryViolations),
		Violations:     append([]types.Violation(nil), retryViolations...),
		Hash:           finalizeRepairDraftHash(out.FinalAnswer),
	}
	if mut != nil {
		if doc := mut.AnswerDocumentV2(); doc != nil && len(doc.Blocks) > 0 {
			if raw, err := json.Marshal(doc); err == nil {
				rec.DocJSON = raw
				// Capture the json:"-" authority markers the marshal
				// above just stripped (see SystemBlockKinds).
				rec.SystemBlockKinds = types.CaptureSystemGeneratedBlockKinds(doc)
			}
		}
	}
	return append(ledger, rec)
}

func finalizeRepairDraftHash(answer string) string {
	sum := sha256.Sum256([]byte(answer))
	return hex.EncodeToString(sum[:])[:12]
}

// selectBestFinalizeRepairDraft returns the index of the ledger draft
// with the FEWEST hard violations; ties select the EARLIEST draft
// (§29.12 item 3 — later rounds are not presumed better). Returns -1
// on an empty ledger. Strict-less-than comparison over an in-order
// walk implements "tie → earliest" with zero extra state.
func selectBestFinalizeRepairDraft(ledger []finalizeRepairDraftRecord) int {
	best := -1
	for i := range ledger {
		if best < 0 || ledger[i].HardViolations < ledger[best].HardViolations {
			best = i
		}
	}
	return best
}

// restoreFinalizeRepairDraft swaps the shipping StageOutput to the
// selected earlier draft: the structured document is restored into
// MutableState (so every downstream re-render — first-draft
// attachment, answer reviewer, result recording — reads the SAME
// draft) and the customer answer is re-rendered through the TRUNC
// last-mile chokepoint so the deterministic 系统补充 blocks survive
// the swap. When the document cannot be decoded, the recorded
// rendered answer string is used verbatim (it was produced by the
// same chokepoint at its own round). Returns false when nothing was
// swapped (defensive inputs only — callers pre-check best != last).
func (o *Orchestrator) restoreFinalizeRepairDraft(out *agent.StageOutput, rec *finalizeRepairDraftRecord) bool {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || out == nil || rec == nil {
		return false
	}
	if strings.TrimSpace(rec.Answer) == "" {
		return false
	}
	restored := false
	if len(rec.DocJSON) > 0 {
		var doc types.AnswerDocumentV2
		if err := json.Unmarshal(rec.DocJSON, &doc); err == nil && len(doc.Blocks) > 0 {
			// Re-authenticate the snapshot (marker-stripping class root
			// fix, audit 2026-07-10): DocJSON lost every json:"-"
			// SystemGeneratedKind marker at record time; re-stamp from
			// the sidecar captured off the SAME in-memory document, so
			// the restored draft's genuine system blocks keep chapter
			// promotion, dedupe/idempotence identity, and the PSG
			// evidence face. System-side snapshot only — never applied
			// to model-emitted JSON.
			types.ReauthenticateSystemSnapshotBlockKinds(&doc, rec.SystemBlockKinds)
			o.busCtx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &doc)
			attachments := answertool.FilterAcceptedAnswerDisplayAttachments(&doc, o.busCtx.Mutable.AnswerDisplayAttachments())
			o.busCtx.Mutable.SetAnswerDisplayAttachments(attachments)
			render.ApplyAuthorityHedging(&doc, finalizerAutoRepairAuthorityEvidencePool(o.busCtx), o.busCtx.Language)
			if answer := o.renderFinalAnswerWithLastMileSupplements(&doc, attachments); strings.TrimSpace(answer) != "" {
				out.FinalAnswer = answer
				restored = true
			}
		}
	}
	if !restored {
		out.FinalAnswer = rec.Answer
	}
	out.Data = marshalFinalizerAutoRepairStageData(out.FinalAnswer)
	return true
}
