// Package hint is the Session 11 F4 HintComposer — the single
// structured entry point for every retry hint the orchestrator
// injects into a downstream agent's prompt. Pre-session-11, retry
// hints were string concatenations assembled at each producer:
// contract_check.renderViolations joined violation details with
// "; "; CGEC G1 dry-run built a fixed-template message; window
// hints manually wove in EdgeValidationFeedback. Each caller saw a
// different slice of the failure; LLMs saw a different format per
// retry.
//
// F4 centralises every retry hint through Composer.Render — a
// 6-field struct (WhatFailed / WhyItFailed / WhatSystemDid /
// ExactFix / AllowedSet / ForbiddenPatterns) with strict-mode
// validation so producers cannot emit a hint that skips "why it
// failed" or the allowed/forbidden sets. Session-11 full-design
// §3 has the complete spec.
//
// Composer is stateless and safe to construct per-call; Compose +
// Render are pure functions over the (violations, context) pair.
package hint

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// AllowedKind classifies one entry in the AllowedSet. The LLM
// reads the Kind label inline ("file", "literal", "shape", ...)
// so the rendered hint is self-describing.
type AllowedKind string

const (
	AllowedFileCitation AllowedKind = "file"
	AllowedLiteralValue AllowedKind = "literal"
	AllowedShapeEnum    AllowedKind = "shape"
	AllowedSubjectKind  AllowedKind = "subject_kind"
)

// Allowed is one explicit candidate entry. Value is what the LLM
// must use; Hint is a ≤ 80-char explanation of why it is allowed
// (e.g. "read at turn N", "contract-mandated single shape").
type Allowed struct {
	Kind  AllowedKind
	Value string
	Hint  string
}

// Hint is the structured retry directive the composer emits.
// Required fields (enforced by Validate) — missing any of them
// triggers Composer.strict-mode rejection so no LLM ever receives
// a half-formed hint.
//
// Producers fill the struct via Composer.Compose; consumers call
// Composer.Render to get the markdown body for injection.
type Hint struct {
	WhatFailed        string
	WhyItFailed       string
	WhatSystemDid     string
	ExactFix          string
	AllowedSet        []Allowed
	ForbiddenPatterns []string

	// Diagnostics — not required for rendering but help operators
	// audit the hint's lineage.
	SourceViolations []types.Violation
	Stage            types.PipelineStage
	DispatchTarget   types.PipelineStage
}

// Validate returns an error describing the first missing / empty
// required field. Composer.Compose calls this in strict mode and
// returns the error instead of a rendered hint; in non-strict mode
// the caller falls back to a legacy string concatenation.
func (h *Hint) Validate() error {
	if h == nil {
		return fmt.Errorf("hint: nil Hint")
	}
	if strings.TrimSpace(h.WhatFailed) == "" {
		return fmt.Errorf("hint: WhatFailed required")
	}
	if strings.TrimSpace(h.WhyItFailed) == "" {
		return fmt.Errorf("hint: WhyItFailed required")
	}
	if strings.TrimSpace(h.WhatSystemDid) == "" {
		return fmt.Errorf("hint: WhatSystemDid required")
	}
	if strings.TrimSpace(h.ExactFix) == "" {
		return fmt.Errorf("hint: ExactFix required")
	}
	if len(h.AllowedSet) == 0 {
		return fmt.Errorf("hint: AllowedSet must contain ≥1 entry")
	}
	return nil
}

// Config controls Composer behaviour. Default values match the
// package defaults; operators can override via codrax.yaml.
type Config struct {
	// StrictMode, when true, makes Compose return an error on
	// Validate failure. Non-strict mode falls back to the legacy
	// concatenation (Render still works on an incomplete Hint by
	// skipping empty fields) — this is the G3 gradual-rollout lever.
	StrictMode bool

	// MaxAllowedSet caps the number of AllowedSet entries rendered.
	// Default 10.
	MaxAllowedSet int

	// MaxForbiddenPatterns caps the number of ForbiddenPatterns
	// rendered. Default 5.
	MaxForbiddenPatterns int
}

// DefaultConfig returns non-strict defaults safe to use on boot
// before codrax.yaml is loaded.
func DefaultConfig() Config {
	return Config{
		StrictMode:           false,
		MaxAllowedSet:        10,
		MaxForbiddenPatterns: 5,
	}
}

// Composer builds hints from violation data. Stateless; safe to
// reuse across goroutines.
type Composer struct {
	cfg Config
}

// New constructs a Composer from cfg. Missing fields fall back to
// DefaultConfig values.
func New(cfg Config) *Composer {
	if cfg.MaxAllowedSet <= 0 {
		cfg.MaxAllowedSet = 10
	}
	if cfg.MaxForbiddenPatterns <= 0 {
		cfg.MaxForbiddenPatterns = 5
	}
	return &Composer{cfg: cfg}
}

// Compose synthesises a Hint from the given violations + context.
// ctx supplies the auxiliary data the composer needs to build
// AllowedSet (e.g. ReadSet for citation violations, shape enum for
// shape violations) without reaching into the orchestrator.
//
// In strict mode, Compose returns an error when Validate fails
// so the caller can decide whether to fall back to a legacy
// concatenation or fail loud. In non-strict mode, Compose never
// returns an error — validation gaps are accepted and Render
// simply omits empty sections.
func (c *Composer) Compose(ctx Context, violations []types.Violation) (*Hint, error) {
	h := &Hint{
		SourceViolations: violations,
		Stage:            ctx.Stage,
		DispatchTarget:   ctx.DispatchTarget,
	}
	if len(violations) == 0 {
		// Nothing to hint about. Return an empty hint so callers
		// can decide whether to emit a blank directive or skip.
		if c.cfg.StrictMode {
			return nil, fmt.Errorf("hint: no violations to compose hint from")
		}
		return h, nil
	}

	h.WhatFailed = summariseWhatFailed(violations)
	h.WhyItFailed = summariseWhyItFailed(violations)
	h.WhatSystemDid = summariseWhatSystemDid(ctx)
	h.ExactFix = summariseExactFix(violations, ctx)
	h.AllowedSet = limitAllowed(buildAllowedSet(violations, ctx), c.cfg.MaxAllowedSet)
	h.ForbiddenPatterns = limitStrings(buildForbiddenPatterns(violations, ctx), c.cfg.MaxForbiddenPatterns)

	if c.cfg.StrictMode {
		if err := h.Validate(); err != nil {
			return nil, err
		}
	}
	return h, nil
}

// Render turns a Hint into the markdown body. Empty fields are
// skipped so a non-strict, partial Hint still produces a valid
// (if terse) prompt section.
func (c *Composer) Render(h *Hint) string {
	if h == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Retry Directive\n\n")
	if h.WhatFailed != "" {
		fmt.Fprintf(&b, "**What failed**: %s\n\n", h.WhatFailed)
	}
	if h.WhyItFailed != "" {
		fmt.Fprintf(&b, "**Why it failed**: %s\n\n", h.WhyItFailed)
	}
	if h.WhatSystemDid != "" {
		fmt.Fprintf(&b, "**What I already did**: %s\n\n", h.WhatSystemDid)
	}
	if h.ExactFix != "" {
		fmt.Fprintf(&b, "**How to fix now**: %s\n\n", h.ExactFix)
	}
	if len(h.AllowedSet) > 0 {
		b.WriteString("**Allowed**:\n")
		for _, a := range h.AllowedSet {
			if a.Hint != "" {
				fmt.Fprintf(&b, "- `%s` — %s (%s)\n", a.Value, a.Hint, a.Kind)
			} else {
				fmt.Fprintf(&b, "- `%s` (%s)\n", a.Value, a.Kind)
			}
		}
		b.WriteString("\n")
	}
	if len(h.ForbiddenPatterns) > 0 {
		b.WriteString("**Do NOT**:\n")
		for _, p := range h.ForbiddenPatterns {
			fmt.Fprintf(&b, "- %s\n", p)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// RenderCompact returns a single-line fallback format for legacy
// call sites that want a terse ";"-separated string (the pre-
// session-11 renderViolations contract). Keeps existing tests /
// monitors that match the legacy format happy during the G3
// rollout.
func (c *Composer) RenderCompact(h *Hint) string {
	if h == nil || len(h.SourceViolations) == 0 {
		return ""
	}
	parts := make([]string, 0, len(h.SourceViolations))
	for _, v := range h.SourceViolations {
		s := v.Detail
		if v.Repair != "" {
			s += " (" + v.Repair + ")"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "; ")
}

// Context carries the auxiliary inputs Compose uses to populate
// AllowedSet / ForbiddenPatterns / WhatSystemDid. Producers fill
// only the fields they have; missing fields degrade gracefully
// (the corresponding Hint section becomes shorter).
type Context struct {
	Stage          types.PipelineStage
	DispatchTarget types.PipelineStage

	// ReadSet is the authoritative Turn A file list; fed into
	// AllowedSet when a citation violation fires.
	ReadSet []string

	// TargetShape is the contract's RequiredAnswerShape — drives
	// AllowedSet entries for shape violations.
	TargetShape types.AnswerShape

	// TargetSubjectKind is the contract's AnswerSubject.Kind —
	// drives AllowedSet entries for literal-form violations.
	TargetSubjectKind types.AnswerSubjectKind

	// PrimaryEntity is the user-question's primary entity name —
	// drives ForbiddenPatterns for self-reference violations.
	PrimaryEntity string

	// RecentPatches summarises any IR patches the F3 engine has
	// applied this dispatch. Rendered into WhatSystemDid so the
	// LLM sees that the system has already reconciled fields.
	RecentPatches []PatchSummary
}

// PatchSummary is a thin projection of patcher.PatchRecord so the
// hint package does not import the (not-yet-written G2) patcher.
type PatchSummary struct {
	Field    string
	FromDesc string
	ToDesc   string
	Reason   string
}

// ── internal helpers ───────────────────────────────────────────

// summariseWhatFailed returns a one-sentence label for the
// primary violation kind. Callers that need all violations listed
// can iterate h.SourceViolations; the top-line is meant for LLM
// orientation.
func summariseWhatFailed(violations []types.Violation) string {
	kinds := distinctKinds(violations)
	if len(kinds) == 0 {
		return "one or more contract / enforcer checks failed"
	}
	if len(kinds) == 1 {
		return fmt.Sprintf("%s violation: %s",
			string(kinds[0]),
			violations[0].Detail)
	}
	kindStrs := make([]string, len(kinds))
	for i, k := range kinds {
		kindStrs[i] = string(k)
	}
	return fmt.Sprintf("multiple violations (%s)", strings.Join(kindStrs, ", "))
}

// summariseWhyItFailed pulls the highest-confidence SuspectedRoot
// reason out of the violation set. Falls back to a generic string
// when no SuspectedRoot is populated. Always references at least
// one source violation so the validator's "must cite source"
// invariant is satisfied.
func summariseWhyItFailed(violations []types.Violation) string {
	var bestIdx int = -1
	var bestConf float64
	for i, v := range violations {
		if v.SuspectedRoot.Confidence > bestConf {
			bestConf = v.SuspectedRoot.Confidence
			bestIdx = i
		}
	}
	if bestIdx == -1 {
		// No suspected-root info; fall back to the first
		// violation's Detail so the field is non-empty.
		return fmt.Sprintf("from %d violation(s): %s",
			len(violations), violations[0].Detail)
	}
	v := violations[bestIdx]
	return fmt.Sprintf("%s (suspected field=%s, confidence=%.2f, from %d event(s))",
		v.SuspectedRoot.Reason,
		v.SuspectedRoot.IRField,
		bestConf,
		len(violations))
}

// summariseWhatSystemDid renders the patch summary in ctx, or a
// "no patch applied" sentinel so the field is never empty.
func summariseWhatSystemDid(ctx Context) string {
	if len(ctx.RecentPatches) == 0 {
		return "no patch applied"
	}
	parts := make([]string, 0, len(ctx.RecentPatches))
	for _, p := range ctx.RecentPatches {
		parts = append(parts, fmt.Sprintf("patched %s: %s → %s (%s)",
			p.Field, p.FromDesc, p.ToDesc, p.Reason))
	}
	return strings.Join(parts, "; ")
}

// summariseExactFix emits a one-sentence imperative instruction
// tailored to the dominant violation kind. Operators add new
// kind cases here as hookups land in later groups.
func summariseExactFix(violations []types.Violation, ctx Context) string {
	if len(violations) == 0 {
		return ""
	}
	switch violations[0].Kind {
	case types.ViolShape, types.ViolShapeSwap:
		if ctx.TargetShape != "" {
			return fmt.Sprintf("Re-emit with shape=%q and only the fields that shape requires.", ctx.TargetShape)
		}
		return "Re-emit with the contract's required shape; do NOT use any other shape."
	case types.ViolCitation:
		return "Cite only files from the Allowed list below; do NOT invent line numbers."
	case types.ViolGhostAnchor:
		return "The system will read the referenced files on your behalf; wait for the updated ReadSet before re-emitting."
	case types.ViolSelfRefLiteral:
		return "Pick a literal matching the target subject kind; self-name of the primary entity is invalid."
	case types.ViolLiteralFormFailed:
		if ctx.TargetSubjectKind != "" {
			return fmt.Sprintf("Emit a literal matching answer_subject.kind=%q; see Allowed for example shapes.", ctx.TargetSubjectKind)
		}
		return "Emit a literal matching the target subject kind's form."
	case types.ViolPreCompleteDowngrade:
		return "Continue the investigation; emit more file:line evidence anchored in files Turn A actually read."
	case types.ViolMustInclude:
		return "Include the required term(s) in your answer text."
	case types.ViolMustExclude:
		return "Remove the forbidden term(s) from your answer text."
	case types.ViolAcceptance, types.ViolSuccessCriterion:
		return "Adjust the answer to satisfy the acceptance criterion."
	}
	return "Address the violation(s) listed above and re-emit."
}

// buildAllowedSet assembles the explicit-enumeration allowed list
// based on the dominant violation kind.
func buildAllowedSet(violations []types.Violation, ctx Context) []Allowed {
	var out []Allowed
	if len(violations) == 0 {
		return out
	}
	switch violations[0].Kind {
	case types.ViolCitation, types.ViolGhostAnchor, types.ViolPreCompleteDowngrade:
		for _, f := range ctx.ReadSet {
			out = append(out, Allowed{
				Kind:  AllowedFileCitation,
				Value: f,
				Hint:  "file in Turn A ReadSet",
			})
		}
	case types.ViolShape, types.ViolShapeSwap:
		if ctx.TargetShape != "" {
			out = append(out, Allowed{
				Kind:  AllowedShapeEnum,
				Value: string(ctx.TargetShape),
				Hint:  "contract-mandated single shape",
			})
		}
	case types.ViolSelfRefLiteral, types.ViolLiteralFormFailed:
		if ctx.TargetSubjectKind != "" {
			out = append(out, Allowed{
				Kind:  AllowedSubjectKind,
				Value: string(ctx.TargetSubjectKind),
				Hint:  "pick a literal matching this kind",
			})
		}
	}
	return out
}

// buildForbiddenPatterns lists negative examples keyed off the
// violation kind. Self-reference and shape-off-enum are the two
// most common traps.
func buildForbiddenPatterns(violations []types.Violation, ctx Context) []string {
	var out []string
	if len(violations) == 0 {
		return out
	}
	switch violations[0].Kind {
	case types.ViolSelfRefLiteral:
		if ctx.PrimaryEntity != "" {
			out = append(out, fmt.Sprintf("Do NOT emit literal=%q — that is the question's primary entity (self-reference).", ctx.PrimaryEntity))
		}
	case types.ViolShape, types.ViolShapeSwap:
		if ctx.TargetShape != "" {
			out = append(out, fmt.Sprintf("Do NOT output any shape other than %q.", ctx.TargetShape))
		}
	case types.ViolCitation:
		out = append(out, "Do NOT cite files that Turn A did not read.")
		out = append(out, "Do NOT invent line numbers.")
	case types.ViolGhostAnchor:
		out = append(out, "Do NOT reference files outside the ScannedSet.")
	}
	return out
}

// distinctKinds returns the unique ViolationKind set sorted for
// stable rendering.
func distinctKinds(violations []types.Violation) []types.ViolationKind {
	seen := make(map[types.ViolationKind]bool)
	var kinds []types.ViolationKind
	for _, v := range violations {
		if !seen[v.Kind] {
			seen[v.Kind] = true
			kinds = append(kinds, v.Kind)
		}
	}
	sort.Slice(kinds, func(i, j int) bool { return string(kinds[i]) < string(kinds[j]) })
	return kinds
}

func limitAllowed(in []Allowed, cap int) []Allowed {
	if cap <= 0 || len(in) <= cap {
		return in
	}
	out := make([]Allowed, cap+1)
	copy(out, in[:cap])
	out[cap] = Allowed{Kind: "", Value: fmt.Sprintf("...and %d more", len(in)-cap)}
	return out
}

func limitStrings(in []string, cap int) []string {
	if cap <= 0 || len(in) <= cap {
		return in
	}
	out := make([]string, cap+1)
	copy(out, in[:cap])
	out[cap] = fmt.Sprintf("...and %d more", len(in)-cap)
	return out
}
