// Package patcher is the Session 11 F3 IRPatchEngine — the
// deterministic reconciler that applies targeted mutations to a
// RequestModel after the analyzer has emitted the initial IR.
// Callers (the orchestrator's window-close hook, driven by F2
// Aggregator) submit PatchRequest values; the engine enforces
// white-listed fields, per-field / per-run budgets, monotonicity,
// and idempotency. Every applied patch is recorded in an audit
// log that the F4 HintComposer reads when filling the
// WhatSystemDid field.
//
// The engine does NOT re-run the analyzer LLM. The full-design
// doc §2.2 lays out why: LLM re-entry is expensive, non-
// deterministic, and would multiply retry cost. Instead we
// recognise that the IR invariants that C1 (manual cascade
// reconcile) enforced by hand are themselves deterministic,
// field-specific rules — the engine encodes them as pure
// transforms.
//
// G2 ships the Engine skeleton + field-replace operation. Full
// recompile hooks (compiler.RecomputeShapeDerivedFields, evidence
// staleness marking) land when G6/G7 hookups drive real patch
// requests through the engine.
package patcher

import (
	"fmt"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// PatchOp is the category of mutation a PatchRequest carries. The
// Engine enforces a different contract per operation: Replace is
// idempotent + monotonic on a single scalar; AppendOnly never
// removes entries; PatchByKey mutates a named sub-field.
type PatchOp string

const (
	OpReplace    PatchOp = "replace"
	OpAppendOnly PatchOp = "append_only"
	OpPatchByKey PatchOp = "patch_by_key"
)

// PatchRequest describes one proposed mutation to the IR. Callers
// (F2 Aggregator) fill Field + NewValue; Engine.Apply is
// responsible for enforcing the whitelist and deciding whether
// the patch is accepted, skipped, or rejected.
type PatchRequest struct {
	Field        string // dotted-path IR field (see allowedFields)
	Operation    PatchOp
	OldValue     any
	NewValue     any
	Reason       string
	SourceEvents []types.Violation // traceability back to the ledger
	DispatchID   string            // for audit correlation
}

// PatchRecord is the engine's audit entry per request. Applied
// distinguishes accepted patches from rejected ones; SkipReason
// classifies why a request was not applied.
type PatchRecord struct {
	Request    PatchRequest
	Applied    bool
	SkipReason string // "idempotent" / "monotonic_violation" /
	                 // "budget_exhausted" / "field_not_whitelisted" /
	                 // "value_type_mismatch"
	AppliedAt  time.Time
}

// Budget — per-Run limits. Zero-valued Budget means "no cap"; the
// Engine enforces Session 11 defaults via the WithDefaults
// constructor.
type Budget struct {
	MaxPatchesPerRun   int
	MaxPatchesPerField int
}

// Engine is the F3 entry point. Stateful within a single Run:
// tracks applied history per field so idempotent + monotonic
// checks can fire on repeat submissions. Safe for concurrent
// Submit / Apply calls from multiple goroutines.
type Engine struct {
	mu      sync.Mutex
	budget  Budget
	applied map[string][]PatchRecord // field → history
	audit   []PatchRecord            // global timeline
}

// New constructs an Engine with the given Budget. Passing a
// zero Budget uses Session 11 defaults (MaxPatchesPerRun=4,
// MaxPatchesPerField=2).
func New(b Budget) *Engine {
	if b.MaxPatchesPerRun <= 0 {
		b.MaxPatchesPerRun = 4
	}
	if b.MaxPatchesPerField <= 0 {
		b.MaxPatchesPerField = 2
	}
	return &Engine{
		budget:  b,
		applied: make(map[string][]PatchRecord),
	}
}

// allowedFields enumerates the subset of IR mutations F3 accepts.
// Adding a field here is a deliberate design decision — operators
// review + test + document each. Unknown fields get a
// "field_not_whitelisted" SkipReason.
var allowedFields = map[string]bool{
	"answer_shape":             true,
	"answer_subject.kind":      true,
	"question_kind":            true,
	"entity_axes":              true,
	"keywords":                 true, // append-only
	"EvidencePlan.SourceMix":   true,
	"EvidencePlan.NodeBudgets": true,
	"ScannedSet":               true, // append-only
}

// IsFieldWhitelisted returns true when the Engine will consider
// patches targeting field. Exported so the aggregator can skip
// building patch requests for out-of-whitelist fields.
func IsFieldWhitelisted(field string) bool {
	return allowedFields[field]
}

// Submit processes a PatchRequest. Returns the PatchRecord
// describing the outcome. The Engine never mutates the IR
// directly on Submit — callers pass the target RequestModel to
// Apply below when they are ready for the mutation to take
// effect. This separation lets tests probe acceptance rules
// without side-effects on a shared IR.
func (e *Engine) Submit(req PatchRequest) PatchRecord {
	if e == nil {
		return PatchRecord{Request: req, SkipReason: "nil_engine"}
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	rec := PatchRecord{Request: req}

	if !allowedFields[req.Field] {
		rec.SkipReason = "field_not_whitelisted"
		e.audit = append(e.audit, rec)
		return rec
	}
	if len(e.audit) >= e.budget.MaxPatchesPerRun {
		rec.SkipReason = "budget_exhausted_run"
		e.audit = append(e.audit, rec)
		return rec
	}
	fieldHistory := e.applied[req.Field]
	if len(fieldHistory) >= e.budget.MaxPatchesPerField {
		rec.SkipReason = "budget_exhausted_field"
		e.audit = append(e.audit, rec)
		return rec
	}
	// Idempotent: if the most recent accepted patch on this
	// field matches req.NewValue, skip.
	if last := findLastApplied(fieldHistory); last != nil {
		if fmt.Sprintf("%v", last.Request.NewValue) == fmt.Sprintf("%v", req.NewValue) {
			rec.SkipReason = "idempotent"
			e.audit = append(e.audit, rec)
			return rec
		}
		// Monotonic: reject a patch that reverts to any earlier
		// applied value on the same field.
		for _, prior := range fieldHistory {
			if !prior.Applied {
				continue
			}
			if fmt.Sprintf("%v", prior.Request.NewValue) == fmt.Sprintf("%v", req.NewValue) {
				rec.SkipReason = "monotonic_violation"
				e.audit = append(e.audit, rec)
				return rec
			}
		}
	}

	rec.Applied = true
	rec.AppliedAt = time.Now()
	e.applied[req.Field] = append(e.applied[req.Field], rec)
	e.audit = append(e.audit, rec)
	return rec
}

// Apply walks the engine's accepted records and mutates rm for
// each Replace operation on a whitelisted scalar field. The
// mutation path is intentionally conservative: only fields whose
// NewValue type matches the IR field's Go type are applied;
// mismatched types record a "value_type_mismatch" skip AFTER the
// fact so the audit still shows the attempt.
//
// Callers run Apply AFTER every Submit batch so the IR reflects
// the reconciled view when the next window dispatches. Passing
// nil rm is safe — Apply returns without mutating.
func (e *Engine) Apply(rm *types.RequestModel) {
	if e == nil || rm == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	for i := range e.audit {
		rec := &e.audit[i]
		if !rec.Applied {
			continue
		}
		if rec.Request.Operation != OpReplace {
			continue // append-only + patch-by-key handled elsewhere
		}
		if err := applyReplace(rm, rec.Request.Field, rec.Request.NewValue); err != nil {
			// Demote from Applied and record the reason.
			rec.Applied = false
			rec.SkipReason = "value_type_mismatch:" + err.Error()
		}
	}
}

// AuditLog returns a defensive copy of every record the engine
// has processed. Consumed by the F4 HintComposer's
// WhatSystemDid field and by diagnostic tooling.
func (e *Engine) AuditLog() []PatchRecord {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]PatchRecord, len(e.audit))
	copy(out, e.audit)
	return out
}

// AppliedCount returns the number of successfully-applied
// patches; used by the F5 yield check.
func (e *Engine) AppliedCount() int {
	if e == nil {
		return 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, r := range e.audit {
		if r.Applied {
			n++
		}
	}
	return n
}

// findLastApplied returns the most recent Applied=true record in
// history, or nil.
func findLastApplied(history []PatchRecord) *PatchRecord {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Applied {
			return &history[i]
		}
	}
	return nil
}

// applyReplace mutates rm's named scalar field with newVal.
// Currently handles the whitelist's scalar fields
// (answer_shape, answer_subject.kind, question_kind); list /
// map fields are left to future G6/G7 implementations.
func applyReplace(rm *types.RequestModel, field string, newVal any) error {
	switch field {
	case "answer_shape":
		// The LLM-string label lives in AnalyzerHints.Shape;
		// typed AnswerShape sits on AnswerContract after compile.
		s, ok := newVal.(string)
		if !ok {
			return fmt.Errorf("expected string for answer_shape")
		}
		rm.AnalyzerHints.Shape = s
		return nil
	case "answer_subject.kind":
		// Accept either the typed enum or its string label.
		switch v := newVal.(type) {
		case types.AnswerSubjectKind:
			rm.AnswerSubject.Kind = v
			return nil
		case string:
			rm.AnswerSubject.Kind = types.AnswerSubjectKind(v)
			return nil
		}
		return fmt.Errorf("expected AnswerSubjectKind or string")
	case "question_kind":
		s, ok := newVal.(string)
		if !ok {
			return fmt.Errorf("expected string for question_kind")
		}
		rm.AnalyzerHints.Kind = s
		return nil
	}
	// Other whitelisted fields not yet implemented — reject so
	// Submit's Applied=true gets reverted.
	return fmt.Errorf("apply for field %q not implemented", field)
}
