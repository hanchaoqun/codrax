package types

import (
	"sort"
	"strings"
)

// RepairKind enumerates the structured repair categories the four
// CGEC enforcers can raise. Each kind maps to a distinct retry-hint
// section the orchestrator renders into the next explore-window
// prompt; the explorer then knows EXACTLY what to do (read these
// files, swap to that QuestionFamily, narrow to that subject)
// instead of re-deriving the requirement from a free-form violation
// message.
//
// Why an enum and not a free-form string: the previous retry hint
// was the human-readable contract violation ("0 citations < 1") and
// the explorer had no structured way to know that the underlying
// fix was "read the file the previous citation pointed at". A
// typed kind lets the renderer (scheduler.renderWindowHint) and
// the framework-side enforcers (Lazy Auto-Read in runTaskGraph)
// dispatch on the kind without parsing prose.
type RepairKind string

const (
	// RepairReadFile says the next explore round MUST cover the
	// listed files (via read_file). Files set; Subject ignored.
	// Origin describes which enforcer raised it (e.g. "grounder",
	// "chain_promotion", "stall_detector").
	RepairReadFile RepairKind = "read_file"

	// RepairEmitEvidence says the required file scope has already
	// been read, but the investigation still lacks a properly
	// grounded / structured evidence batch. Files set; Subject
	// ignored. The explorer should stay on these already-read
	// anchors, emit corrected grounded evidence, and retry
	// completion instead of widening scope with more navigation.
	RepairEmitEvidence RepairKind = "emit_evidence"

	// RepairExpandSearch says the explorer should broaden its
	// keyword grep coverage. Keywords set; Files ignored. Used by
	// the stall detector when ReadSet is full but evidence count
	// has not grown.
	RepairExpandSearch RepairKind = "expand_search"

	// RepairSwapView says the AnalysisIR's compiled QuestionFamily /
	// AnswerSemanticView routing under-fits the resolved answer
	// subject and the explorer should surface the conflict. Subject
	// = "from=<family>,to=<family>" string; the renderer surfaces
	// this as a View Reconcile section. (Pre-PR3 this was named
	// "swap_shape"; the rename mirrors the AnswerShape →
	// AnswerSemanticView migration — the underlying intent has
	// always been "the family routing is wrong", never "the shape
	// enum is wrong".)
	RepairSwapView RepairKind = "swap_view"

	// RepairRebindSubject says the chain ranker found a subject /
	// terminal mismatch and the explorer should constrain its
	// answer to a specific kind. Subject = AnswerSubjectKind value.
	RepairRebindSubject RepairKind = "rebind_subject"

	// RepairForceCompleteDowngrade tells the orchestrator to mark
	// the in-flight investigation complete with whatever evidence
	// has accumulated. Raised by the stall detector after three
	// fingerprint-identical rounds; the explorer's loop is forced
	// to exit so the user gets a best-effort answer rather than
	// "answer-contract validation exhausted".
	RepairForceCompleteDowngrade RepairKind = "force_complete_downgrade"
)

// AllRepairKinds returns every legal RepairKind in declaration order.
// Used by tests that assert no enforcer raises an unknown kind.
func AllRepairKinds() []RepairKind {
	return []RepairKind{
		RepairReadFile,
		RepairEmitEvidence,
		RepairExpandSearch,
		RepairSwapView,
		RepairRebindSubject,
		RepairForceCompleteDowngrade,
	}
}

// IsValid returns true when k is one of the declared constants.
func (k RepairKind) IsValid() bool {
	switch k {
	case RepairReadFile, RepairEmitEvidence, RepairExpandSearch, RepairSwapView,
		RepairRebindSubject, RepairForceCompleteDowngrade:
		return true
	}
	return false
}

// RepairDirective is the structured payload one enforcer hands the
// orchestrator. The renderer (scheduler.renderWindowHint) consumes
// it via EvidenceClosure.ConsumeRepairs and turns each directive
// into a section of the next explore-window prompt.
//
// Fields are kind-conditional: read the Kind, then read whichever
// of Files / Keywords / Subject the kind populates. Unset fields
// for a given kind are ignored. Rationale is always the prose the
// LLM sees ("citation cited X:Y but file is not in Turn A's
// ReadSet — read it before next emit_investigation_complete");
// Origin is short tag for attribution / debugging.
type RepairDirective struct {
	Kind      RepairKind
	Files     []string
	Keywords  []string
	Subject   string
	Rationale string
	Origin    string

	// LineRanges (Kind == RepairReadFile only) carries surgical
	// 1-based [Start, End] line ranges. When non-empty AND propagated
	// through the AddRepair → addPendingReadLocked auto-bridge, the
	// resulting PendingRead carries the surgical demand into
	// runForcedReads (CGEC E2 Lazy Auto-Read) for per-range read_file
	// recovery. Without this field, any future enforcer that wants to
	// emit surgical demand via AddRepair would silently fall back to
	// the historical full-file read, defeating the multipath gate's
	// "surgical, no noise" rewrite.
	//
	// nil / empty preserves the historical full-file fallback so old
	// callers (chain promotion / grounder reject) keep working
	// byte-identically.
	LineRanges []LineRange

	// Stage (A.1+E.1, 2026-05-02) is the explicit stage attribution
	// for this directive. When non-empty, AddRepair bumps
	// stats.PerStage[Stage].Repairs and propagates Stage into the
	// auto-bridged PendingRead so StageHealthSnapshot no longer needs
	// to derive from Origin. Empty Stage preserves byte-identical
	// legacy behaviour for un-migrated callers.
	Stage string
}

// MergeRepairs deduplicates a slice of RepairDirective by Kind +
// sorted-Files + Subject. The renderer calls this before formatting
// so two enforcers raising the same directive surface only once. The
// caller-supplied order is preserved for the kept entries.
func MergeRepairs(in []RepairDirective) []RepairDirective {
	if len(in) <= 1 {
		return in
	}
	type key struct {
		kind    RepairKind
		subject string
		files   string
	}
	seen := make(map[key]bool, len(in))
	out := in[:0]
	for _, r := range in {
		dup := append([]string(nil), r.Files...)
		sort.Strings(dup)
		k := key{kind: r.Kind, subject: r.Subject, files: strings.Join(dup, "\x00")}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, r)
	}
	return out
}

// Render returns the human-readable retry-hint markdown for one
// directive. Pure function; safe to call from any layer. The
// scheduler concatenates the per-directive sections into the full
// hint, separated by blank lines.
//
// Format per kind:
//
//	read_file:
//	  ## Forced Read List (read these files before emit_investigation_complete)
//	  - <file> — <rationale>
//
//	expand_search:
//	  ## Search Coverage Gap
//	  Run grep over additional keyword stems: <kw1>, <kw2>, ...
//
//	swap_view:
//	  ## View Reconcile
//	  Previous attempt routed family=<old>; reconcile to family=<new> — <rationale>
//
//	rebind_subject:
//	  ## Subject Constraint
//	  Answer must be a `<subject>` token. Discard chains whose terminal is a different kind.
//
//	force_complete_downgrade:
//	  ## Force-Complete Downgrade
//	  No progress detected across retries; close out with current evidence.
func (r RepairDirective) Render() string {
	var b strings.Builder
	switch r.Kind {
	case RepairReadFile:
		b.WriteString("## Forced Read List (read these files before emit_investigation_complete)\n")
		for _, f := range r.Files {
			if f == "" {
				continue
			}
			if r.Rationale != "" {
				b.WriteString("- " + f + " — " + r.Rationale + "\n")
			} else {
				b.WriteString("- " + f + "\n")
			}
		}
	case RepairEmitEvidence:
		b.WriteString("## Evidence Materialization\n")
		if len(r.Files) > 0 {
			b.WriteString("Re-emit grounded evidence from these already-read files before retrying completion:\n")
			for _, f := range r.Files {
				if f == "" {
					continue
				}
				if r.Rationale != "" {
					b.WriteString("- " + f + " — " + r.Rationale + "\n")
				} else {
					b.WriteString("- " + f + "\n")
				}
			}
		} else if r.Rationale != "" {
			b.WriteString(r.Rationale + "\n")
		}
	case RepairExpandSearch:
		b.WriteString("## Search Coverage Gap\n")
		b.WriteString("Run grep over additional keyword stems: ")
		b.WriteString(strings.Join(r.Keywords, ", "))
		b.WriteString("\n")
		if r.Rationale != "" {
			b.WriteString(r.Rationale + "\n")
		}
	case RepairSwapView:
		b.WriteString("## View Reconcile\n")
		if r.Subject != "" {
			b.WriteString("Previous attempt routed mismatched family: " + r.Subject + ".\n")
		}
		if r.Rationale != "" {
			b.WriteString(r.Rationale + "\n")
		}
	case RepairRebindSubject:
		b.WriteString("## Subject Constraint\n")
		b.WriteString("Answer must be a `" + r.Subject + "` token. Discard chains whose terminal is a different kind.\n")
		if r.Rationale != "" {
			b.WriteString(r.Rationale + "\n")
		}
	case RepairForceCompleteDowngrade:
		b.WriteString("## Force-Complete Downgrade\n")
		b.WriteString("No progress detected across retries; close out with current evidence.\n")
		if r.Rationale != "" {
			b.WriteString(r.Rationale + "\n")
		}
	default:
		// Unknown kinds render nothing rather than crash; the
		// repair is silently dropped (a hard bug surface elsewhere
		// should be the warning, not a render-time panic).
	}
	return b.String()
}
