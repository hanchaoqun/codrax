package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/ground"
	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// EmitEvidence is the structured replacement for the LLM-authored
// markdown evidence channel that parseEvidenceItems historically
// walked. The explorer phase-2
// prompt instructs the LLM to call this tool once per file with a
// batch of EvidenceItem-shaped objects; the tool validates them and
// appends to BusContext.Mutable.AppendEvidence, where the explorer's
// ensureStructuredEvidence picks them up after the ReAct loop exits.
//
// P1.1 design (memory/project_architecture_remediation_roadmap.md §6):
// kills root cause R2 ("no schema where LLM is both writer and
// reader") by removing the regex-walked markdown bridge. The contract
// is intentionally fail-loud: unknown fields and unknown kinds are
// rejected with explicit errors rather than silently coerced, so the
// LLM gets a round-trip error message instead of producing a
// well-formed-looking item the parser quietly mangles.
//
// Classified ReadOnly because IsWrite() is the filesystem-write
// boundary; mutating BusContext is not a filesystem write.
// Classified NonEvidenceTool because the tool itself does not produce
// repo facts — the items it carries are evidence, but the tool call
// is just transport.
type EmitEvidence struct {
	ReadOnly
	NonEvidenceTool
}

// emitEvidenceAllowedKinds is derived at package init from
// types.LLMEmittableEvidenceKinds() so the whitelist, the canonical
// EvidenceKind declaration, and the JSON schema enum stay in lockstep
// by construction. Adding a new LLM-emittable kind means flipping
// IsLLMEmittable in internal/types/evidence.go and nothing else.
var emitEvidenceAllowedKinds = buildEmitEvidenceAllowedKinds()
var emitAnchorKinds = buildEmitAnchorKinds()

func buildEmitEvidenceAllowedKinds() map[string]types.EvidenceKind {
	out := make(map[string]types.EvidenceKind, len(types.LLMEmittableEvidenceKinds()))
	for _, k := range types.LLMEmittableEvidenceKinds() {
		out[strings.ToLower(string(k))] = k
	}
	return out
}

func buildEmitAnchorKinds() map[string]types.AnchorKind {
	out := make(map[string]types.AnchorKind, len(types.AllAnchorKinds()))
	for _, k := range types.AllAnchorKinds() {
		out[strings.ToLower(string(k))] = k
	}
	return out
}

// emitEvidenceAllowedKindNames returns the accepted kind strings in
// canonical order, for schema enum rendering and error messages.
func emitEvidenceAllowedKindNames() []string {
	kinds := types.LLMEmittableEvidenceKinds()
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = string(k)
	}
	return names
}

// emitAnchorKindNames returns the accepted AnchorKind strings in
// canonical order. Required on every emit_evidence item so the
// grounder (internal/tool/ground) can dispatch Tier 2 without
// guessing "is this a definition, a callsite, or a condition?".
func emitAnchorKindNames() []string {
	kinds := types.AllAnchorKinds()
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = string(k)
	}
	return names
}

func emitEvidenceContextRoleNames() []string {
	roles := types.AllEvidenceContextRoles()
	names := make([]string, 0, len(roles)-1)
	for _, r := range roles {
		if r == types.EvidenceContextRoleUnknown {
			continue
		}
		names = append(names, string(r))
	}
	return names
}

func emitEvidenceDiagramRoleNames() []string {
	roles := types.AllEvidenceDiagramRoles()
	names := make([]string, 0, len(roles)-1)
	for _, r := range roles {
		if r == types.EvidenceDiagramRoleUnknown {
			continue
		}
		names = append(names, string(r))
	}
	return names
}

type emitEvidenceParams struct {
	Items []emitEvidenceItem `json:"items"`
}

type emitEvidenceItem struct {
	EvidenceKind string `json:"evidence_kind,omitempty"`
	LegacyKind   string `json:"kind,omitempty"`
	Subject      string `json:"subject,omitempty"`
	Predicate    string `json:"predicate,omitempty"`
	Object       string `json:"object,omitempty"`
	Source       string `json:"source"`
	// LineStart / LineEnd use FlexInt so LLMs that emit numeric
	// strings ("42") or floats (42.0) pass strict schema validation
	// instead of failing the whole batch on format pedantry.
	LineStart    FlexInt `json:"line_start,omitempty"`
	LineEnd      FlexInt `json:"line_end,omitempty"`
	Condition    string  `json:"condition,omitempty"`
	Summary      string  `json:"summary,omitempty"`
	AnchorKind   string  `json:"anchor_kind"`
	AnchorSymbol string  `json:"anchor_symbol"`
	Snippet      string  `json:"snippet,omitempty"`
	ContextRole  string  `json:"context_role_hint,omitempty"`
	DiagramRole  string  `json:"diagram_role_hint,omitempty"`
}

// EmitEvidenceProducer is the Producer string stamped on every item
// the tool ingests. Exported so tests and downstream consumers
// (filtering-pipeline doc, future grounder integration) can identify
// the channel without grepping for a literal.
const EmitEvidenceProducer = "explorer.emit_evidence"

func (t *EmitEvidence) Name() string { return "emit_evidence" }

func (t *EmitEvidence) Description() string {
	return "Emit one or more structured evidence items as the result of reading a source file. " +
		"Call this AFTER you have read a file during the depth-investigation stage, with one item per " +
		"fact you want the synthesis layer to see. The batched 'items' array preserves the " +
		"existing 'one tool call per file' write pattern; do not call this tool once per item.\n\n" +
		"Each item MUST set: source (repo-relative path), line_start (exact gutter line number, " +
		"never estimated), evidence_kind (one of: " + strings.Join(emitEvidenceAllowedKindNames(), ", ") + "), " +
		"anchor_kind (one of: " + strings.Join(emitAnchorKindNames(), ", ") + "), and anchor_symbol " +
		"(the identifier the grounder should find on that line).\n\n" +
		"There are TWO different kind fields with different jobs:\n" +
		"  - evidence_kind = the SEMANTIC fact shape (direct / conditional / registration / mechanism / relationship)\n" +
		"  - anchor_kind   = the CODE syntax at source:line_start (definition / call / condition / return / assignment / import)\n" +
		"Never put `direct` / `conditional` / `registration` / `mechanism` / `relationship` into anchor_kind. " +
		"Never put `definition` / `call` / `condition` / `return` / `assignment` / `import` into evidence_kind.\n\n" +
		"anchor_kind tells the grounder what KIND of code location you are pointing at:\n" +
		"  - definition: the line is a function/type/const/var declaration\n" +
		"  - call:       the line contains a function/method call (anchor_symbol = callee name)\n" +
		"  - condition:  the line starts an if / when / unless / switch / case / guard\n" +
		"  - return:     the line is a return or yield\n" +
		"  - assignment: the line assigns (:= or =)\n" +
		"  - import:     the line is an import / use / require (anchor_symbol = package path/alias)\n\n" +
		"anchor_symbol is the concrete identifier the grounder should see at line_start. For a " +
		"method call 'x.Execute()' at line 42 the anchor_symbol is 'Execute' and anchor_kind is 'call'. " +
		"For a struct type declaration 'type Orchestrator struct' the anchor_symbol is 'Orchestrator' " +
		"and anchor_kind is 'definition'. On that same line the evidence_kind may still be 'direct' " +
		"because the semantic claim is simply a direct fact about a definition. Likewise a config assignment " +
		"line can be `evidence_kind=\"direct\"` with `anchor_kind=\"assignment\"`.\n\n" +
		"For call-like evidence (`predicate` = calls / invokes / dispatches / delegates to) with " +
		"`anchor_kind=\"call\"`, the semantic direction is ALWAYS caller -> callee: `subject` must be " +
		"the containing function/method and `object` must be the callee on that line. Example: if " +
		"`outer()` contains `return inner(...)`, emit `subject=\"outer\"`, `predicate=\"calls\"`, " +
		"`object=\"inner\"`.\n\n" +
		"Optional hint fields: `context_role_hint` may be `defining`, `absence_support`, `related_context`, or `illustrative_only` " +
		"to recommend how the item should be used for exact-target answers. `diagram_role_hint` may be `default`, " +
		"`config`, `runtime`, or `override` for config-precedence traces (`config` = grounded repo/user config-file layer such as YAML/JSON/TOML/INI/etc.). These are recommendations only: the tool " +
		"validates them structurally and may downgrade or ignore inconsistent hints.\n\n" +
		"snippet is optional but recommended for conditional / mechanism / registration items: paste " +
		"1-2 lines of the actual code so the snippet_fuzzy recovery tier can re-anchor if your " +
		"line_start is off by one.\n\n" +
		"The emit_evidence tool grounds every item synchronously and returns per-item feedback " +
		"(grounded / recovered / ungrounded) in the same turn, so you can correct line numbers or " +
		"anchor_symbols on the next call without waiting for a later stage. Unknown evidence_kind / anchor_kind values and " +
		"unknown fields are REJECTED — the tool will not silently coerce."
}

func emitEvidenceParametersSchema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"items": map[string]any{
				"type":        "array",
				"description": "Batch of evidence items extracted from one or more files. Send the full batch in one call 鈥?do not invoke the tool per item.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"evidence_kind": map[string]any{
							"type":        "string",
							"enum":        emitEvidenceAllowedKindNames(),
							"description": "REQUIRED. Semantic fact shape, NOT the syntax at line_start. direct = literal fact at file:line. conditional = behaviour gated by an IF clause. registration = something registered/bound with EXACT values. mechanism = how a process works step by step. relationship = link between two symbols (use subject + object). Values like definition/call/assignment belong in anchor_kind, not here. NOTE: for absence claims (searched and found nothing) do NOT emit via this tool 鈥?every evidence_kind requires a concrete file:line anchor, which is unsatisfiable for 'not found'. Use emit_investigation_complete.absence_justification for whole-answer absence, or simply omit the item for per-fact absence.",
						},
						"subject": map[string]any{
							"type":        "string",
							"description": "Primary semantic symbol the item is about (function name, type, key). For call-like predicates with anchor_kind='call', subject MUST be the caller / containing function at that line.",
						},
						"predicate": map[string]any{
							"type":        "string",
							"description": "Lowercase verb tying subject to object. PREFER these canonical verbs so the finalizer's deterministic relation-diagram renderer picks the edge up 鈥?anything outside this list is rendered as unstructured prose: calls, invokes, dispatches, delegates to, binds, binds ONLY, registers, wires, provides, returns, yields, constructs, instantiates, defines, implements, extends, embeds, maps, config, decorates. Optional; defaults to the lower-cased evidence_kind.",
						},
						"object": map[string]any{
							"type":        "string",
							"description": "Secondary symbol or value. Required for relationship; optional otherwise. For call-like predicates with anchor_kind='call', object MUST be the callee symbol on that line.",
						},
						"source": map[string]any{
							"type":        "string",
							"description": "Repository-relative file path the fact comes from. Required.",
						},
						"line_start": map[string]any{
							"type":        "integer",
							"description": "Exact gutter line number from read_file 鈥?NEVER estimated. The grounder uses this to verify the claim; wrong numbers are flagged as ungrounded or auto-recovered.",
						},
						"line_end": map[string]any{
							"type":        "integer",
							"description": "Last line of the cited range. Defaults to line_start when omitted.",
						},
						"condition": map[string]any{
							"type":        "string",
							"description": "For conditional items: the exact IF clause that triggers the behaviour.",
						},
						"summary": map[string]any{
							"type":        "string",
							"description": "Free-text rationale describing the fact. Keep concise; do not paraphrase numbers or string literals.",
						},
						"context_role_hint": map[string]any{
							"type":        "string",
							"enum":        emitEvidenceContextRoleNames(),
							"description": "OPTIONAL recommendation for exact-target questions. defining = direct defining proof, related_context = grounded nearby context but not the exact target itself, illustrative_only = comment/doc/test/example mention that should NOT be treated as defining proof. The tool validates and may downgrade the hint.",
						},
						"diagram_role_hint": map[string]any{
							"type":        "string",
							"enum":        emitEvidenceDiagramRoleNames(),
							"description": "OPTIONAL recommendation for config-precedence traces. default = code defaults, config = repo/user config-file layer (YAML/JSON/TOML/INI/etc.), runtime = code/runtime binding layer, override = CLI/high-precedence override layer. The tool validates and may ignore inconsistent hints.",
						},
						"anchor_kind": map[string]any{
							"type":        "string",
							"enum":        emitAnchorKindNames(),
							"description": "REQUIRED. Code syntax at line_start, NOT the semantic evidence shape. definition = symbol declaration, call = function/method call site, condition = if/when/switch/case/guard line, return = return or yield, assignment = := or = assignment, import = import/use/require statement. Values like direct/conditional/registration belong in evidence_kind, not here. The grounder dispatches on this so wrong anchor kinds produce confusing ungrounded verdicts.",
						},
						"anchor_symbol": map[string]any{
							"type":        "string",
							"description": "REQUIRED. The identifier the grounder should find on line_start. For a call like 'x.Execute()' the anchor_symbol is 'Execute'. For a type decl 'type Orchestrator struct' the anchor_symbol is 'Orchestrator'. For an import the anchor_symbol is the package path or local alias.",
						},
						"snippet": map[string]any{
							"type":        "string",
							"description": "Optional. 1-2 lines of actual code from the cited location. Enables snippet_fuzzy recovery when line_start is off by 卤15 lines 鈥?recommended for conditional / mechanism / registration items.",
						},
					},
					"required": []string{"evidence_kind", "source", "line_start", "anchor_kind", "anchor_symbol"},
				},
			},
		},
		"required": []string{"items"},
	}
	buf, _ := json.Marshal(schema)
	return json.RawMessage(buf)
}

func (t *EmitEvidence) Parameters() json.RawMessage {
	if schema := emitEvidenceParametersSchema(); len(schema) > 0 {
		return schema
	}
	// Build the enum lists as JSON so they stay in lockstep with the
	// canonical types.LLMEmittableEvidenceKinds / AllAnchorKinds lists
	// — hand-editing the schema literal is how the 6-vs-11 drift bug
	// was born.
	evidenceKindEnumJSON, _ := json.Marshal(emitEvidenceAllowedKindNames())
	contextRoleEnumJSON, _ := json.Marshal(emitEvidenceContextRoleNames())
	diagramRoleEnumJSON, _ := json.Marshal(emitEvidenceDiagramRoleNames())
	anchorEnumJSON, _ := json.Marshal(emitAnchorKindNames())
	schema := fmt.Sprintf(`{
  "type": "object",
  "properties": {
    "items": {
      "type": "array",
      "description": "Batch of evidence items extracted from one or more files. Send the full batch in one call — do not invoke the tool per item.",
      "items": {
        "type": "object",
        "properties": {
          "kind":          {"type": "string", "enum": %s, "description": "Evidence shape. direct = literal fact at file:line. conditional = behaviour gated by an IF clause. registration = something registered/bound with EXACT values. mechanism = how a process works step by step. relationship = link between two symbols (use subject + object). NOTE: for absence claims (searched and found nothing) do NOT emit via this tool — every kind requires a concrete file:line anchor, which is unsatisfiable for 'not found'. Use emit_investigation_complete.absence_justification for whole-answer absence, or simply omit the item for per-fact absence."},
          "subject":       {"type": "string", "description": "Primary semantic symbol the item is about (function name, type, key). For call-like predicates with anchor_kind='call', subject MUST be the caller / containing function at that line."},
          "predicate":     {"type": "string", "description": "Lowercase verb tying subject to object. PREFER these canonical verbs so the finalizer's deterministic relation-diagram renderer picks the edge up — anything outside this list is rendered as unstructured prose: calls, invokes, dispatches, delegates to, binds, binds ONLY, registers, wires, provides, returns, yields, constructs, instantiates, defines, implements, extends, embeds, maps, config, decorates. Optional; defaults to the lower-cased kind."},
          "object":        {"type": "string", "description": "Secondary symbol or value. Required for relationship; optional otherwise. For call-like predicates with anchor_kind='call', object MUST be the callee symbol on that line."},
          "source":        {"type": "string", "description": "Repository-relative file path the fact comes from. Required."},
          "line_start":    {"type": "integer", "description": "Exact gutter line number from read_file — NEVER estimated. The grounder uses this to verify the claim; wrong numbers are flagged as ungrounded or auto-recovered."},
          "line_end":      {"type": "integer", "description": "Last line of the cited range. Defaults to line_start when omitted."},
          "condition":     {"type": "string", "description": "For conditional items: the exact IF clause that triggers the behaviour."},
          "summary":       {"type": "string", "description": "Free-text rationale describing the fact. Keep concise; do not paraphrase numbers or string literals."},
          "context_role_hint": {"type": "string", "enum": %s, "description": "OPTIONAL recommendation for exact-target questions. defining = direct defining proof, absence_support = grounded evidence that helps justify why the exact target is absent but does NOT define it, related_context = grounded nearby context but not the exact target itself, illustrative_only = comment/doc/test/example mention that should NOT be treated as defining proof. The tool validates and may downgrade the hint."},
          "diagram_role_hint": {"type": "string", "enum": %s, "description": "OPTIONAL recommendation for config-precedence traces. default = code defaults, config = repo/user config-file layer (YAML/JSON/TOML/INI/etc.), runtime = code/runtime binding layer, override = CLI/high-precedence override layer. The tool validates and may ignore inconsistent hints."},
          "anchor_kind":   {"type": "string", "enum": %s, "description": "REQUIRED. What the line_start points at: 'definition' = symbol declaration, 'call' = function/method call site, 'condition' = if/when/switch/case/guard line, 'return' = return or yield, 'assignment' = := or = assignment, 'import' = import/use/require statement. The grounder dispatches on this so wrong kinds produce confusing ungrounded verdicts."},
          "anchor_symbol": {"type": "string", "description": "REQUIRED. The identifier the grounder should find on line_start. For a call like 'x.Execute()' the anchor_symbol is 'Execute'. For a type decl 'type Orchestrator struct' the anchor_symbol is 'Orchestrator'. For an import the anchor_symbol is the package path or local alias."},
          "snippet":       {"type": "string", "description": "Optional. 1-2 lines of actual code from the cited location. Enables snippet_fuzzy recovery when line_start is off by ±15 lines — recommended for conditional / mechanism / registration items."}
        },
        "required": ["kind", "source", "line_start", "anchor_kind", "anchor_symbol"]
      }
    }
  },
  "required": ["items"]
}`, string(evidenceKindEnumJSON), string(contextRoleEnumJSON), string(diagramRoleEnumJSON), string(anchorEnumJSON))
	return json.RawMessage(schema)
}

func (t *EmitEvidence) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	now := time.Now()
	if ctx == nil || ctx.Mutable == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_evidence requires BusContext.Mutable; the caller did not provide one (sub-agents are not supported)",
			Timestamp: now,
		}, nil
	}

	// Strict decode: unknown fields anywhere in the tree (top-level
	// or inside an item) are rejected so a hallucinated key (e.g.
	// 'evidence' instead of 'items', or 'note' instead of 'summary')
	// fails loudly at parse time rather than silently producing a
	// well-formed-looking item the parser quietly drops fields from.
	dec := json.NewDecoder(bytes.NewReader(params))
	dec.DisallowUnknownFields()
	var p emitEvidenceParams
	if err := dec.Decode(&p); err != nil {
		return failEmit(t.Name(), now, "invalid params: %v", err)
	}
	if len(p.Items) == 0 {
		return failEmit(t.Name(), now, "items is empty; emit at least one evidence object per call")
	}

	workDir := strings.TrimSpace(ctx.WorkDir)
	// Per-item validation. Earlier behaviour rejected the entire batch
	// on any single items[i] error — a one-letter typo dropped four
	// otherwise-valid evidence items (trace 1776439797257469553 iter=2:
	// items[3] had line_end < line_start and wiped items[0..2] + [4]).
	// New semantics:
	//   (1) line_end < line_start is auto-swapped — obvious transposition
	//       typo, warn but keep the item.
	//   (2) Other single-item validation errors skip just that item and
	//       surface the reason in the per-item feedback, UNLESS the
	//       cumulative reject ratio ≥ 50%. At that threshold the batch
	//       is structurally broken (not one typo) and the old "reject
	//       entire call" semantics apply so the LLM sees one failure
	//       envelope with ALL reasons, not a trickle of per-item notes.
	built := make([]types.EvidenceItem, 0, len(p.Items))
	rejectedItems := make([]string, 0)
	autoSwapped := make([]int, 0)
	// Session 11 R4 self-reference filter — pre-compute the
	// question's primary entity (first canonical entity on
	// AnalysisIR.RequestModel.AnalyzerHints.Entities). When an
	// emit_evidence item has subject == primary entity, predicate
	// ∈ {returns, equals}, and snippet contains the entity name
	// as a quoted literal, the item is self-referential and
	// should not compete with real answer chains. We keep the
	// item (no data lost) but zero its Confidence so downstream
	// rankers demote it, and we log a ViolSelfRefLiteral to the
	// ledger so F2 aggregator can weight it.
	primaryEntity := extractPrimaryEntity(ctx)
	exactResolutionContract := answerExactResolutionContract(ctx)
	pendingExactTargets := pendingExactResolutionTargets(ctx, exactResolutionContract)
	for i, in := range p.Items {
		ev, perr := buildEmitEvidenceItemWithSwap(&in, i, workDir, &autoSwapped)
		if perr != nil {
			rejectedItems = append(rejectedItems, fmt.Sprintf("items[%d]: %v", i, perr))
			continue
		}
		if primaryEntity != "" && isSelfRefEvidence(&ev, primaryEntity) {
			ev.Confidence = 0
			if ctx.Mutable != nil {
				ctx.Mutable.EvidenceClosure().AppendViolation(types.Violation{
					Kind:   types.ViolSelfRefLiteral,
					Detail: fmt.Sprintf("items[%d]: subject=%s matches primary entity; anchor literal is self-reference", i, ev.Subject),
					Stage:  string(types.StageExplore),
					SuspectedRoot: types.SuspectedRoot{
						IRField:    "answer_subject.kind",
						Reason:     "evidence subject=primary_entity with self-name literal",
						Confidence: 0.75,
					},
				})
			}
			logging.Debug("[emit_evidence] R4 self-ref trap: items[%d] subject=%s zeroed confidence", i, ev.Subject)
		}
		built = append(built, ev)
	}
	// Majority-reject gate: when half or more of the items failed,
	// return a hard failure with all reasons. Prevents the LLM from
	// "hiding" a poisoned batch behind one successful item.
	if len(rejectedItems) > 0 && len(rejectedItems)*2 >= len(p.Items) {
		return failEmit(t.Name(), now,
			"batch rejected: %d of %d items failed validation. Fix the following and re-emit:\n%s",
			len(rejectedItems), len(p.Items), strings.Join(rejectedItems, "\n"))
	}
	// Sparse rejects: if a handful failed but the batch is majority
	// healthy, keep going and stamp the rejections into the per-item
	// rendered Summary so the LLM sees the reason on the same turn.
	if len(built) == 0 {
		return failEmit(t.Name(), now,
			"no valid items after per-item validation:\n%s",
			strings.Join(rejectedItems, "\n"))
	}

	// Synchronous grounding. Each item is validated against the
	// read_file gutter index (Tier 1) and the repomap graph (Tier 2);
	// recovery tiers (shipped in Step 11) will additionally rewrite
	// near-miss LineStart/Source. The Report drives the per-item
	// feedback in the tool Summary so the LLM sees grounded /
	// recovered / ungrounded verdicts in the same turn it emitted them.
	gc := ground.BuildContext(ctx)
	diagramRequiredFiles := exactResolutionDiagramRequiredFiles(ctx, exactResolutionContract)
	reports := make([]ground.Report, len(built))
	for i := range built {
		r := ground.GroundItem(&built[i], gc)
		normalizeCallEvidenceDirection(&built[i], gc)
		requestedDiagramRole := built[i].DiagramRole
		built[i].ContextRole = validatedEvidenceContextRole(built[i], gc, exactResolutionContract)
		built[i].DiagramRole = validatedEvidenceDiagramRole(built[i], gc, exactResolutionContract, diagramRequiredFiles)
		if stabilizeExactResolutionEvidence(&built[i], gc, exactResolutionContract, pendingExactTargets) {
			r.Status = built[i].GroundingStatus
			r.Tier = built[i].GroundingTier
			r.Note = built[i].GroundingNote
		}
		if stabilizeIllustrativeEvidence(&built[i]) {
			r.Status = built[i].GroundingStatus
			r.Tier = built[i].GroundingTier
			r.Note = built[i].GroundingNote
		}
		if appendDiagramRoleValidationNote(&built[i], requestedDiagramRole, exactResolutionContract, diagramRequiredFiles) {
			r.Status = built[i].GroundingStatus
			r.Tier = built[i].GroundingTier
			r.Note = built[i].GroundingNote
		}
		// Recovery can rewrite LineStart/Source; keep the stable ID
		// in sync so merge-by-ID downstream coalesces correctly.
		built[i].ID = types.StableEvidenceID(
			built[i].Kind, built[i].Subject, built[i].Predicate,
			built[i].Object, built[i].Condition, built[i].Source,
			built[i].LineStart, built[i].LineEnd,
		)
		r.ItemID = built[i].ID
		reports[i] = r
	}

	// Session-24 codename-grounding filter.
	//
	// The explorer-side peer of the emit_answer_document gate. u3a
	// run-2 of 2026-04-22 showed explorer.go:iter=7 emitting an
	// evidence item with summary="S2 终止条件：..." where the cited
	// line range (explorer.go:2432) contains zero `S2` substring —
	// a pattern-completion fabrication sitting inside a structurally
	// correct evidence anchor. extract/finalize happened to filter
	// that item out downstream in that run, but the next run could
	// just as easily propagate it. Reject here so the LLM re-emits
	// the item without the invented label rather than letting the
	// fabrication travel through the pipeline.
	//
	// Uses the same LineIndex (gc) already built above, so no extra
	// I/O. The item's own Source/LineStart/LineEnd IS the cited
	// window — narrower than emit_answer_document's citations-pool
	// check because each evidence item is a single-anchor claim.
	filteredBuilt := built[:0]
	filteredReports := reports[:0]
	for i := range built {
		if err := validateEvidenceSummaryCodenameGrounding(&built[i], gc); err != nil {
			rejectedItems = append(rejectedItems, fmt.Sprintf("items[id=%s]: %v", built[i].ID, err))
			continue
		}
		filteredBuilt = append(filteredBuilt, built[i])
		filteredReports = append(filteredReports, reports[i])
	}
	built = filteredBuilt
	reports = filteredReports
	if len(built) == 0 {
		return failEmit(t.Name(), now,
			"all items rejected post-grounding (codename-grounding gate):\n%s",
			strings.Join(rejectedItems, "\n"))
	}

	ctx.Mutable.AppendEvidence(built)

	summary := renderEmitSummary(ctx, built, reports, ctx.Mutable.EmittedEvidence())
	if len(rejectedItems) > 0 || len(autoSwapped) > 0 {
		var b strings.Builder
		b.WriteString(summary)
		if len(rejectedItems) > 0 {
			fmt.Fprintf(&b, "\n%d item(s) were SKIPPED due to validation errors and are NOT in the accepted buffer:\n",
				len(rejectedItems))
			for _, r := range rejectedItems {
				fmt.Fprintf(&b, "  - %s\n", r)
			}
			b.WriteString("Re-emit these with corrected fields if they are load-bearing.\n")
		}
		if len(autoSwapped) > 0 {
			fmt.Fprintf(&b, "\n%d item(s) had line_end < line_start (likely typo) and were AUTO-SWAPPED; double-check the range was what you intended: ",
				len(autoSwapped))
			var parts []string
			for _, idx := range autoSwapped {
				parts = append(parts, fmt.Sprintf("items[%d]", idx))
			}
			b.WriteString(strings.Join(parts, ", ") + "\n")
		}
		summary = b.String()
	}
	repair := buildEmitEvidenceRepair(ctx, built, reports)
	return types.ToolResult{
		ToolName:  t.Name(),
		Repair:    repair,
		Success:   true,
		Summary:   summary,
		Timestamp: now,
	}, nil
}

// buildEmitEvidenceItemWithSwap wraps buildEmitEvidenceItem with the
// 2026-04-17 line_end<line_start auto-swap. If the decoded item has
// line_end < line_start AND line_end > 0, swap the two values before
// delegating. Records the item index into autoSwapped so the caller
// can surface a "double-check the range" warning. All other
// validation errors flow through buildEmitEvidenceItem unchanged.
func buildEmitEvidenceItemWithSwap(in *emitEvidenceItem, index int, workDir string, autoSwapped *[]int) (types.EvidenceItem, error) {
	if in.LineStart.Int() > 0 && in.LineEnd.Int() > 0 && in.LineEnd.Int() < in.LineStart.Int() {
		// Obvious transposition typo — repair rather than reject.
		swappedStart, swappedEnd := in.LineEnd, in.LineStart
		in.LineStart, in.LineEnd = swappedStart, swappedEnd
		*autoSwapped = append(*autoSwapped, index)
	}
	return buildEmitEvidenceItem(*in, index, workDir)
}

// buildEmitEvidenceItem validates a single decoded item and converts
// it into a types.EvidenceItem with stable ID and producer stamped.
// All validation is structural, never wordlist-based.
func buildEmitEvidenceItem(in emitEvidenceItem, index int, workDir string) (types.EvidenceItem, error) {
	kindText := strings.TrimSpace(in.EvidenceKind)
	if kindText == "" {
		kindText = strings.TrimSpace(in.LegacyKind)
	}
	kindKey := strings.ToLower(kindText)
	if _, anchorNameCollision := emitAnchorKinds[kindKey]; anchorNameCollision {
		return types.EvidenceItem{}, fmt.Errorf(
			"items[%d]: evidence_kind %q is invalid because %q is an anchor_kind, not an evidence_kind. Use evidence_kind in {%s} and anchor_kind in {%s}.",
			index,
			kindText,
			kindText,
			strings.Join(emitEvidenceAllowedKindNames(), ", "),
			strings.Join(emitAnchorKindNames(), ", "),
		)
	}
	kind, ok := emitEvidenceAllowedKinds[kindKey]
	if !ok {
		// Schema-deprecation redirect (logtri_custom root cause):
		// kind=absent used to live in the tool's enum but the
		// validator always required line_start > 0 + anchor_kind +
		// anchor_symbol, which an "I searched and found nothing"
		// claim cannot satisfy. Rather than emit a bland "unknown
		// kind" message that sends the LLM into retry-loop territory
		// (trace 2026-04-21 logtri_custom: 10+ iters before the LLM
		// reverse-engineered a hacky workaround), point it at the
		// proper absence channel directly.
		if kindKey == "absent" {
			return types.EvidenceItem{}, fmt.Errorf(
				"items[%d]: kind=absent is not emittable via emit_evidence — the tool requires line_start > 0 + anchor_kind + anchor_symbol for every evidence item, which is unsatisfiable for 'searched and found nothing' claims. For whole-answer absence (the user's question resolves to 'no X exists'), set `absence_justification` on emit_investigation_complete with a one-sentence explanation and skip emit_evidence for the zero result. For per-fact absence inside a larger investigation, simply omit the item — the finalizer's summary is the right place to describe what was absent. Allowed kinds via emit_evidence: %s.",
				index, strings.Join(emitEvidenceAllowedKindNames(), ", "))
		}
		return types.EvidenceItem{}, fmt.Errorf("items[%d]: unknown evidence_kind %q (allowed: %s)", index, kindText, strings.Join(emitEvidenceAllowedKindNames(), ", "))
	}
	source := strings.TrimSpace(in.Source)
	if source == "" {
		return types.EvidenceItem{}, fmt.Errorf("items[%d]: source is required", index)
	}
	if !emitLooksLikePath(source) {
		return types.EvidenceItem{}, fmt.Errorf("items[%d]: source %q does not look like a repo-relative file path", index, in.Source)
	}
	// Blob-file path leak gate (UNRESOLVED bug N=1, see
	// memory/project_blob_file_leak_unresolved.md). The same
	// structural prefix check ships in emit_answer_symbol; both tools
	// ride on isInsideWorkDir from emit_answer_symbol.go.
	if isInsideWorkDir(source, workDir) {
		return types.EvidenceItem{}, fmt.Errorf("items[%d]: source %q lives inside the per-trace WorkDir (%s) — that is a tool-output blob, not a repo file. Re-cite the original repo path that the blob was extracted from.", index, in.Source, workDir)
	}
	preAnchorKindKey := strings.ToLower(strings.TrimSpace(in.AnchorKind))
	if _, evidenceNameCollision := emitEvidenceAllowedKinds[preAnchorKindKey]; evidenceNameCollision {
		return types.EvidenceItem{}, fmt.Errorf(
			"items[%d]: anchor_kind %q is invalid because %q belongs to evidence_kind, not anchor_kind. Use evidence_kind=%q and anchor_kind in {%s}.",
			index,
			strings.TrimSpace(in.AnchorKind),
			strings.TrimSpace(in.AnchorKind),
			strings.TrimSpace(in.AnchorKind),
			strings.Join(emitAnchorKindNames(), ", "),
		)
	}
	preAnchorSymbol := strings.TrimSpace(in.AnchorSymbol)
	if preAnchorSymbol == "" {
		return types.EvidenceItem{}, fmt.Errorf("items[%d]: anchor_symbol is required 鈥?the identifier the grounder should find at source:line_start (e.g. the callee name for anchor_kind=call, the type name for anchor_kind=definition, the package path for anchor_kind=import)", index)
	}
	lineStartN := in.LineStart.Int()
	lineEndN := in.LineEnd.Int()
	if lineStartN <= 0 {
		return types.EvidenceItem{}, fmt.Errorf("items[%d]: line_start is required and must be > 0 (emit the exact gutter line from read_file, never estimate)", index)
	}
	if lineEndN < 0 {
		return types.EvidenceItem{}, fmt.Errorf("items[%d]: line_end must be >= 0", index)
	}
	if lineEndN > 0 && lineEndN < lineStartN {
		return types.EvidenceItem{}, fmt.Errorf("items[%d]: line_end (%d) is before line_start (%d)", index, lineEndN, lineStartN)
	}
	if kind == types.EvidenceRelationship && strings.TrimSpace(in.Object) == "" {
		return types.EvidenceItem{}, fmt.Errorf("items[%d]: relationship items require object", index)
	}

	anchorKindKey := strings.ToLower(strings.TrimSpace(in.AnchorKind))
	if anchorKindKey == "" {
		return types.EvidenceItem{}, fmt.Errorf("items[%d]: anchor_kind is required (one of: %s)", index, strings.Join(emitAnchorKindNames(), ", "))
	}
	anchorKind, ok := findAnchorKind(anchorKindKey)
	if !ok {
		if _, evidenceNameCollision := emitEvidenceAllowedKinds[anchorKindKey]; evidenceNameCollision {
			return types.EvidenceItem{}, fmt.Errorf(
				"items[%d]: anchor_kind %q is invalid because %q belongs to evidence_kind, not anchor_kind. Use evidence_kind=%q and anchor_kind in {%s}.",
				index,
				strings.TrimSpace(in.AnchorKind),
				strings.TrimSpace(in.AnchorKind),
				strings.TrimSpace(in.AnchorKind),
				strings.Join(emitAnchorKindNames(), ", "),
			)
		}
		return types.EvidenceItem{}, fmt.Errorf("items[%d]: unknown anchor_kind %q (allowed: %s)", index, in.AnchorKind, strings.Join(emitAnchorKindNames(), ", "))
	}
	anchorSymbol := strings.TrimSpace(in.AnchorSymbol)
	if anchorSymbol == "" {
		return types.EvidenceItem{}, fmt.Errorf("items[%d]: anchor_symbol is required — the identifier the grounder should find at source:line_start (e.g. the callee name for anchor_kind=call, the type name for anchor_kind=definition, the package path for anchor_kind=import)", index)
	}

	predicate := strings.ToLower(strings.TrimSpace(in.Predicate))
	if predicate == "" {
		predicate = string(kind)
	}
	subject := strings.TrimSpace(in.Subject)
	object := strings.TrimSpace(in.Object)
	condition := strings.TrimSpace(in.Condition)
	summary := strings.TrimSpace(in.Summary)
	snippet := strings.TrimSpace(in.Snippet)
	contextRole, err := parseEvidenceContextRoleHint(index, in.ContextRole)
	if err != nil {
		return types.EvidenceItem{}, err
	}
	diagramRole, err := parseEvidenceDiagramRoleHint(index, in.DiagramRole)
	if err != nil {
		return types.EvidenceItem{}, err
	}
	lineStart := lineStartN
	lineEnd := lineEndN
	if lineEnd == 0 {
		lineEnd = lineStart
	}

	item := types.EvidenceItem{
		Kind:         kind,
		Subject:      subject,
		Predicate:    predicate,
		Object:       object,
		Summary:      summary,
		Condition:    condition,
		Source:       source,
		LineStart:    lineStart,
		LineEnd:      lineEnd,
		Confidence:   0.78, // matches parseEvidenceLine's confidence floor
		Producer:     EmitEvidenceProducer,
		ContextRole:  contextRole,
		DiagramRole:  diagramRole,
		AnchorKind:   anchorKind,
		AnchorSymbol: anchorSymbol,
		Snippet:      snippet,
	}
	item.ID = types.StableEvidenceID(item.Kind, item.Subject, item.Predicate, item.Object, item.Condition, item.Source, item.LineStart, item.LineEnd)
	return item, nil
}

func parseEvidenceContextRoleHint(index int, raw string) (types.EvidenceContextRole, error) {
	role := types.EvidenceContextRole(strings.ToLower(strings.TrimSpace(raw)))
	if role == "" {
		return types.EvidenceContextRoleUnknown, nil
	}
	if role.IsValid() && role != types.EvidenceContextRoleUnknown {
		return role, nil
	}
	return types.EvidenceContextRoleUnknown, fmt.Errorf(
		"items[%d]: unknown context_role_hint %q (allowed: %s)",
		index, raw, strings.Join(emitEvidenceContextRoleNames(), ", "))
}

func parseEvidenceDiagramRoleHint(index int, raw string) (types.EvidenceDiagramRole, error) {
	roleText := strings.ToLower(strings.TrimSpace(raw))
	switch roleText {
	case "yaml":
		roleText = string(types.EvidenceDiagramRoleConfig)
	}
	role := types.EvidenceDiagramRole(roleText)
	if role == "" {
		return types.EvidenceDiagramRoleUnknown, nil
	}
	if role.IsValid() && role != types.EvidenceDiagramRoleUnknown {
		return role, nil
	}
	return types.EvidenceDiagramRoleUnknown, fmt.Errorf(
		"items[%d]: unknown diagram_role_hint %q (allowed: %s)",
		index, raw, strings.Join(emitEvidenceDiagramRoleNames(), ", "))
}

func validatedEvidenceContextRole(ev types.EvidenceItem, gc *ground.Context, contract *types.ExactResolutionContract) types.EvidenceContextRole {
	if evidenceLooksIllustrative(ev, gc) {
		return types.EvidenceContextRoleIllustrativeOnly
	}
	switch ev.ContextRole {
	case types.EvidenceContextRoleDefining:
		if evidenceCanBeDefining(ev) {
			return ev.ContextRole
		}
	case types.EvidenceContextRoleAbsenceSupport:
		if evidenceCanBeAbsenceSupport(ev, contract) {
			return ev.ContextRole
		}
	case types.EvidenceContextRoleRelatedContext:
		if evidenceCanBeRelatedContext(ev) {
			return ev.ContextRole
		}
	case types.EvidenceContextRoleIllustrativeOnly:
		if evidenceLooksIllustrative(ev, gc) {
			return ev.ContextRole
		}
	}
	return types.EvidenceContextRoleUnknown
}

func validatedEvidenceDiagramRole(ev types.EvidenceItem, gc *ground.Context, contract *types.ExactResolutionContract, requiredFiles []string) types.EvidenceDiagramRole {
	if evidenceLooksIllustrative(ev, gc) {
		return types.EvidenceDiagramRoleUnknown
	}
	switch ev.DiagramRole {
	case types.EvidenceDiagramRoleConfig:
		if types.ConfigTraceDiagramRoleAnchorCompatible(ev.DiagramRole, ev) &&
			types.LooksLikeConfigFilePath(ev.Source) {
			return ev.DiagramRole
		}
	case types.EvidenceDiagramRoleOverride:
		if types.ConfigTraceDiagramRoleAnchorCompatible(ev.DiagramRole, ev) &&
			evidenceCanBeDiagramCodeLayer(ev, contract, requiredFiles) {
			return ev.DiagramRole
		}
	case types.EvidenceDiagramRoleRuntime:
		if types.ConfigTraceDiagramRoleAnchorCompatible(ev.DiagramRole, ev) &&
			evidenceCanBeDiagramCodeLayer(ev, contract, requiredFiles) {
			return ev.DiagramRole
		}
	case types.EvidenceDiagramRoleDefault:
		if types.ConfigTraceDiagramRoleAnchorCompatible(ev.DiagramRole, ev) &&
			evidenceCanBeDiagramCodeLayer(ev, contract, requiredFiles) &&
			!types.LooksLikeConfigFilePath(ev.Source) {
			return ev.DiagramRole
		}
	}
	return types.EvidenceDiagramRoleUnknown
}

func appendDiagramRoleValidationNote(ev *types.EvidenceItem, requested types.EvidenceDiagramRole, contract *types.ExactResolutionContract, requiredFiles []string) bool {
	if ev == nil || requested == types.EvidenceDiagramRoleUnknown || requested == ev.DiagramRole {
		return false
	}
	note := ""
	switch requested {
	case types.EvidenceDiagramRoleConfig:
		if types.LooksLikeAuxiliaryEvidencePath(ev.Source) {
			note = "diagram_role_hint=config was ignored because doc/test/example anchors are never precedence-grade config-file evidence. Keep this item as context only."
		} else if !types.LooksLikeConfigFilePath(ev.Source) {
			note = "diagram_role_hint=config was ignored because `config` is only for grounded config-file anchors (YAML/JSON/TOML/INI/etc.). This code anchor may stay prose-only context, or be re-emitted with `default` / `runtime` / `override` only if the anchored line itself proves that code-layer role."
		}
	case types.EvidenceDiagramRoleDefault, types.EvidenceDiagramRoleRuntime, types.EvidenceDiagramRoleOverride:
		if types.LooksLikeAuxiliaryEvidencePath(ev.Source) {
			note = fmt.Sprintf("diagram_role_hint=%s was ignored because doc/test/example anchors are never code-layer precedence evidence. Keep this item as context only.", requested)
		} else if types.LooksLikeConfigFilePath(ev.Source) {
			note = fmt.Sprintf("diagram_role_hint=%s was ignored because `%s` is only for grounded code-layer anchors. Config-file evidence should use `diagram_role_hint=config`.", requested, requested)
		} else if len(requiredFiles) > 0 && !emitSourceMatchesAnyRequiredFile(ev.Source, requiredFiles) {
			note = fmt.Sprintf("diagram_role_hint=%s was ignored because this anchor is outside the current same-scope required files for the exact-target context. Keep it as nearby context only.", requested)
		}
	}
	if note == "" {
		note = fmt.Sprintf("diagram_role_hint=%s was ignored because the anchored line does not structurally prove that precedence role. Keep it as prose-only grounded context, or re-emit with a role that the anchored line itself demonstrates.", requested)
	}
	if contract != nil && contract.TargetKind == types.SubjectConfigKey && types.ExactResolutionEvidenceCanSatisfyRelatedContext(contract, *ev, requiredFiles) {
		note += " The current exact target is still unresolved, so nearby same-scope anchors can stay on the answer surface only as context until a validated precedence role is grounded."
	}
	return appendGroundingNoteOnce(ev, note)
}

func evidenceLooksIllustrative(ev types.EvidenceItem, gc *ground.Context) bool {
	if types.LooksLikeAuxiliaryEvidencePath(ev.Source) {
		return true
	}
	if gc == nil || len(gc.LineIndex) == 0 || ev.Source == "" || ev.LineStart <= 0 {
		return false
	}
	fileLines, ok := gc.LineIndex[ev.Source]
	if !ok {
		return false
	}
	return ground.LineLooksCommentOnly(fileLines, ev.LineStart, ev.Source)
}

func evidenceCanBeDefining(ev types.EvidenceItem) bool {
	if ev.Source == "" {
		return false
	}
	switch ev.AnchorKind {
	case types.AnchorDefinition, types.AnchorAssignment, types.AnchorImport, types.AnchorReturn:
		return true
	default:
		return false
	}
}

func evidenceCanBeRelatedContext(ev types.EvidenceItem) bool {
	return ev.Source != ""
}

func evidenceCanBeAbsenceSupport(ev types.EvidenceItem, contract *types.ExactResolutionContract) bool {
	if !evidenceCanBeRelatedContext(ev) {
		return false
	}
	if contract == nil {
		return false
	}
	if types.ExactResolutionDirectAnchorMatchesAnyTarget(contract, ev.Subject, ev.AnchorSymbol, ev.Object) {
		return false
	}
	return types.ExactResolutionTextsMentionAnyTarget(contract,
		ev.Subject, ev.Predicate, ev.Object, ev.AnchorSymbol, ev.Condition, ev.Snippet, ev.Summary)
}

func evidenceCanBeDiagramCodeLayer(ev types.EvidenceItem, contract *types.ExactResolutionContract, requiredFiles []string) bool {
	if ev.Source == "" || types.LooksLikeConfigFilePath(ev.Source) {
		return false
	}
	if types.LooksLikeAuxiliaryEvidencePath(ev.Source) {
		return false
	}
	if ev.AnchorKind == "" {
		return false
	}
	if contract == nil || contract.TargetKind != types.SubjectConfigKey {
		return true
	}
	if len(requiredFiles) > 0 && !emitSourceMatchesAnyRequiredFile(ev.Source, requiredFiles) {
		return false
	}
	terms := types.ExactResolutionContextTerms(contract)
	if !types.ExactResolutionTextsMentionAnyTarget(contract,
		ev.Subject, ev.Predicate, ev.Object, ev.AnchorSymbol, ev.Condition, ev.Snippet, ev.Summary) &&
		!types.EvidenceItemStructurallyMentionsAnyTerm(ev, terms) {
		return false
	}
	return ev.ContextRole != types.EvidenceContextRoleIllustrativeOnly &&
		ev.ContextRole != types.EvidenceContextRoleAbsenceSupport
}

var callLikePredicates = map[string]bool{
	"call":         true,
	"calls":        true,
	"invoke":       true,
	"invokes":      true,
	"dispatch":     true,
	"dispatches":   true,
	"delegates":    true,
	"delegates to": true,
}

// normalizeCallEvidenceDirection canonicalises call-site evidence to
// the single semantic direction the rest of the pipeline expects:
// caller -> callee. The LLM occasionally inverts this
// (subject=callee, object=caller) because the visible callee name is
// more salient than the containing function. We already have the
// authoritative callsite and symbol windows in the grounding graph, so
// reuse that existing structure here instead of asking downstream
// stages to infer direction from prose.
func normalizeCallEvidenceDirection(it *types.EvidenceItem, gc *ground.Context) bool {
	if it == nil || gc == nil || gc.Graph == nil || it.AnchorKind != types.AnchorCall {
		return false
	}
	if !callLikePredicates[strings.ToLower(strings.TrimSpace(it.Predicate))] {
		return false
	}
	fi, ok := gc.Graph.FileIndex[it.Source]
	if !ok || fi == nil {
		return false
	}
	rel, ok := findCallRelationAtLine(fi, it.LineStart, it.AnchorSymbol)
	if !ok {
		return false
	}
	caller := enclosingCallableSymbolName(fi, it.LineStart)
	callee := callRelationTargetName(gc.Graph, fi, rel)
	if caller == "" || callee == "" {
		return false
	}
	changed := false
	if strings.TrimSpace(it.Subject) != caller {
		it.Subject = caller
		changed = true
	}
	if strings.TrimSpace(it.Object) != callee {
		it.Object = callee
		changed = true
	}
	if changed {
		logging.Debug("[emit_evidence] normalized call direction at %s:%d -> %s %s %s",
			it.Source, it.LineStart, it.Subject, it.Predicate, it.Object)
	}
	return changed
}

func findCallRelationAtLine(fi *repomap.FileInfo, line int, anchorSymbol string) (*repomap.Relation, bool) {
	if fi == nil || line <= 0 {
		return nil, false
	}
	anchorSymbol = strings.TrimSpace(anchorSymbol)
	shortAnchor := emitLastDotSegment(anchorSymbol)
	var fallback *repomap.Relation
	for i := range fi.Relations {
		rel := &fi.Relations[i]
		if rel.Kind != "call" || rel.Line != line {
			continue
		}
		if fallback == nil {
			fallback = rel
		}
		relName := strings.TrimSpace(rel.ToEP.Name)
		if relName == "" {
			relName = strings.TrimSpace(rel.To)
		}
		if anchorSymbol == "" || relName == anchorSymbol || relName == shortAnchor {
			return rel, true
		}
	}
	if anchorSymbol == "" && fallback != nil {
		return fallback, true
	}
	return nil, false
}

func enclosingCallableSymbolName(fi *repomap.FileInfo, line int) string {
	if fi == nil || line <= 0 {
		return ""
	}
	var best *repomap.Symbol
	bestPriority := math.MaxInt
	bestSpan := math.MaxInt
	for i := range fi.Symbols {
		sym := &fi.Symbols[i]
		if sym.Line <= 0 {
			continue
		}
		end := sym.EndLine
		if end < sym.Line {
			end = sym.Line
		}
		if line < sym.Line || line > end {
			continue
		}
		priority := 1
		if sym.Kind == "function" || sym.Kind == "method" {
			priority = 0
		}
		span := end - sym.Line
		if best == nil || priority < bestPriority || (priority == bestPriority && span < bestSpan) {
			best = sym
			bestPriority = priority
			bestSpan = span
		}
	}
	return qualifiedEvidenceSymbolName(best)
}

func callRelationTargetName(graph *repomap.Graph, fi *repomap.FileInfo, rel *repomap.Relation) string {
	if rel == nil {
		return ""
	}
	if graph != nil && fi != nil {
		if target := graph.ResolveCallTarget(fi, *rel); target != nil {
			if name := qualifiedEvidenceSymbolName(target); name != "" {
				return name
			}
		}
	}
	name := strings.TrimSpace(rel.ToEP.Name)
	if recv := strings.TrimSpace(rel.ToEP.Receiver); recv != "" && name != "" {
		return recv + "." + name
	}
	if name != "" {
		return name
	}
	return strings.TrimSpace(rel.To)
}

func qualifiedEvidenceSymbolName(sym *repomap.Symbol) string {
	if sym == nil {
		return ""
	}
	switch {
	case strings.TrimSpace(sym.Receiver) != "":
		return strings.TrimSpace(sym.Receiver) + "." + strings.TrimSpace(sym.Name)
	case strings.TrimSpace(sym.Parent) != "":
		return strings.TrimSpace(sym.Parent) + "." + strings.TrimSpace(sym.Name)
	default:
		return strings.TrimSpace(sym.Name)
	}
}

func emitLastDotSegment(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.LastIndex(s, "."); idx >= 0 && idx+1 < len(s) {
		return s[idx+1:]
	}
	return s
}

// findAnchorKind lookups an AnchorKind by its lowercase string form.
// Mirrors the EvidenceKind dispatch above and keeps the allowed list
// in lockstep with types.AllAnchorKinds().
func findAnchorKind(key string) (types.AnchorKind, bool) {
	for _, k := range types.AllAnchorKinds() {
		if strings.ToLower(string(k)) == key {
			return k, true
		}
	}
	return "", false
}

// emitLooksLikePath is the tool-package twin of internal/agent's
// looksLikePath. Duplicated rather than imported because internal/agent
// already imports internal/tool — the reverse import would be a cycle.
// Both functions implement the same predicate: the string contains a
// '/' or '.', so it shapes like a repo-relative path.
func emitLooksLikePath(s string) bool {
	if s == "" {
		return false
	}
	return strings.Contains(s, "/") || strings.Contains(s, ".")
}

func canonicalEmitPath(s string) string {
	s = filepath.ToSlash(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	return path.Clean(s)
}

// evidenceSemantic renders the semantic payload of an evidence item
// ("Subject Predicate Object" when all present, else Summary, else
// empty). Called by renderEmitSummary so the emit_evidence tool
// dump surfaces the actual CLAIM each item makes. Prior to this
// helper the dump hid Object entirely — an item like
// `{Subject: "Register", Predicate: "registers", Object: "explore-skill"}`
// rendered only as `registration Register @ defaults.go:11`, losing
// the one token (`explore-skill`) that answered the user's question.
func evidenceSemantic(it types.EvidenceItem) string {
	if s := strings.TrimSpace(it.Summary); s != "" {
		return s
	}
	parts := make([]string, 0, 3)
	if t := strings.TrimSpace(it.Subject); t != "" {
		parts = append(parts, t)
	}
	if t := strings.TrimSpace(it.Predicate); t != "" {
		parts = append(parts, t)
	}
	if t := strings.TrimSpace(it.Object); t != "" {
		// Quote Object when it looks like a literal (contains
		// hyphen / dot / space — would-be shell / token chars).
		if strings.ContainsAny(t, "-. ") {
			parts = append(parts, fmt.Sprintf("%q", t))
		} else {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " ")
}

// renderEmitSummary builds the per-item + global grounding feedback
// the LLM sees in the same turn it emitted the batch. This is the
// core of the "one emit = one closed loop" contract: the LLM does not
// have to wait until a later stage to learn which items grounded,
// recovered, or failed.
//
// Format:
//
//	emit_evidence accepted N item(s)
//	  [1] <kind> <anchor_symbol> @ <source>:<line>
//	      → grounded (tier=line_text)
//	  [2] <kind> <anchor_symbol> @ <source>:<line>
//	      → recovered (tier=..., LLM claimed line X, adjusted to Y)
//	  [3] <kind> <anchor_symbol> @ <source>:<line>
//	      → ungrounded: <reason>
//	          fix: (A) read_file ...; (B) re-emit with different anchor_symbol; (C) drop if speculative
//
//	Evidence so far: G grounded / R recovered / U ungrounded across N files.
//
// allEvidence is the mutable buffer after Append, used for the global
// tally so the LLM sees cumulative state across multiple emit_evidence
// calls in the same dispatch.
func renderEmitSummary(ctx *types.BusContext, items []types.EvidenceItem, reports []ground.Report, allEvidence []types.EvidenceItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "emit_evidence accepted %d item(s)\n\n", len(items))
	for i, it := range items {
		r := reports[i]
		line := it.LineStart
		// Surface the semantic payload (Subject Predicate Object)
		// alongside the anchor so the LLM — especially in the
		// finalizer stage reading this dump from prior tool history
		// — sees WHAT the evidence claims, not just WHERE it lives.
		// Before this format change the line read "[2] registration
		// Register @ defaults.go:11" and the LLM had no way to know
		// that Object="explore-skill" (the literal answer).
		semantic := evidenceSemantic(it)
		if semantic != "" {
			fmt.Fprintf(&b, "  [%d] %s %s @ %s:%d — %s\n",
				i+1, it.Kind, prefOrDash(it.AnchorSymbol), it.Source, line, semantic)
		} else {
			fmt.Fprintf(&b, "  [%d] %s %s @ %s:%d\n",
				i+1, it.Kind, prefOrDash(it.AnchorSymbol), it.Source, line)
		}
		switch it.GroundingStatus {
		case types.GroundingGrounded:
			fmt.Fprintf(&b, "      → grounded (tier=%s)\n", it.GroundingTier)
			if note := strings.TrimSpace(it.GroundingNote); note != "" {
				fmt.Fprintf(&b, "        note: %s\n", note)
			}
		case types.GroundingRecovered:
			if r.OriginalLine != 0 && r.OriginalLine != r.AdjustedLine {
				fmt.Fprintf(&b, "      → recovered (tier=%s, you claimed line %d, adjusted to %d)\n",
					it.GroundingTier, r.OriginalLine, r.AdjustedLine)
			} else {
				fmt.Fprintf(&b, "      → recovered (tier=%s)\n", it.GroundingTier)
			}
			if note := strings.TrimSpace(it.GroundingNote); note != "" {
				fmt.Fprintf(&b, "        %s\n", note)
			}
		case types.GroundingUngrounded:
			note := strings.TrimSpace(it.GroundingNote)
			if note == "" {
				note = "no tier accepted the citation"
			}
			fmt.Fprintf(&b, "      → ungrounded: %s\n", note)
			if evidenceRepairShouldDrop(it) {
				fmt.Fprintf(&b, "        fix: drop the item; do NOT spend read_file budget repairing this non-defining mention\n")
			} else {
				fmt.Fprintf(&b, "        fix: (A) read_file %s near line %d  (B) re-emit with a different anchor_symbol  (C) drop the item if it was speculative\n", it.Source, line)
			}
		}
	}
	// Global tally across this dispatch's accumulated emit_evidence.
	var g, rc, u int
	sources := make(map[string]struct{})
	for _, e := range allEvidence {
		sources[e.Source] = struct{}{}
		switch e.GroundingStatus {
		case types.GroundingGrounded:
			g++
		case types.GroundingRecovered:
			rc++
		case types.GroundingUngrounded:
			u++
		}
	}
	srcList := make([]string, 0, len(sources))
	for s := range sources {
		srcList = append(srcList, s)
	}
	sort.Strings(srcList)
	fmt.Fprintf(&b, "\nEvidence so far: %d grounded / %d recovered / %d ungrounded across %d file(s).\n",
		g, rc, u, len(srcList))
	if shouldNudgeDiagramRoleHints(ctx, items) {
		b.WriteString("Config-precedence task detected: when an evidence item represents code defaults, a config-file layer (YAML/JSON/TOML/INI/etc.), a runtime binding layer, or a high-precedence override layer, set `diagram_role_hint` on that item so downstream diagram rendering can reuse validated structure instead of inferring roles from prose.\n")
	}
	return b.String()
}

func shouldNudgeDiagramRoleHints(ctx *types.BusContext, items []types.EvidenceItem) bool {
	if ctx == nil || ctx.AnalysisIR == nil || len(items) == 0 {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.Scenario != types.ScenarioConfigTrace &&
		!strings.EqualFold(strings.TrimSpace(rm.AnalyzerHints.Kind), "config_mapping") &&
		rm.AnswerSubject.Kind != types.SubjectConfigKey {
		return false
	}
	for _, it := range items {
		if it.DiagramRole != types.EvidenceDiagramRoleUnknown {
			return false
		}
	}
	return true
}

func prefOrDash(s string) string {
	if s = strings.TrimSpace(s); s != "" {
		return s
	}
	return "-"
}

func evidenceRepairShouldDrop(it types.EvidenceItem) bool {
	note := strings.ToLower(strings.TrimSpace(it.GroundingNote))
	return strings.Contains(note, "do not repair this item") || strings.Contains(note, "non-defining proof")
}

func buildEmitEvidenceRepair(ctx *types.BusContext, items []types.EvidenceItem, reports []ground.Report) *types.ToolRepair {
	targets := emitEvidenceRepairTargets(items, reports)
	if len(targets) == 0 {
		return nil
	}
	return &types.ToolRepair{
		Code:    "evidence_line_text_repair",
		Hint:    renderEmitEvidenceRepairToolHint(targets),
		Targets: targets,
		Metadata: map[string]string{
			"repair_scope": "line_text_grounding",
			"repair_stage": "explorer",
		},
	}
}

type groundedRepairCarrier struct {
	line  int
	tails map[string]bool
}

func emitEvidenceRepairTargets(items []types.EvidenceItem, reports []ground.Report) []types.ToolRepairTarget {
	type bucket struct {
		order []int
		seen  map[int]bool
	}
	byFile := make(map[string]*bucket)
	groundedByFile := make(map[string][]groundedRepairCarrier)
	for _, it := range items {
		if it.Source == "" || it.LineStart <= 0 || it.GroundingStatus != types.GroundingGrounded {
			continue
		}
		file := canonicalEmitPath(it.Source)
		if file == "" {
			file = it.Source
		}
		tails := make(map[string]bool)
		for _, tail := range types.EvidenceSurfaceSymbolTails(it) {
			tails[tail] = true
		}
		groundedByFile[file] = append(groundedByFile[file], groundedRepairCarrier{
			line:  it.LineStart,
			tails: tails,
		})
	}
	for i, it := range items {
		if it.Source == "" || it.LineStart <= 0 {
			continue
		}
		switch it.GroundingStatus {
		case types.GroundingRecovered, types.GroundingUngrounded:
		default:
			continue
		}
		if evidenceRepairShouldDrop(it) {
			continue
		}
		file := canonicalEmitPath(it.Source)
		if file == "" {
			file = it.Source
		}
		line := it.LineStart
		if i < len(reports) && reports[i].AdjustedLine > 0 {
			line = reports[i].AdjustedLine
		}
		if evidenceRepairCoveredByGroundedSibling(it, file, line, groundedByFile[file]) {
			continue
		}
		b := byFile[file]
		if b == nil {
			b = &bucket{seen: make(map[int]bool)}
			byFile[file] = b
		}
		if !b.seen[line] {
			b.seen[line] = true
			b.order = append(b.order, line)
		}
	}
	if len(byFile) == 0 {
		return nil
	}
	files := make([]string, 0, len(byFile))
	for file := range byFile {
		files = append(files, file)
	}
	sort.Strings(files)
	out := make([]types.ToolRepairTarget, 0, len(files))
	for _, file := range files {
		lines := append([]int(nil), byFile[file].order...)
		sort.Ints(lines)
		out = append(out, types.ToolRepairTarget{
			File:   file,
			Lines:  lines,
			Action: string(types.RepairReadFile),
		})
	}
	return out
}

func evidenceRepairCoveredByGroundedSibling(item types.EvidenceItem, file string, line int, carriers []groundedRepairCarrier) bool {
	if file == "" || line <= 0 || len(carriers) == 0 {
		return false
	}
	tails := types.EvidenceSurfaceSymbolTails(item)
	for _, carrier := range carriers {
		if carrier.line <= 0 || evidenceRepairAbsInt(carrier.line-line) > 2 {
			continue
		}
		if carrier.line == line {
			return true
		}
		if len(tails) == 0 {
			continue
		}
		for _, tail := range tails {
			if carrier.tails[tail] {
				return true
			}
		}
	}
	return false
}

func evidenceRepairAbsInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func renderEmitEvidenceRepairToolHint(targets []types.ToolRepairTarget) string {
	if len(targets) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Re-read the exact source locations below and re-emit grounded evidence before widening scope:\n")
	for _, target := range targets {
		lines := renderToolRepairLineList(target.Lines, 4)
		if lines == "" {
			fmt.Fprintf(&b, "- %s\n", target.File)
			continue
		}
		fmt.Fprintf(&b, "- %s near lines %s\n", target.File, lines)
	}
	b.WriteString("Do not spend read_file budget on non-repairable illustrative/context-only mentions.")
	return b.String()
}

func renderToolRepairLineList(lines []int, max int) string {
	if len(lines) == 0 {
		return ""
	}
	if max <= 0 || max > len(lines) {
		max = len(lines)
	}
	parts := make([]string, 0, max+1)
	for _, line := range lines[:max] {
		parts = append(parts, strconv.Itoa(line))
	}
	if len(lines) > max {
		parts = append(parts, "...")
	}
	return strings.Join(parts, ", ")
}

func failEmit(name string, now time.Time, format string, args ...interface{}) (types.ToolResult, error) {
	msg := fmt.Sprintf(format, args...)
	return types.ToolResult{
		ToolName:  name,
		Success:   false,
		Summary:   msg,
		Timestamp: now,
	}, nil
}

// Session 11 R4 helpers — self-reference trap detection.

// extractPrimaryEntity returns the first element of
// AnalysisIR.RequestModel.AnalyzerHints.Entities as the question's
// primary entity. Returns "" when no IR / no entities / the first
// entity is empty; the caller treats "" as "skip R4 for this
// dispatch" so non-analyzed tool calls stay unaffected.
func extractPrimaryEntity(ctx *types.BusContext) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	if len(ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities) == 0 {
		return ""
	}
	return strings.TrimSpace(ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities[0])
}

func pendingExactResolutionTargets(ctx *types.BusContext, contract *types.ExactResolutionContract) []string {
	if ctx == nil || ctx.Mutable == nil || contract == nil {
		return nil
	}
	unverified := append(ctx.Mutable.EvidenceClosure().UnverifiedFindings(), unverifiedFindingsFromStageReports(ctx.StageReports)...)
	unverified = dedupeUnverifiedFindings(unverified)
	if pending := types.ExactResolutionPendingTargets(contract, unverified); len(pending) > 0 {
		return pending
	}
	if ctx.Mutable.StableInvestigationResultKind() == "absence" &&
		strings.TrimSpace(ctx.Mutable.StableAbsenceJustification()) != "" {
		return nil
	}
	pool := types.ExactResolutionSurfaceEvidencePool(ctx.Mutable.EmittedEvidence(), ctx.EvidenceItems, ctx.AnswerChains)
	if types.ExactResolutionHasDefiningTargetProof(contract, pool) {
		return nil
	}
	return append([]string(nil), contract.Targets...)
}

func stabilizeExactResolutionEvidence(ev *types.EvidenceItem, gc *ground.Context, contract *types.ExactResolutionContract, pendingTargets []string) bool {
	if ev == nil || contract == nil {
		return false
	}
	changed := false
	if ev.ContextRole == types.EvidenceContextRoleIllustrativeOnly && exactResolutionEvidenceMentionsAnyTarget(contract, *ev) {
		note := fmt.Sprintf(
			"illustrative mention of the exact %s is not defining proof. Use absence_justification plus grounded defining anchors; do NOT repair this item.",
			exactResolutionTargetLabel(contract),
		)
		ev.Kind = types.EvidenceUnresolved
		ev.Confidence = 0
		ev.GroundingStatus = types.GroundingUngrounded
		ev.GroundingTier = ""
		ev.GroundingNote = note
		return true
	}
	if len(pendingTargets) > 0 &&
		types.IsNegativeEvidencePredicate(ev.Predicate) &&
		(ev.ContextRole == types.EvidenceContextRoleUnknown || ev.ContextRole == types.EvidenceContextRoleDefining) {
		surface := strings.Join([]string{
			ev.Subject, ev.AnchorSymbol, ev.Object, ev.Source,
		}, "\n")
		sameFamily := types.ExactResolutionSameFamilyMatchScore(contract, surface) > 0 ||
			types.ExactResolutionEvidenceCanSatisfyRelatedContext(contract, *ev, nil)
		targetMention := exactResolutionEvidenceMentionsAnyTarget(contract, *ev) ||
			evidenceGroundedWindowMentionsAnyTarget(*ev, gc, contract)
		if targetMention || sameFamily {
			if targetMention {
				ev.ContextRole = types.EvidenceContextRoleAbsenceSupport
			} else {
				ev.ContextRole = types.EvidenceContextRoleRelatedContext
			}
			note := fmt.Sprintf(
				"this item is a grounded negative probe about the unresolved exact %s and cannot count as defining proof. Treat it as context only; do NOT repair this item.",
				exactResolutionTargetLabel(contract),
			)
			if targetMention {
				note = fmt.Sprintf(
					"this item is a grounded negative probe for the unresolved exact %s and supports the absence conclusion, but it does not define the target. Treat it as absence support only; do NOT repair this item.",
					exactResolutionTargetLabel(contract),
				)
			}
			if appendGroundingNoteOnce(ev, note) {
				changed = true
			}
			changed = true
		}
	}
	if ev.ContextRole != types.EvidenceContextRoleIllustrativeOnly &&
		exactResolutionEvidenceMentionsAnyTarget(contract, *ev) &&
		!exactResolutionEvidenceDirectlyAnchorsAnyTarget(contract, *ev) {
		note := fmt.Sprintf(
			"this item names the requested exact %s only in explanatory context, not as a defining anchor. Treat it as nearby context only; do NOT repair this item.",
			exactResolutionTargetLabel(contract),
		)
		if evidenceGroundedWindowMentionsAnyTarget(*ev, gc, contract) {
			if ev.ContextRole != types.EvidenceContextRoleAbsenceSupport {
				ev.ContextRole = types.EvidenceContextRoleAbsenceSupport
				changed = true
			}
			note = fmt.Sprintf(
				"this item names the requested exact %s in the anchored code/config window but not as a defining anchor. Treat it as absence support only; do NOT repair this item.",
				exactResolutionTargetLabel(contract),
			)
		} else if ev.ContextRole == types.EvidenceContextRoleUnknown || ev.ContextRole == types.EvidenceContextRoleDefining {
			ev.ContextRole = types.EvidenceContextRoleRelatedContext
			changed = true
		}
		if appendGroundingNoteOnce(ev, note) {
			changed = true
		}
	}
	if len(pendingTargets) == 0 {
		return changed
	}
	if ev.ContextRole == types.EvidenceContextRoleIllustrativeOnly || exactResolutionEvidenceMentionsAnyTarget(contract, *ev) {
		return changed
	}
	if ev.ContextRole == types.EvidenceContextRoleUnknown || ev.ContextRole == types.EvidenceContextRoleDefining {
		ev.ContextRole = types.EvidenceContextRoleRelatedContext
		changed = true
	}
	familyScore := types.ExactResolutionSameFamilyMatchScore(contract, strings.Join([]string{
		ev.Subject, ev.AnchorSymbol, ev.Object, ev.Source,
	}, "\n"))
	note := fmt.Sprintf(
		"the primary exact %s %q remains unresolved; this grounded evidence does not define the exact target and must stay context only. Do NOT repair this item.",
		exactResolutionTargetLabel(contract),
		strings.Join(pendingTargets, ", "),
	)
	if contract.RelatedContextPolicy == types.ExactContextSameFamilyGrounded &&
		contract.TargetKind == types.SubjectConfigKey &&
		familyScore == 0 {
		note = fmt.Sprintf(
			"the primary exact %s %q remains unresolved; this grounded evidence is from a different nearby config family and must stay context only, not the main answer or precedence chain. Do NOT repair this item.",
			exactResolutionTargetLabel(contract),
			strings.Join(pendingTargets, ", "),
		)
	} else if types.EvidenceItemStructurallyMentionsAnyTerm(*ev, types.ExactResolutionContextTerms(contract)) || familyScore > 0 {
		note = fmt.Sprintf(
			"the primary exact %s %q remains unresolved; this grounded same-scope evidence is nearby context only and must not be treated as a substitute. Do NOT repair this item.",
			exactResolutionTargetLabel(contract),
			strings.Join(pendingTargets, ", "),
		)
	}
	if appendGroundingNoteOnce(ev, note) {
		changed = true
	}
	return changed
}

func stabilizeIllustrativeEvidence(ev *types.EvidenceItem) bool {
	if ev == nil || ev.ContextRole != types.EvidenceContextRoleIllustrativeOnly {
		return false
	}
	return appendGroundingNoteOnce(ev, "this anchor points at comment/doc/example text, so it is illustrative context only. Do NOT repair this item.")
}

func exactResolutionEvidenceDirectlyAnchorsAnyTarget(contract *types.ExactResolutionContract, ev types.EvidenceItem) bool {
	return types.ExactResolutionDirectAnchorMatchesAnyTarget(contract, ev.Subject, ev.AnchorSymbol, ev.Object)
}

func exactResolutionEvidenceMentionsAnyTarget(contract *types.ExactResolutionContract, ev types.EvidenceItem) bool {
	return types.ExactResolutionTextsMentionAnyTarget(contract,
		ev.Subject, ev.Predicate, ev.Object, ev.AnchorSymbol, ev.Condition, ev.Snippet, ev.Summary)
}

func evidenceGroundedWindowMentionsAnyTarget(ev types.EvidenceItem, gc *ground.Context, contract *types.ExactResolutionContract) bool {
	if gc == nil || contract == nil || ev.Source == "" || ev.LineStart <= 0 {
		return false
	}
	fileLines, ok := gc.LineIndex[ev.Source]
	if !ok || len(fileLines) == 0 {
		return false
	}
	start := ev.LineStart - 3
	if start < 1 {
		start = 1
	}
	end := ev.LineEnd
	if end < ev.LineStart {
		end = ev.LineStart
	}
	end += 3
	var b strings.Builder
	for line := start; line <= end; line++ {
		text, ok := fileLines[line]
		if !ok {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(text)
	}
	return types.ExactResolutionTextsMentionAnyTarget(contract, b.String())
}

func appendGroundingNoteOnce(ev *types.EvidenceItem, note string) bool {
	note = strings.TrimSpace(note)
	if ev == nil || note == "" {
		return false
	}
	if strings.Contains(strings.ToLower(ev.GroundingNote), strings.ToLower(note)) {
		return false
	}
	if strings.TrimSpace(ev.GroundingNote) == "" {
		ev.GroundingNote = note
		return true
	}
	ev.GroundingNote = strings.TrimSpace(ev.GroundingNote) + " " + note
	return true
}

func exactResolutionTargetLabel(contract *types.ExactResolutionContract) string {
	if contract == nil || strings.TrimSpace(contract.TargetLabel) == "" {
		return "target"
	}
	return strings.TrimSpace(contract.TargetLabel)
}

func exactResolutionDiagramRequiredFiles(ctx *types.BusContext, contract *types.ExactResolutionContract) []string {
	if ctx == nil || contract == nil {
		return nil
	}
	if ctx.AnalysisIR == nil ||
		ctx.AnalysisIR.RequestModel.Scenario != types.ScenarioConfigTrace ||
		contract.TargetKind != types.SubjectConfigKey ||
		contract.RelatedContextPolicy != types.ExactContextSameFamilyGrounded {
		return nil
	}
	var files []string
	if ctx.Mutable != nil {
		files = ctx.Mutable.ExactContextRequiredFiles()
	}
	if len(files) == 0 {
		files = ctx.AnalysisIR.EvidencePlan.RequiredFiles
	}
	if len(files) == 0 {
		return nil
	}
	out := make([]string, 0, len(files))
	for _, file := range files {
		canon := ground.CanonicalRepoRelative(file, ctx.RepoRoot)
		if canon == "" {
			continue
		}
		out = append(out, canon)
	}
	return out
}

func emitSourceMatchesAnyRequiredFile(source string, requiredFiles []string) bool {
	source = ground.CanonicalRepoRelative(source, "")
	if source == "" || len(requiredFiles) == 0 {
		return false
	}
	for _, file := range requiredFiles {
		file = ground.CanonicalRepoRelative(file, "")
		if file != "" && file == source {
			return true
		}
	}
	return false
}

// isSelfRefEvidence tests the triple-condition R4 predicate:
//
//  1. ev.Subject equals (case-insensitive) the primary entity
//     (so the evidence talks ABOUT the primary entity)
//  2. ev.Predicate is a "terminal value" predicate (returns,
//     equals, is) — the evidence claims the primary entity
//     resolves to its own name, a self-reference pattern
//  3. ev.Snippet contains the primary entity as a quoted literal
//     ("explorer", 'explorer', or equivalent) — the smoking gun
//     that ties the claim back to the trap shape. Without the
//     quoted literal, evidence like "Explorer.Name() returns from
//     a field" is legitimate and not a trap.
//
// All three conditions must hold; loose matching here would
// reject legitimate self-descriptive evidence (e.g. a struct
// that genuinely bears its own name).
func isSelfRefEvidence(ev *types.EvidenceItem, primaryEntity string) bool {
	if ev == nil || primaryEntity == "" {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(ev.Subject), primaryEntity) {
		return false
	}
	predicate := strings.ToLower(strings.TrimSpace(ev.Predicate))
	if predicate != "returns" && predicate != "equals" && predicate != "is" {
		return false
	}
	lit1 := `"` + primaryEntity + `"`
	lit2 := `'` + primaryEntity + `'`
	return strings.Contains(ev.Snippet, lit1) || strings.Contains(ev.Snippet, lit2)
}
