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

// RepairDebtClass is a coarse, machine-consumable severity label for repair
// pressure that survives prompt pruning and retry scheduling. It is derived
// only from structured repair metadata, never from user text or model prose.
type RepairDebtClass string

const (
	// RepairDebtPrincipalBlocking means the system has a structured reason to
	// believe the missing repair can change the principal answer surface. This
	// is the only class downstream code should treat as a hard closure blocker.
	RepairDebtPrincipalBlocking RepairDebtClass = "principal_blocking"

	// RepairDebtSurgicalGrounding means the remaining work is a bounded
	// line/anchor/evidence-materialization repair. It may justify one local
	// verification attempt, but should not reopen broad navigation by itself.
	RepairDebtSurgicalGrounding RepairDebtClass = "surgical_grounding"

	// RepairDebtAdvisory means optional breadth, audit, telemetry, or a
	// non-blocking boundary note. It must not hard-block an accepted closure.
	RepairDebtAdvisory RepairDebtClass = "advisory"
)

// ClassifyRepairDirective assigns a structured repair-debt class. The helper is
// intentionally conservative for unknown kinds: callers can use it for
// reporting/checkpoints without weakening existing hard-block behavior.
func ClassifyRepairDirective(r RepairDirective) RepairDebtClass {
	if r.Advisory {
		return RepairDebtAdvisory
	}
	switch r.Kind {
	case RepairExpandSearch, RepairForceCompleteDowngrade:
		return RepairDebtAdvisory
	case RepairEmitEvidence:
		return RepairDebtSurgicalGrounding
	case RepairReadFile:
		if len(r.LineRanges) > 0 {
			return RepairDebtSurgicalGrounding
		}
		if !PendingReadBlocksAcceptedClosure(PendingRead{Origin: r.Origin}) {
			return RepairDebtAdvisory
		}
		return RepairDebtPrincipalBlocking
	case RepairSwapView, RepairRebindSubject:
		return RepairDebtPrincipalBlocking
	default:
		if r.Kind == "" {
			return RepairDebtAdvisory
		}
		return RepairDebtPrincipalBlocking
	}
}

// ClassifyPendingReadRepair mirrors ClassifyRepairDirective for the live
// pending-read queue, which is the authoritative read-file repair surface.
func ClassifyPendingReadRepair(p PendingRead) RepairDebtClass {
	if !PendingReadBlocksAcceptedClosure(p) {
		return RepairDebtAdvisory
	}
	if len(p.LineRanges) > 0 {
		return RepairDebtSurgicalGrounding
	}
	return RepairDebtPrincipalBlocking
}

// PendingReadBlocksAcceptedClosure reports whether a pending read is still a
// load-bearing reason to reopen exploration after the model has already passed
// emit_investigation_complete pre-complete gates.
//
// The default is deliberately conservative: unknown origins keep blocking.
// Only framework-owned support/breadth origins that are known to be soft after
// a typed closure are allowed through. This is structured origin routing, not a
// parse of user text or model prose.
func PendingReadBlocksAcceptedClosure(p PendingRead) bool {
	origin := strings.TrimSpace(p.Origin)
	switch origin {
	case "":
		return true
	case "phase1_unread":
		return false
	}
	if strings.HasPrefix(origin, "chain_promotion.concrete_values_tracer") {
		return false
	}
	return true
}

// RepairBlocksAcceptedClosure mirrors PendingReadBlocksAcceptedClosure for the
// queued repair ledger. Advisory repairs are audit-only by contract. Read-file
// repairs are represented by PendingReads for active gating, so the queued
// historical repair entry itself does not block an accepted closure.
func RepairBlocksAcceptedClosure(r RepairDirective) bool {
	if r.Advisory || r.Kind == RepairReadFile {
		return false
	}
	return r.Kind != ""
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

	// Advisory marks telemetry-only guidance. Advisory directives are
	// recorded for audit/statistics but must not drive ActiveRepairs,
	// reopen an otherwise-complete explore window, or render as a
	// blocking "retry before completion" demand. Use this only when the
	// framework has a noisy or optional improvement signal, not a
	// precise structural defect.
	Advisory bool

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
		kind     RepairKind
		subject  string
		files    string
		advisory bool
	}
	seen := make(map[key]bool, len(in))
	out := in[:0]
	for _, r := range in {
		dup := append([]string(nil), r.Files...)
		sort.Strings(dup)
		k := key{kind: r.Kind, subject: r.Subject, files: strings.Join(dup, "\x00"), advisory: r.Advisory}
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
	if r.Advisory {
		switch r.Kind {
		case RepairReadFile:
			b.WriteString("## Optional Read Advisory\n")
			if len(r.Files) > 0 {
				b.WriteString("If you continue investigating this same branch, these files may add context, but they are not blocking completion:\n")
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
		case RepairEmitEvidence:
			b.WriteString("## Optional Evidence Advisory\n")
			if len(r.Files) > 0 {
				b.WriteString("The current answer can proceed; if another investigation round happens anyway, prefer materializing any useful extra evidence from:\n")
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
		default:
			b.WriteString("## Optional Investigation Advisory\n")
			if len(r.Keywords) > 0 {
				b.WriteString("Non-blocking keyword stems: ")
				b.WriteString(strings.Join(r.Keywords, ", "))
				b.WriteString("\n")
			}
			if r.Subject != "" {
				b.WriteString(r.Subject + "\n")
			}
			if r.Rationale != "" {
				b.WriteString(r.Rationale + "\n")
			}
		}
		return b.String()
	}
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
