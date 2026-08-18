package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/authority"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/stageauthority"
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
var emitEvidenceCommitHashTokenRe = regexp.MustCompile(`(?i)\b[0-9a-f]{7,40}\b`)

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

func emitEvidenceSalienceNames() []string {
	return types.EvidenceSalienceStrings()
}

type emitEvidenceParams struct {
	Items []emitEvidenceItem `json:"items"`
}

type emitEvidenceItem struct {
	// Scope is REQUIRED. Each scope routes through a different
	// grounder; the per-scope required-field bundles below are
	// validated by buildEmitEvidenceItem against types.ValidateScope.
	Scope string `json:"scope"`

	EvidenceKind string `json:"evidence_kind,omitempty"`
	LegacyKind   string `json:"kind,omitempty"`
	Subject      string `json:"subject,omitempty"`
	Predicate    string `json:"predicate,omitempty"`
	Object       string `json:"object,omitempty"`
	Source       string `json:"source,omitempty"`
	// LineStart / LineEnd use FlexInt so LLMs that emit numeric
	// strings ("42") or floats (42.0) pass strict schema validation
	// instead of failing the whole batch on format pedantry.
	LineStart    FlexInt  `json:"line_start,omitempty"`
	LineEnd      FlexInt  `json:"line_end,omitempty"`
	Condition    string   `json:"condition,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	AnchorKind   string   `json:"anchor_kind,omitempty"`
	AnchorSymbol string   `json:"anchor_symbol,omitempty"`
	Snippet      string   `json:"snippet,omitempty"`
	ContextRole  string   `json:"context_role_hint,omitempty"`
	DiagramRole  string   `json:"diagram_role_hint,omitempty"`
	SurfaceTerms []string `json:"surface_terms,omitempty"`
	// LoadBearingSummary opts the summary into authoritative surface
	// rendering for downstream stages. Default false: typed fields are
	// the canonical surface and free-form summary text gets stripped
	// before the next stage sees it. Set true ONLY when the summary
	// carries a scalar (hash, version string, count, single concrete
	// identifier from a tool call) that the answer cannot reproduce
	// from the typed fields (subject / predicate / object /
	// anchor_symbol / snippet) alone. Schema validation rejects the
	// flag when summary is empty (an empty summary cannot be load-
	// bearing).
	LoadBearingSummary bool   `json:"load_bearing_summary,omitempty"`
	Salience           string `json:"salience,omitempty"`

	// Scope-specific bundles. Each bundle is read only when the Scope
	// field selects it; ValidateScope enforces that the required
	// bundle fields are populated.
	SectionPath        string               `json:"section_path,omitempty"`
	FileRoleLabel      string               `json:"file_role_label,omitempty"`
	CrossfileQuery     *emitCrossfileQuery  `json:"crossfile_query,omitempty"`
	CrossfileAssertion *emitCrossfileAssert `json:"crossfile_assertion,omitempty"`
	NegativeQuery      *emitNegativeQuery   `json:"negative_query,omitempty"`
	NegativeScope      string               `json:"negative_scope,omitempty"`
}

type emitCrossfileQuery struct {
	Files   []string `json:"files,omitempty"`
	Pattern string   `json:"pattern,omitempty"`
	Context string   `json:"context,omitempty"`
}

type emitCrossfileAssert struct {
	Kind  string `json:"kind,omitempty"`
	Count int    `json:"count,omitempty"`
}

type emitNegativeQuery struct {
	File    string `json:"file,omitempty"`
	Pattern string `json:"pattern,omitempty"`
	Section string `json:"section,omitempty"`
}

// EmitEvidenceProducer is the Producer string stamped on every item
// the tool ingests. Exported so tests and downstream consumers
// (filtering-pipeline doc, future grounder integration) can identify
// the channel without grepping for a literal.
const EmitEvidenceProducer = types.EvidenceProducerExplorerEmitEvidence

// EmitEvidenceSurfaceTermReviewCode marks a successful emit_evidence
// result whose accepted facts are grounded, but whose nearby
// already-read source/header labels look like user-visible aliases the
// model did not yet author into surface_terms. The repair is advisory:
// it asks the model to re-emit structured data when those labels are
// load-bearing; it never auto-fills answer text.
const EmitEvidenceSurfaceTermReviewCode = "evidence_surface_terms_review"

// EmitEvidenceDuplicateNoopCode marks a successful emit_evidence call
// that was schema-valid but recorded no new or enriching evidence
// because every item was already present in the mutable evidence
// buffer. Downstream loop controllers treat this typed repair code as
// "no progress" so duplicate emits do not keep exploration alive.
const EmitEvidenceDuplicateNoopCode = "evidence_duplicate_noop"

func (t *EmitEvidence) Name() string { return "emit_evidence" }

func (t *EmitEvidence) Description() string {
	return "Emit one or more structured evidence items as the result of reading a source file. " +
		"JSON carrier boundary: `items` is one native JSON array, never a quoted/escaped JSON string. Each items[i] uses only this tool's item fields; `support_refs` belongs to aggregate_facts on emit_investigation_complete and is not an emit_evidence item field. " +
		"Call this AFTER you have read a file during investigation, with one item per " +
		"fact you want the synthesis layer to see. The batched 'items' array preserves the " +
		"existing 'one tool call per file' write pattern; do not call this tool once per item. " +
		"Do not use this tool for VCS metadata from git_log / git_show / git_diff / exec_command git output; carry those findings in emit_investigation_complete.reason and aggregate_facts unless you also read a real current-repo file line.\n\n" +
		"Each item MUST set: source (repo-relative path), line_start (exact gutter line number, " +
		"never estimated), evidence_kind (one of: " + strings.Join(emitEvidenceAllowedKindNames(), ", ") + "), " +
		"anchor_kind (one of: " + strings.Join(emitAnchorKindNames(), ", ") + "), and anchor_symbol " +
		"(the identifier the grounder should find on that line).\n\n" +
		"There are TWO different kind fields with different jobs:\n" +
		"  - evidence_kind = the SEMANTIC fact shape (direct / conditional / registration / mechanism / relationship)\n" +
		"  - anchor_kind   = the source surface at source:line_start (use the schema enum above; it is the sole vocabulary authority)\n" +
		"Never put `direct` / `conditional` / `registration` / `mechanism` / `relationship` into anchor_kind. " +
		"Never put an anchor_kind enum value into evidence_kind.\n\n" +
		"anchor_kind tells the grounder what KIND of code location you are pointing at:\n" +
		"  - definition: the line is a function/type/const/var declaration\n" +
		"  - call:       the line contains a function/method call (anchor_symbol = callee name)\n" +
		"  - callback:   the line passes a non-invoked callable value to a receiving API; subject = receiving API, object/anchor_symbol = passed callable. This proves handoff, not execution\n" +
		"  - argument:   the line passes one complete non-callable value expression to a receiving API; subject/anchor_symbol = byte-exact complete argument, object = receiving API. This proves only argument-to-receiver handoff\n" +
		"  - condition:  the line starts an if / when / unless / switch / case / guard\n" +
		"  - return:     the line is a return or yield\n" +
		"  - assignment: the line assigns (:= or =)\n" +
		"  - initializer: the line initializes a field/property/member inside a struct/object/named-argument/designated/config literal\n" +
		"  - import:     the line is an import / use / require (anchor_symbol = package path/alias)\n\n" +
		"  - string_literal: the line contains a source-code string/char/template literal whose value is the evidence (tool name, route path, config key, enum string value, protocol name, log marker). It does NOT prove a definition/call/assignment.\n\n" +
		"  - text_reference: the line's visible source/config/doc/comment text is itself the evidence; use this for documentation references, examples, generated headers, config prose, or comment-only mentions. It does NOT prove a definition/call/assignment.\n\n" +
		"anchor_symbol is the concrete identifier the grounder should see at line_start. For a " +
		"method call 'x.Execute()' at line 42 the anchor_symbol is 'Execute' and anchor_kind is 'call'. " +
		"For a struct type declaration 'type Orchestrator struct' the anchor_symbol is 'Orchestrator' " +
		"and anchor_kind is 'definition'. On that same line the evidence_kind may still be 'direct' " +
		"because the semantic claim is simply a direct fact about a definition. Likewise a config assignment " +
		"line can be `evidence_kind=\"direct\"` with `anchor_kind=\"assignment\"`. A line such as " +
		"`CitationReq: types.CitationReq{Required: false}` or `.required = false` is an initializer, " +
		"not a symbol definition. For condition / return / assignment / initializer anchors, anchor_symbol " +
		"should still be a visible token on the cited line (for `for attempt := 0; attempt < max; {`, use " +
		"`attempt` or `max`, not `for loop`); if the line has no durable symbol, include the exact `snippet` " +
		"and structured `condition` / value fields so the tool can ground the line itself.\n\n" +
		"For call-like evidence (`predicate` = calls / invokes / dispatches / delegates to) with " +
		"`anchor_kind=\"call\"`, the semantic direction is ALWAYS caller -> callee: `subject` must be " +
		"the containing function/method and `object` must be the callee on that line. Example: if " +
		"`outer()` contains `return inner(...)`, emit `subject=\"outer\"`, `predicate=\"calls\"`, " +
		"`object=\"inner\"`.\n\n" +
		"For callback handoff evidence, use `evidence_kind=\"relationship\"`, `anchor_kind=\"callback\"`, " +
		"`subject` = the receiving call/API expression, and `object` + `anchor_symbol` = the callable value as written on that same line. Example: `executor.submit(worker)` becomes subject=`executor.submit`, object=`worker`; do not relabel it as a direct call to `worker`.\n\n" +
		"For ordinary argument handoff evidence, use `evidence_kind=\"relationship\"`, `anchor_kind=\"argument\"`, `subject` + `anchor_symbol` = the byte-exact complete non-callable argument expression, and `object` = the receiving call/API expression. Example: `BuildAgentContext(o.busCtx, stage)` becomes subject=`o.busCtx`, object=`BuildAgentContext`; this does not prove how the receiver stores or uses that value.\n\n" +
		"For registration/binding evidence, cite the expression that performs the binding, not its surrounding module or factory definition. Example: `registry.add(wrapper(target))` becomes evidence_kind=`registration`, anchor_kind=`call`, anchor_symbol=`add`, subject=`registry`, object=`wrapper(target)`. When you also select the surrounding module/factory/container definition as mechanism or direct evidence, keep that definition row and emit this actual binding expression as a separate registration row in the SAME batch; the definition row never substitutes for the binding row. This proves the binding only; emit any downstream invocation as its own relationship/call row.\n\n" +
		"Evidence entailment is bounded by the typed anchor and source span. A call-site item proves only " +
		"that the caller invokes the callee; it does NOT prove the callee's internal guards, return value, " +
		"side effects, or pipeline/stage ordering. A definition item proves the declaration/signature, not " +
		"arbitrary behavior inside the body; condition, return, assignment, and initializer items prove only " +
		"the construct at their grounded line/range. For a structured object/record entry, the entry identity " +
		"line proves only that identity: anchor a requested field value at its exact initializer line, or use " +
		"one bounded line_range when a single answer row genuinely combines fields across adjacent lines. " +
		"If a summary needs behavior from another function or " +
		"a line outside this item's grounded span, read that source and emit a separate evidence item there. " +
		"Never combine sibling functions' behavior into one line-anchored evidence summary.\n\n" +
		"Optional hint fields: `context_role_hint` may be `defining`, `absence_support`, `related_context`, or `illustrative_only` " +
		"to recommend how the item should be used for exact-target answers. `diagram_role_hint` may be `default`, " +
		"`config`, `runtime`, or `override` for config-precedence traces (`config` = grounded repo/user config-file layer such as YAML/JSON/TOML/INI/etc.). These are recommendations only: the tool " +
		"validates them structurally and may downgrade or ignore inconsistent hints.\n\n" +
		"surface_terms is optional model-authored structured data for exact user-visible labels / aliases copied verbatim from already-read source, log, or trace lines (for example route names, package/module labels, config keys, macro names, trace span names, original file labels, and labels in leading documentation/header comments attached to the cited anchor). Ungrounded optional terms are dropped with a summary note while the evidence item is kept; answer synthesis treats accepted terms as preservation guidance when they are relevant to the visible answer.\n\n" +
		"salience is optional structured data for answer participation: load_bearing means the answer cannot honor a visible claim without this row; exhaust_listed means this row is one member of a complete list the user asked for; supporting means an intermediate fact the answer chain uses; context means background the answer does not lean on. Omit it when unsure. This field helps preserve important rows in long investigations but does not replace member_set, answer_symbol, citations, or final answer obligations.\n\n" +
		"For list/enumeration members such as exported constants, enum values, public functions, fields, routes, or config keys, `summary` should explain the member's role using already-read code (signature, right-hand value, registry mapping, caller/callee relation, or visible comment). Do not use summary only to say that the item is the Nth member of a category; ordinal/count information belongs in aggregate_facts, while evidence summary should carry meaning.\n\n" +
		"snippet is optional but recommended for conditional / mechanism / registration items: paste " +
		"1-2 lines of the actual code so the snippet_fuzzy recovery tier can re-anchor if your " +
		"line_start is off by one.\n\n" +
		"If a previously accepted evidence item has incorrect metadata (for example anchor_kind, " +
		"anchor_symbol, owner, snippet, salience, or surface_terms), re-emit the SAME source/line/fact " +
		"with corrected non-empty fields. The system merges same StableEvidenceID rows as an amendment " +
		"instead of duplicating answer-grade evidence. Exact same rows remain a no-progress duplicate no-op.\n\n" +
		"The emit_evidence tool grounds every item synchronously and returns per-item feedback " +
		"(grounded / recovered / ungrounded) in the same turn, so you can correct line numbers or " +
		"anchor_symbols on the next call without waiting for a later stage. Unknown evidence_kind / anchor_kind values and " +
		"unknown fields are REJECTED — the tool will not silently coerce."
}

var (
	emitEvidenceParametersOnce   sync.Once
	emitEvidenceParametersCached json.RawMessage
)

func emitEvidenceParametersSchema() json.RawMessage {
	emitEvidenceParametersOnce.Do(func() {
		emitEvidenceParametersCached = buildEmitEvidenceParametersSchema()
	})
	return cloneJSONRawMessage(emitEvidenceParametersCached)
}

func buildEmitEvidenceParametersSchema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"items": map[string]any{
				"type":        "array",
				"description": "Batch of evidence items extracted from one or more files. This must be one native JSON array, never a quoted/escaped JSON string. Send the full batch in one call — do not invoke the tool per item. Every item uses only the item properties declared below; support_refs is not an item property and belongs to aggregate_facts on emit_investigation_complete.",
				"items": map[string]any{
					"type": "object",
					// Relation-shaped facts are unusable when either endpoint is
					// hidden only in summary prose. Mirror the runtime contract in
					// the provider-visible schema so the model receives one JSON
					// lesson instead of succeeding at decode and losing the row at
					// capsule/diagram construction time. The runtime remains
					// compatible with older sparse persisted rows.
					"allOf": []any{
						map[string]any{
							"if": map[string]any{
								"properties": map[string]any{
									"evidence_kind": map[string]any{"enum": []string{"relationship", "registration"}},
								},
								"required": []string{"evidence_kind"},
							},
							"then": map[string]any{"required": []string{"subject", "object"}},
						},
						map[string]any{
							"if": map[string]any{
								"properties": map[string]any{
									"evidence_kind": map[string]any{"const": "conditional"},
								},
								"required": []string{"evidence_kind"},
							},
							"then": map[string]any{
								"required": []string{"condition", "anchor_kind"},
								"properties": map[string]any{
									"anchor_kind": map[string]any{"const": "condition"},
								},
							},
						},
					},
					"properties": map[string]any{
						"scope": map[string]any{
							"type":        "string",
							"enum":        emitEvidenceScopeNames(),
							"description": "REQUIRED. Anchor shape — the system routes the item through a scope-specific grounder. Pick the scope that matches what your evidence proves. NOTE: 'line' is NOT a default; pick the most specific scope. If the fact is layer-shaped / contract-shaped / absence-shaped (NOT a per-line code claim), prefer 'file' / 'crossfile' / 'negative' — using 'line' to anchor a sibling line as a proxy for layer identity weakens the answer surface (the line-only renderer hides the layer/contract/absence semantics in prose). Definitions: 'line' = single (file, line) — direct/conditional/mechanism/relationship/registration over a specific code location; 'line_range' = multi-line block (struct definition, function body, comment block); 'section' = named YAML/Go/JSON/TOML schema section (use section_path); 'file' = file's identity AS a layer (use file_role_label) — the file IS the config layer / CLI registration layer / etc., independent of any specific line in it; 'crossfile' = cross-file contract verified by query (use crossfile_query + crossfile_assertion) — the system re-runs your query and rejects the item if the assertion fails; 'negative' = confirmed absence (use negative_query + negative_scope, requires evidence_kind='absent'). The system rejects emit if the per-scope required fields are missing.",
						},
						"evidence_kind": map[string]any{
							"type":        "string",
							"enum":        emitEvidenceAllowedKindNames(),
							"description": "REQUIRED. Semantic fact shape, NOT the syntax at line_start. direct = literal fact at file:line. conditional = behaviour gated by an IF clause. registration = something registered/bound with EXACT values. If a selected module/factory/container definition contains the binding operation, keep the definition as its own direct/mechanism row AND emit the actual binding expression as a separate registration row in the same batch; a definition row never substitutes for that binding row. mechanism = how a process works step by step. relationship = link between two symbols (use subject + object). absent = confirmed absence (REQUIRES scope='negative'). Values like definition/call/assignment belong in anchor_kind, not here.",
						},
						"subject": map[string]any{
							"type":        "string",
							"description": "Primary semantic symbol the item is about (function name, type, key). For anchor_kind='call', subject MUST be the caller / containing function at that line. For anchor_kind='callback', subject MUST be the receiving call/API expression on that line. For anchor_kind='argument', subject MUST be the byte-exact complete non-callable argument expression. For anchor_kind='condition', subject should be the exact enclosing callable when known; grounding derives owner identity independently, so do not invent or attempt to emit an owner_symbol field. For anchor_kind='precedence', subject MUST be the earlier endpoint in the cited bounded source range. For evidence_kind='registration', subject is the exact registry slot/key/binding source and object is the exact bound target; populate both fields.",
						},
						"predicate": map[string]any{
							"type":        "string",
							"description": "Lowercase verb tying subject to object. PREFER these canonical verbs so the deterministic relation-diagram renderer picks the edge up — anything outside this list is rendered as unstructured prose: calls, invokes, dispatches, delegates to, passes callback, passes argument, binds, binds ONLY, registers, wires, provides, returns, yields, constructs, instantiates, defines, implements, extends, embeds, maps, config, decorates. Optional; defaults to the lower-cased evidence_kind.",
						},
						"object": map[string]any{
							"type":        "string",
							"description": "Secondary symbol or value. Required for relationship and registration. For anchor_kind='call', object MUST be the callee symbol on that line. For anchor_kind='callback', object MUST be the non-invoked callable value passed to subject on that line. For anchor_kind='argument', object MUST be the receiving call/API expression. For anchor_kind='condition', object is optional and may only repeat the exact condition identity/expression when an explicit guard endpoint is needed; it must not name a selected branch-body operation that the guard line does not itself contain. For anchor_kind='precedence', object MUST be the later endpoint in the same cited bounded source range. For evidence_kind='registration', object is the exact class/function/handler/value bound by subject; do not leave either endpoint only in summary prose.",
						},
						"source": map[string]any{
							"type":        "string",
							"description": "Repository-relative file path the fact comes from. Required.",
						},
						"line_start": map[string]any{
							"type":        "integer",
							"description": "Exact gutter line number from read_file — NEVER estimated. The grounder uses this to verify the claim; wrong numbers are flagged as ungrounded or auto-recovered. scope='file' requires line_start omitted or 0 (enforced — a file-identity anchor has no specific line).",
						},
						"line_end": map[string]any{
							"type":        "integer",
							"description": "Last line of the cited range. Defaults to line_start when omitted.",
						},
						"condition": map[string]any{
							"type":        "string",
							"description": "For conditional items: REQUIRED exact IF clause from the guard line. A guard is a unary decision fact: use the exact enclosing callable in subject when known and a visible guard token in anchor_symbol; owner identity is populated after grounding and is not a model input. A guarded call is two evidence rows: one conditional/condition row on the guard and one relationship/call row on the invocation. Do not put a selected tool/handler or body call in object unless the guard expression itself contains that identity.",
						},
						"summary": map[string]any{
							"type":        "string",
							"description": "Free-text rationale describing the fact. Keep concise; do not paraphrase numbers or string literals. For exhaustive list members, explain the member's role from already-read code (signature, RHS value, registry mapping, caller/callee relation, or visible comment); do not use summary only for ordinal/category wording such as 'Nth item'.",
						},
						"context_role_hint": map[string]any{
							"type":        "string",
							"enum":        emitEvidenceContextRoleNames(),
							"description": "OPTIONAL recommendation for exact-target questions. defining = direct defining proof, related_context = grounded nearby context but not the exact target itself, illustrative_only = documentation/prompt-support prose or a comment-only anchored line that should NOT be treated as defining proof. Test, fixture, example, third-party, vendor, and generated source paths are not automatically illustrative; the tool validates the anchored line and may downgrade the hint.",
						},
						"diagram_role_hint": map[string]any{
							"type":        "string",
							"enum":        emitEvidenceDiagramRoleNames(),
							"description": "OPTIONAL recommendation for config-precedence traces. default = code defaults, config = repo/user config-file layer (YAML/JSON/TOML/INI/etc.), runtime = code/runtime binding layer, override = CLI/high-precedence override layer. The tool validates and may ignore inconsistent hints.",
						},
						"load_bearing_summary": map[string]any{
							"type":        "boolean",
							"description": "OPTIONAL. Default false. Set true ONLY when the `summary` text holds a scalar (commit hash, version string, count, single concrete identifier, value derived from a tool / shell / git command output) that the user-facing answer must reproduce verbatim AND the typed fields (subject / predicate / object / anchor_symbol / snippet) cannot themselves carry that scalar. False is correct for the common case where summary is a paraphrase / rationale that the typed fields already encode. The tool rejects this flag when summary is empty.",
						},
						"salience": map[string]any{
							"type":        "string",
							"enum":        emitEvidenceSalienceNames(),
							"description": "OPTIONAL. Names how this fact participates in the user-facing answer. load_bearing = the answer cannot honor a visible claim without this row; exhaust_listed = this row is one member of a complete list the user asked for; supporting = an intermediate fact the answer chain uses; context = background the answer does not lean on. Omit when unsure; unset preserves ordinary behavior. This field helps preserve important rows in long investigations but does not replace member_set, answer_symbol, citations, or final answer obligations.",
						},
						"surface_terms": map[string]any{
							"type":        "array",
							"items":       map[string]any{"type": "string"},
							"description": "OPTIONAL. Model-authored exact strings from the already-read source/log/trace lines that the final answer should preserve as visible aliases or labels, but that are not already captured by subject/object/anchor_symbol. Use for original file labels, route names, package/module names, config keys, macro names, trace span names, runtime object labels, or labels in leading documentation/header comments attached to the cited anchor. Every accepted term must appear verbatim in the cited source window; ungrounded optional terms are dropped with a summary note.",
						},
						"anchor_kind": map[string]any{
							"type":        "string",
							"enum":        emitAnchorKindNames(),
							"description": "REQUIRED. Source surface at line_start, NOT the semantic evidence shape. The sibling enum is the sole allowed-value authority. call means direct invocation; callback means a non-invoked callable value passed to a receiving API; argument means one byte-exact complete non-callable argument passed to a receiving API. Both handoff forms prove only their exact source transfer. string_literal/text_reference must not be treated as definition/call/assignment proof. Values like direct/conditional/registration belong in evidence_kind, not here. The grounder dispatches on this so wrong anchor kinds produce confusing ungrounded verdicts.",
						},
						"anchor_symbol": map[string]any{
							"type":        "string",
							"description": "REQUIRED. The identifier or literal value the grounder should find on line_start. For a call like 'x.Execute()' the anchor_symbol is 'Execute'. For callback handoff it is the passed callable expression as written, not the receiving API. For argument handoff it is the byte-exact complete argument expression, identical to subject. For a type decl 'type Orchestrator struct' the anchor_symbol is 'Orchestrator'. For an import the anchor_symbol is the package path or local alias. For string_literal it is the literal value without forcing symbol-definition grounding.",
						},
						"snippet": map[string]any{
							"type":        "string",
							"description": "Optional. 1-2 lines of actual code from the cited location. Enables snippet_fuzzy recovery when line_start is off by ±15 lines — recommended for conditional / mechanism / registration items.",
						},
						"section_path": map[string]any{
							"type":        "string",
							"description": "REQUIRED when scope='section'. Dot-separated path inside the parsed file (e.g. 'explore_*' for a YAML group, 'agents.env_recommender' for a nested config key, 'ExploreHeuristics' for a Go const block). The grounder parses the source and verifies the section exists.",
						},
						"file_role_label": map[string]any{
							"type":        "string",
							"enum":        emitEvidenceFileRoleLabelNames(),
							"description": "REQUIRED when scope='file'. Names the canonical role this file plays as a layer. config_canonical = canonical user-facing config file (e.g. *.yaml / *.toml at repo root); cli_registration = file registering CLI flags / command-line entry points; default_struct = file holding the default-value struct; manifest = package/project manifest (go.mod, package.json, ...); schema = schema-defining file (proto, openapi, ...).",
						},
						"crossfile_query": map[string]any{
							"type":        "object",
							"description": "REQUIRED when scope='crossfile'. Structured query the LLM is asserting about. files accepts 1-5 entries — a longer list REJECTS the whole item rather than trimming it, so pick the ≤5 strongest files or split into multiple items; pattern is a Go regex.",
							"properties": map[string]any{
								"files":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 5},
								"pattern": map[string]any{"type": "string"},
								"context": map[string]any{"type": "string", "description": "Optional: limit search to a section / function / type body."},
							},
						},
						"crossfile_assertion": map[string]any{
							"type":        "object",
							"description": "REQUIRED when scope='crossfile'. The predicate the LLM commits to about the crossfile_query result. The grounder re-runs the query and rejects the item if the assertion does not hold. exists = at least 1 match; forbidden = 0 matches; count_eq = match count exactly equals 'count'.",
							"properties": map[string]any{
								"kind":  map[string]any{"type": "string", "enum": []string{"exists", "forbidden", "count_eq"}},
								"count": map[string]any{"type": "integer"},
							},
						},
						"negative_query": map[string]any{
							"type":        "object",
							"description": "REQUIRED when scope='negative'. The query whose ABSENCE of matches is the claim. Pair with negative_scope to control where the query searches. pattern is a Go regex that the system RE-RUNS; the item is accepted only when it has zero matches in the declared scope. Escape regex metacharacters when you mean a literal string — an unescaped literal can silently match more (or fail to compile) and the absence claim is then rejected.",
							"properties": map[string]any{
								"file":    map[string]any{"type": "string"},
								"pattern": map[string]any{"type": "string"},
								"section": map[string]any{"type": "string", "description": "Required when negative_scope='section'."},
							},
						},
						"negative_scope": map[string]any{
							"type":        "string",
							"enum":        emitEvidenceNegativeScopeNames(),
							"description": "REQUIRED when scope='negative'. Qualifies WHERE the absence holds: file = whole file; range = within a line range (verified precisely against those lines); section = within a named schema section; struct_fields = against a struct's field set. NOTE: section and struct_fields absence is currently verified against the ENTIRE file's text — if the pattern legitimately appears elsewhere in the same file, use negative_scope='range' with line bounds, or restate the query so zero matches holds file-wide.",
						},
					},
					"required": []string{"scope", "evidence_kind"},
				},
			},
		},
		"required": []string{"items"},
	}
	buf, _ := json.Marshal(schema)
	return json.RawMessage(buf)
}

// emitEvidenceScopeNames returns the canonical scope-name list used in
// the JSON schema. Sourced from types.AllEvidenceScopes so the schema
// and the type cannot drift apart.
func emitEvidenceScopeNames() []string {
	scopes := types.AllEvidenceScopes()
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		out = append(out, string(s))
	}
	return out
}

// emitEvidenceFileRoleLabelNames returns the canonical FileRoleLabel
// names. Sourced from types.AllFileRoleLabels.
func emitEvidenceFileRoleLabelNames() []string {
	labels := types.AllFileRoleLabels()
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		out = append(out, string(l))
	}
	return out
}

// emitEvidenceNegativeScopeNames returns the canonical NegativeScope
// names. Sourced from types.AllNegativeScopes.
func emitEvidenceNegativeScopeNames() []string {
	scopes := types.AllNegativeScopes()
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		out = append(out, string(s))
	}
	return out
}

func (t *EmitEvidence) Parameters() json.RawMessage {
	// One schema producer only.  The former handwritten fallback still taught
	// the retired `kind` field and contradicted the typed negative-scope lane.
	// json.Marshal over this closed map cannot fail; keeping a second schema for
	// an impossible failure path merely creates prompt/runtime drift.
	return emitEvidenceParametersSchema()
}

func (t *EmitEvidence) Execute(ctx *types.BusContext, params json.RawMessage) (result types.ToolResult, err error) {
	now := time.Now()
	runtimeTimings := make([]types.ToolRuntimeTiming, 0, 5)
	defer func() {
		attachToolRuntimeTimings(&result, runtimeTimings)
	}()
	if ctx == nil || ctx.Mutable == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_evidence requires a writable context; the caller did not provide one (sub-agents are not supported)",
			Timestamp: now,
		}, nil
	}

	// Strict decode: unknown fields anywhere in the tree (top-level
	// or inside an item) are rejected so a hallucinated key (e.g.
	// 'evidence' instead of 'items', or 'note' instead of 'summary')
	// fails loudly at parse time rather than silently producing a
	// well-formed-looking item the parser quietly drops fields from.
	decodeStart := time.Now()
	params = applyStructuredPayloadCompatWithLegacyStringFieldRepair(t.Name(), params, t.Parameters())
	if repaired, paths, ok := repairEmitEvidenceKnownCompatFields(params); ok {
		logging.Warning("[emit_evidence] local-model compatibility fields normalized before strict decode: %s", strings.Join(paths, ", "))
		params = repaired
	}
	dec := json.NewDecoder(bytes.NewReader(params))
	dec.DisallowUnknownFields()
	var p emitEvidenceParams
	if err := dec.Decode(&p); err != nil {
		recordToolRuntimeTiming(&runtimeTimings, "schema_compat_decode", decodeStart, 0)
		return failStrictDecode(t.Name(), now, err, emitEvidenceMisplacedHints, params)
	}
	recordToolRuntimeTiming(&runtimeTimings, "schema_compat_decode", decodeStart, len(p.Items))
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
	builtParamIndexes := make([]int, 0, len(p.Items))
	rejectedItems := make([]string, 0)
	validationRepairFields := make([]string, 0)
	validationRepairBlocksCompletion := false
	softSkippedItems := make([]string, 0)
	externalObservationSkippedItems := make([]string, 0)
	absenceCompletionRejectedItems := make([]string, 0)
	autoSwapped := make([]int, 0)
	compatRepairs := make([]string, 0)
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
	if oracleSource, ok := ctx.MultiGraph.(interface {
		Oracle() types.SymbolOracle
	}); ok && oracleSource != nil {
		ground.SetCrossRepoOracle(oracleSource.Oracle())
	} else {
		ground.SetCrossRepoOracle(nil)
	}
	groundContextStart := time.Now()
	gc := ground.BuildContext(ctx)
	attachEmitEvidenceSourceGraphResolver(ctx, gc)
	recordToolRuntimeTiming(&runtimeTimings, emitEvidenceGroundContextTimingPhase(gc), groundContextStart, len(p.Items))
	for i, in := range p.Items {
		if reason, ok := emitEvidenceHistoryMetadataSoftSkipReason(ctx, in, i); ok {
			softSkippedItems = append(softSkippedItems, reason)
			continue
		}
		if reason, ok := emitEvidenceExternalObservationSoftSkipReason(ctx, in, i); ok {
			externalObservationSkippedItems = append(externalObservationSkippedItems, reason)
			continue
		}
		ev, perr := buildEmitEvidenceItemWithSwap(&in, i, workDir, gc, &autoSwapped, &compatRepairs)
		if perr != nil {
			if reason, ok := emitEvidenceToolValueSoftSkipReason(ctx, in, i, perr); ok {
				softSkippedItems = append(softSkippedItems, reason)
				continue
			}
			rejection := fmt.Sprintf("items[%d]: %v", i, perr)
			rejectedItems = append(rejectedItems, rejection)
			validationRepairFields = append(validationRepairFields,
				emitEvidenceValidationRepairFields(in, i, perr)...)
			validationRepairBlocksCompletion = validationRepairBlocksCompletion ||
				emitEvidenceValidationFailureBlocksCompletion(ctx, in)
			if emitEvidenceAbsenceCompletionRepairApplies(in) {
				absenceCompletionRejectedItems = append(absenceCompletionRejectedItems, rejection)
			}
			continue
		}
		if primaryEntity != "" && isSelfRefEvidence(&ev, primaryEntity) {
			ev.Confidence = 0
			if ctx.Mutable != nil {
				ctx.Mutable.EvidenceClosure().AppendViolation(types.Violation{
					Kind:       types.ViolSelfRefLiteral,
					Detail:     fmt.Sprintf("items[%d]: subject=%s matches primary entity; anchor literal is self-reference", i, ev.Subject),
					ClusterKey: types.SymbolClusterKey(primaryEntity, "answer_subject.kind"),
					Stage:      string(types.StageExplore),
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
		builtParamIndexes = append(builtParamIndexes, i)
	}
	// Commit 61 Batch F.1 (audit CRITICAL #1, red line "no system
	// hard-cap"): pre-fix, ≥50% rejected items triggered a hard
	// rejection of the WHOLE batch — even the valid items were
	// discarded, forcing the LLM to re-emit everything. The 50%
	// threshold was a heuristic anti-gaming guard with no objective
	// truth; per the no-hard-cap principle, surface per-item reject
	// reasons + keep all valid items.
	//
	// The remaining gate is empty-survival: if EVERY item failed
	// (zero built), the batch can't proceed because there's nothing
	// to record. Caller's tool-result Summary already lists per-item
	// reasons via renderEmitSummary so the LLM sees what failed and
	// can re-emit just the failed ones (mirror of what emit_log_
	// triage / emit_answer_document do).
	if len(built) == 0 && len(softSkippedItems) > 0 && len(rejectedItems) == 0 {
		return types.ToolResult{
			ToolName: t.Name(),
			Success:  true,
			Summary:  renderEmitEvidenceCommandScalarSoftSkipSummary(softSkippedItems),
			Repair: attachToolJSONSurfaceMetadata(t.Name(), &types.ToolRepair{
				Code: "evidence_command_value_to_closure",
				Hint: "Command/VCS-derived outputs are not source-line evidence. Carry verified scalar/count/list answers through emit_investigation_complete.aggregate_facts when they are the requested principal shape; for non-scalar history summaries or diagnostics, carry the VCS finding in emit_investigation_complete.reason and mark any commit/count metadata aggregate_facts as supporting_coverage.",
				Fields: []string{
					"emit_investigation_complete.aggregate_facts",
				},
				Metadata: map[string]string{
					"repair_status": types.ToolRepairStatusAdvisory,
				},
			}),
			Timestamp: now,
		}, nil
	}
	if len(built) == 0 && len(externalObservationSkippedItems) > 0 && len(softSkippedItems) == 0 && len(rejectedItems) == 0 {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   true,
			Summary:   renderEmitEvidenceExternalObservationSoftSkipSummary(ctx, externalObservationSkippedItems),
			Repair:    attachToolJSONSurfaceMetadata(t.Name(), emitEvidenceExternalObservationRepair()),
			Timestamp: now,
		}, nil
	}
	if len(built) == 0 {
		validationRepair := buildEmitEvidenceItemValidationRepair(
			rejectedItems, validationRepairFields, validationRepairBlocksCompletion)
		if len(rejectedItems) > 0 && len(absenceCompletionRejectedItems) == len(rejectedItems) {
			return failEmitWithRepair(t.Name(), now, emitEvidenceAbsenceCompletionRepair(),
				"no valid items after per-item validation:\n%s",
				strings.Join(rejectedItems, "\n"))
		}
		return failEmitWithRepair(t.Name(), now, validationRepair,
			"no valid items after per-item validation:\n%s", strings.Join(rejectedItems, "\n"))
	}

	// Synchronous grounding. Each item is validated against the
	// read_file gutter index (Tier 1) and the repomap graph (Tier 2);
	// recovery tiers (shipped in Step 11) will additionally rewrite
	// near-miss LineStart/Source. The Report drives the per-item
	// feedback in the tool Summary so the LLM sees grounded /
	// recovered / ungrounded verdicts in the same turn it emitted them.
	// P4-cross-sub-repo (Sc 6): wire ground's package-level
	// CrossRepoOracle from BusContext.MultiGraph (type-asserted via
	// inline interface so internal/tool/ground stays free of the
	// repomap/multigraph import cycle that tool/ground →
	// repomap/multigraph → repomap/topology → tool would form).
	diagramRequiredFiles := exactResolutionDiagramRequiredFiles(ctx, exactResolutionContract)
	reports := make([]ground.Report, len(built))
	valueTransferClassificationRepairs := make([]emitEvidenceValueTransferClassificationRepair, 0)
	assignmentEndpointRepairs := make([]emitEvidenceAssignmentEndpointRepair, 0)
	callEndpointRepairs := make([]emitEvidenceCallEndpointRepair, 0)
	argumentFlowRepairs := make([]emitEvidenceArgumentFlowRepair, 0)
	registrationBindingRepairs := make([]emitEvidenceRegistrationBindingRepair, 0)
	surfaceTermDrops := make([]string, 0)
	surfaceAlignmentRejects := make([]string, 0)
	surfaceAlignmentRejected := make(map[int]bool)
	groundingStart := time.Now()
	for i := range built {
		// A callable passed to an executor/framework is a typed handoff, not a
		// direct invocation. Classify that exact-line shape before call-site
		// realignment so nearest_call cannot steal the item onto a sibling call.
		normalizeCallbackHandoffEvidence(&built[i], gc)
		// A call row may carry the right explicit caller/callee pair but cite a
		// nearby call to the same leaf (for example collect_files -> walk cited
		// on walk's later recursive self-call).  Recover the exact already-read
		// same-file callsite from BOTH typed endpoints before canonicalising the
		// row from the cited line.  Otherwise the normaliser truthfully changes
		// the row to walk -> walk, while the model can mistake its now-stale
		// summary for proof that collect_files -> walk survived.
		realignExplicitCallEvidenceLine(&built[i], gc)
		// Call evidence has two identities: the enclosing caller and the
		// invoked callee. The wire model occasionally puts the caller in
		// anchor_symbol even though AnchorCall defines that field as the
		// callee. Canonicalise from the exact parser relation + enclosing
		// callable BEFORE grounding so the first verdict checks the same
		// caller/callee tuple consumed by diagram and relation gates. The old
		// post-grounding order could turn a line-text-visible call into
		// recovered(nearest_call), then stamp the corrected owner/object while
		// leaving the weaker verdict behind.
		normalizeCallEvidenceDirection(&built[i], gc)
		// Per-scope dispatch: ScopeLine routes to the existing tier
		// cascade; schema-level scopes (File / Crossfile / Negative)
		// route to their own grounders.
		r := ground.GroundItemScoped(&built[i], gc)
		// One callback expression carries two independently useful source
		// relations: the enclosing callable invokes the receiving API, and that
		// API receives the callable value.  The callback normalizer above keeps
		// the second relation.  For a typed call/flow investigation, preserve the
		// first as an exact model re-emit obligation when the parser owns one
		// unique caller -> receiver tuple.  This never mints the sibling edge.
		if repair, ok := emitEvidenceRequiredCallbackReceiverCallRepair(
			ctx, built[i], builtParamIndexes[i], gc,
		); ok {
			callEndpointRepairs = append(callEndpointRepairs, repair)
		}
		// AnchorCall is downstream hard authority for a direct invocation. A
		// line-range mention, quoted tool name, selection branch, or other
		// source-text occurrence may still ground lexically, but it must not be
		// published as a call edge unless the exact source line and typed
		// caller/callee tuple prove that invocation. Preserve the useful source
		// reference while downgrading only its relation authority. This consumes
		// parser/read-line facts and typed fields; request, summary, and final
		// answer prose are not inspected.
		exactCallCaller, exactCallCallee, exactCallKnown := exactCallEvidenceDirection(&built[i], gc)
		callItemBeforeDowngrade := built[i]
		if stabilizeUnprovenCallAnchorAuthority(&built[i], gc) {
			if repair, ok := emitEvidenceRequiredRelationCallEndpointRepair(
				ctx, callItemBeforeDowngrade, builtParamIndexes[i], exactCallCaller, exactCallCallee, exactCallKnown,
			); ok {
				callEndpointRepairs = append(callEndpointRepairs, repair)
			}
			r = ground.GroundItemScoped(&built[i], gc)
			note := "anchor_kind=call lacked one exact-line caller -> callee invocation; preserved as text_reference without call-edge authority"
			if exactCallKnown {
				note = fmt.Sprintf(
					"anchor_kind=call used semantic or mismatched endpoints, so it was preserved as text_reference without call-edge authority; the exact source/parser call tuple is evidence_kind=relationship subject=%q predicate=calls object=%q anchor_symbol=%q — re-emit that call row exactly, and keep any broader data-flow/write claim separate unless its own assignment/initializer/return operation proves it",
					exactCallCaller, exactCallCallee, exactCallCallee)
			}
			if appendGroundingNoteOnce(&built[i], note) {
				r.Note = built[i].GroundingNote
			}
		}
		if compatNote := normalizeConditionEvidenceAnchorSymbol(&built[i], gc); compatNote != "" {
			r = ground.GroundItemScoped(&built[i], gc)
			if appendGroundingNoteOnce(&built[i], compatNote) {
				r.Note = built[i].GroundingNote
			}
		}
		if stabilizeStringLiteralIdentifierAnchor(&built[i], gc) {
			r = ground.GroundItemScoped(&built[i], gc)
			compatNote := "anchor_kind=string_literal was treated as definition because anchor_symbol matched a non-comment identifier on the cited source line, while no source-code literal span matched that exact value"
			if built[i].AnchorKind == types.AnchorTextReference {
				compatNote = "anchor_kind=string_literal was treated as text_reference because anchor_symbol matched visible non-comment source text on the cited line, while no source-code literal span matched that exact value"
			}
			if appendGroundingNoteOnce(&built[i], compatNote) {
				r.Note = built[i].GroundingNote
			}
		}
		if compatNote := normalizeRegistrationInitializerAnchor(&built[i], gc); compatNote != "" {
			r = ground.GroundItemScoped(&built[i], gc)
			if appendGroundingNoteOnce(&built[i], compatNote) {
				r.Note = built[i].GroundingNote
			}
		}
		// A required flow investigation can already have the exact writer line
		// in hand while the model labels that line as a definition/mechanism.
		// Detect the unique syntax-owned transfer tuple and return a copy-ready
		// structured repair.  The accepted row remains unchanged; the system
		// supplies precise source facts but never silently promotes them into a
		// relation or draws an answer edge.
		if repair, ok := emitEvidenceRequiredFlowValueTransferClassificationRepair(
			ctx, built[i], builtParamIndexes[i], gc,
		); ok {
			valueTransferClassificationRepairs = append(valueTransferClassificationRepairs, repair)
		}
		// A simple assignment-shaped line proves only its exact LHS <- RHS transfer.
		// It cannot authorize a model-authored enclosing function, nearby
		// participant, or other visible token as a directed endpoint. Grounding
		// has attached the exact observed source line to Snippet, so validate the
		// tuple here and remove only relation authority on mismatch. The source
		// observation remains available as a text reference and the repair note
		// teaches the exact line-local tuple without reading request/final prose.
		if repair, ok := emitEvidenceRequiredFlowAssignmentEndpointRepair(ctx, built[i], builtParamIndexes[i]); ok {
			assignmentEndpointRepairs = append(assignmentEndpointRepairs, repair)
		}
		// An assignment/initializer line may also contain the only exact call
		// operation that receives a requested carrier.  Do not make argument
		// evidence depend on whether the model happened to classify that same
		// line as AnchorCall: when the parser owns exactly one call, expose its
		// complete incident argument tuple as model re-emit debt.  This closes
		// the anchor-entrypoint asymmetry without minting an edge or reading
		// request/answer prose.
		argumentFlowRepairs = append(argumentFlowRepairs,
			emitEvidenceRequiredAssignmentCallArgumentFlowRepairs(ctx, built[i], builtParamIndexes[i], gc)...)
		if compatNote := stabilizeAssignmentEndpointAuthority(&built[i]); compatNote != "" {
			r = ground.GroundItemScoped(&built[i], gc)
			if appendGroundingNoteOnce(&built[i], compatNote) {
				r.Note = built[i].GroundingNote
			}
		}
		if stampEvidenceOwnerSymbol(&built[i], gc) {
			r.Status = built[i].GroundingStatus
			r.Tier = built[i].GroundingTier
			r.Note = built[i].GroundingNote
		}
		if stampEvidenceTypedIdentityBindings(&built[i], gc) {
			r.Status = built[i].GroundingStatus
			r.Tier = built[i].GroundingTier
			r.Note = built[i].GroundingNote
		}
		if stabilizeLineLocalCallableOwner(&built[i], gc) {
			r.Status = built[i].GroundingStatus
			r.Tier = built[i].GroundingTier
			r.Note = built[i].GroundingNote
		}
		if stabilizeStatementLocalAnchorClaim(&built[i], gc) {
			r.Status = built[i].GroundingStatus
			r.Tier = built[i].GroundingTier
			r.Note = built[i].GroundingNote
		}
		// A grounded call anchor proves the invoked callee, not every endpoint
		// the model happened to place in a registration/relationship row.  Keep
		// non-call relation endpoints line-local unless the call normalizer above
		// already replaced them with the exact parser-owned caller -> callee edge.
		// This closes the authority-laundering path where a real wrapper call
		// (make_sink -> SinkRegistry::create) made an absent ConsoleSink endpoint
		// look typed and citable.  The check reads only structured evidence fields
		// and the already-read source line; it never scans request/final prose.
		if stabilizeCallAnchorRelationEndpointAuthority(&built[i], gc) {
			r.Status = built[i].GroundingStatus
			r.Tier = built[i].GroundingTier
			r.Note = built[i].GroundingNote
		}
		// One exact call row can also contain a complete data argument that is
		// statically bound to an incident-required carrier participant. Run this
		// only after every call-authority stabilizer, then publish a separate,
		// copy-ready model re-emit obligation. This is parser-owned source truth,
		// not a system-authored EvidenceItem or diagram edge.
		argumentFlowRepairs = append(argumentFlowRepairs,
			emitEvidenceRequiredCallArgumentFlowRepairs(ctx, built[i], builtParamIndexes[i], gc)...)
		// Registration is typed selection authority. The concrete selected
		// endpoint must therefore occur on the cited binding surface (or be a
		// parser/read-file-proven attached decorator), independent of which
		// syntactic anchor kind the model chose. This prevents a function
		// definition or wrapper line from laundering a target that appears only
		// in free-form summary prose. Factory branches remain representable by
		// the separate condition + return forms taught by the shared guide.
		registrationAlignmentRejected := false
		if err := validateRequestedDecoratorRegistrationAlignment(i, built[i], gc, ctx); err != nil {
			surfaceAlignmentRejects = append(surfaceAlignmentRejects, err.Error())
			surfaceAlignmentRejected[i] = true
			registrationAlignmentRejected = true
		}
		registrationBeforeEndpointAudit := built[i]
		if !registrationAlignmentRejected {
			if stabilizeRegistrationEndpointAuthority(&built[i], gc) {
				r.Status = built[i].GroundingStatus
				r.Tier = built[i].GroundingTier
				r.Note = built[i].GroundingNote
			}
			// A wrong source-shape anchor can fail initial grounding before the
			// endpoint stabilizer runs.  Conversely, a citable declaration can
			// prove only a registration container while hiding the unique exact
			// binding expression in its already-read body. Audit both shapes at
			// this common seam, then remove the obligation below when the exact
			// model-owned row already exists in this or an earlier batch. The
			// system never promotes the coarse row or mints a binding edge.
			if repair, ok := emitEvidenceRequiredRegistrationBindingRepair(
				ctx, registrationBeforeEndpointAudit, builtParamIndexes[i], gc,
			); ok {
				registrationBindingRepairs = append(registrationBindingRepairs, repair)
			}
		}
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
		built[i].ID = types.StableEvidenceID(built[i])
		r.ItemID = built[i].ID
		reports[i] = r
		if drops := dropInvalidEvidenceSurfaceTerms(i, &built[i], gc); len(drops) > 0 {
			surfaceTermDrops = append(surfaceTermDrops, drops...)
		}
	}
	recordToolRuntimeTiming(&runtimeTimings, "per_item_grounding_stabilize", groundingStart, len(built))
	valueTransferClassificationRepair := buildEmitEvidenceValueTransferClassificationRepair(valueTransferClassificationRepairs)
	assignmentEndpointRepair := buildEmitEvidenceAssignmentEndpointRepair(assignmentEndpointRepairs)
	repairEvidence := append([]types.EvidenceItem(nil), ctx.Mutable.EmittedEvidence()...)
	repairEvidence = append(repairEvidence, built...)
	callEndpointRepairs = filterSatisfiedCallbackReceiverCallRepairs(callEndpointRepairs, repairEvidence)
	argumentFlowRepairs = filterSatisfiedArgumentFlowRepairs(argumentFlowRepairs, repairEvidence)
	registrationBindingRepairs = filterSatisfiedRegistrationBindingRepairs(registrationBindingRepairs, repairEvidence)
	callEndpointRepair := buildEmitEvidenceCallEndpointRepair(callEndpointRepairs)
	relationEndpointRepair := mergeEmitEvidenceRelationEndpointRepairs(assignmentEndpointRepair, callEndpointRepair)
	relationEndpointRepair = mergeEmitEvidenceRelationEndpointRepairs(
		relationEndpointRepair, buildEmitEvidenceArgumentFlowRepair(argumentFlowRepairs))
	relationEndpointRepair = mergeEmitEvidenceRelationEndpointRepairs(
		relationEndpointRepair, buildEmitEvidenceRegistrationBindingRepair(registrationBindingRepairs))
	relationEndpointRepair = mergeEmitEvidenceValueTransferRepair(valueTransferClassificationRepair, relationEndpointRepair)
	if relationEndpointRepair != nil {
		validationRepairFields = append(validationRepairFields, relationEndpointRepair.Fields...)
	}
	if len(surfaceAlignmentRejects) > 0 {
		filteredBuilt := make([]types.EvidenceItem, 0, len(built)-len(surfaceAlignmentRejected))
		filteredReports := make([]ground.Report, 0, len(reports)-len(surfaceAlignmentRejected))
		for i := range built {
			if surfaceAlignmentRejected[i] {
				continue
			}
			filteredBuilt = append(filteredBuilt, built[i])
			if i < len(reports) {
				filteredReports = append(filteredReports, reports[i])
			}
		}
		built = filteredBuilt
		reports = filteredReports
		rejectedItems = append(rejectedItems, surfaceAlignmentRejects...)
		if len(built) == 0 {
			return failEmit(t.Name(), now,
				"no valid items after evidence surface alignment:\n%s",
				strings.Join(surfaceAlignmentRejects, "\n"))
		}
	}

	// 2026-05-03 (Phase 6 stage 2): retired the codename-grounding
	// filter (validateEvidenceSummaryCodenameGrounding). Identifier
	// grounding for the answer's surface fields is now enforced by
	// Phase 4's runStepIdentifierBackedByEvidenceOracle (typed
	// evidence pool membership). The upstream emit_evidence side
	// no longer needs a separate prose-level codename gate — typed-
	// pool membership at consume time is the structural authority.

	// AuthorityCeiling axis: project each grounded item to (Origin,
	// Authority, AuthorityReason, DriftReason) before persisting.
	// Pure function of the item + bus state, so a later replay
	// computes the same projection.
	for i := range built {
		proj := authority.ComputeForEvidence(built[i], ctx)
		built[i].Origin = proj.Origin
		built[i].Authority = proj.Authority
		built[i].AuthorityReason = proj.Reason
		built[i].DriftReason = proj.DriftReason
	}

	// A model-selected definition can carry exact parser-authored calls in its
	// already-read body even when the model emitted only definition/mechanism
	// evidence. Publish those calls as separate deterministic source facts so
	// downstream reasoning is not forced to infer relations from a multiline
	// snippet. This never changes the model row and never selects an answer
	// path, bridge, edge, or conclusion.
	selectedBodyCalls := autoPairSelectedDefinitionBodyCallEvidence(ctx, built, gc)
	if len(selectedBodyCalls) > 0 {
		for i := range selectedBodyCalls {
			proj := authority.ComputeForEvidence(selectedBodyCalls[i], ctx)
			selectedBodyCalls[i].Origin = proj.Origin
			selectedBodyCalls[i].Authority = proj.Authority
			selectedBodyCalls[i].AuthorityReason = proj.Reason
			selectedBodyCalls[i].DriftReason = proj.DriftReason
		}
		built = append(built, selectedBodyCalls...)
		for _, call := range selectedBodyCalls {
			reports = append(reports, ground.Report{
				ItemID: call.ID, Status: call.GroundingStatus, Tier: call.GroundingTier,
				OriginalLine: call.LineStart, AdjustedLine: call.LineStart,
				Note: call.GroundingNote,
			})
		}
		logging.Debug("[emit_evidence] auto-paired %d parser-owned selected-definition body call evidence item(s)", len(selectedBodyCalls))
	}

	// Plan 2 v2 (2026-05-05) — deterministic role-description pairing.
	// For every grounded definition-anchor item, attempt to extract the
	// leading doc comment from the same source's read_file gutter and
	// append a parallel evidence_kind=mechanism item carrying the
	// role description. Skill prompt's Plan-2-v1 LLM pairing path
	// stays in place; this hook guarantees the pool covers cases
	// where the LLM forgets to emit the WHAT axis.
	autoPaired := autoPairRoleDescriptionEvidence(built, gc)
	if len(autoPaired) > 0 {
		built = append(built, autoPaired...)
		// Keep reports[] in lockstep with built[] for renderEmitSummary
		// — every auto-paired item is born grounded against the
		// read_file gutter, so its synthetic Report mirrors that.
		for _, ap := range autoPaired {
			reports = append(reports, ground.Report{
				ItemID: ap.ID, Status: ap.GroundingStatus, Tier: ap.GroundingTier,
				OriginalLine: ap.LineStart, AdjustedLine: ap.LineStart,
				Note: ap.GroundingNote,
			})
		}
		if ctx.Mutable != nil {
			ctx.Mutable.EvidenceClosure().BumpAutoPairedRoleDescriptions(len(autoPaired))
		}
		logging.Debug("[emit_evidence] Plan 2 v2 auto-paired %d role-description mechanism evidence items", len(autoPaired))
	}

	mergeStart := time.Now()
	priorEvidence := ctx.Mutable.EmittedEvidence()
	var duplicateItems []types.EvidenceItem
	built, reports, duplicateItems = filterNoopDuplicateEmitEvidence(priorEvidence, built, reports)
	amendedItems := emitEvidenceAmendedItems(priorEvidence, built)
	if len(built) > 0 {
		ctx.Mutable.AppendEvidence(built)
	}
	recordToolRuntimeTiming(&runtimeTimings, "duplicate_amendment_merge", mergeStart, len(built)+len(duplicateItems)+len(amendedItems))

	summaryStart := time.Now()
	surfaceReview := buildEmitEvidenceSurfaceTermReview(built, gc)
	allEvidence := ctx.Mutable.EmittedEvidence()
	summary := ""
	if len(built) > 0 {
		summary = renderEmitSummary(ctx, built, reports, allEvidence, validationRepairFields...)
	} else if len(duplicateItems) > 0 {
		summary = renderEmitEvidenceDuplicateNoopSummary(duplicateItems, allEvidence)
	}
	if len(duplicateItems) > 0 && len(built) > 0 {
		summary = strings.TrimRight(summary, "\n") + "\n\n" +
			renderEmitEvidenceDuplicateSkipNote(duplicateItems)
	}
	if len(amendedItems) > 0 {
		summary = strings.TrimRight(summary, "\n") + "\n\n" +
			renderEmitEvidenceAmendmentNote(amendedItems)
	}
	if surfaceReview != nil && strings.TrimSpace(surfaceReview.Hint) != "" {
		summary = strings.TrimRight(summary, "\n") + "\n\n" + surfaceReview.Hint + "\n"
	}
	if len(surfaceTermDrops) > 0 {
		summary = strings.TrimRight(summary, "\n") +
			"\n\nsurface_terms compatibility: dropped ungrounded optional term(s); evidence items were kept:\n  - " +
			strings.Join(surfaceTermDrops, "\n  - ") + "\n"
	}
	if len(rejectedItems) > 0 || len(softSkippedItems) > 0 || len(externalObservationSkippedItems) > 0 || len(autoSwapped) > 0 || len(compatRepairs) > 0 {
		var b strings.Builder
		b.WriteString(summary)
		if len(softSkippedItems) > 0 {
			fmt.Fprintf(&b, "\n%d command/VCS-derived item(s) were SKIPPED because emit_evidence only records source/log evidence anchors:\n",
				len(softSkippedItems))
			for _, r := range softSkippedItems {
				fmt.Fprintf(&b, "  - %s\n", r)
			}
			b.WriteString("Carry verified command values through emit_investigation_complete.aggregate_facts when they are the requested principal scalar/count/list. For non-scalar history summaries or diagnostics, carry the VCS finding in emit_investigation_complete.reason and mark commit/count metadata aggregates as supporting_coverage.\n")
		}
		if len(externalObservationSkippedItems) > 0 {
			fmt.Fprintf(&b, "\n%d external observation item(s) were SKIPPED because emit_evidence only records current-source/read_file-backed anchors:\n",
				len(externalObservationSkippedItems))
			for _, r := range externalObservationSkippedItems {
				fmt.Fprintf(&b, "  - %s\n", r)
			}
			b.WriteString("Runtime logs/traces, MCP resources, connector rows, web pages, and other non-current-source observations must remain in their external observation lane. Do not retry emit_evidence for these rows; carry them through emit_investigation_complete.reason and aggregate_facts instead of inventing read_file grounding.\n")
		}
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
		if len(compatRepairs) > 0 {
			fmt.Fprintf(&b, "\n%d schema-shape compatibility repair(s) applied before validation:\n",
				len(compatRepairs))
			for _, r := range compatRepairs {
				fmt.Fprintf(&b, "  - %s\n", r)
			}
		}
		summary = b.String()
	}
	repair := buildEmitEvidenceRepair(ctx, built, reports)
	if relationEndpointRepair != nil {
		repair = relationEndpointRepair
	}
	if validationRepair := buildEmitEvidenceItemValidationRepair(
		rejectedItems, validationRepairFields, validationRepairBlocksCompletion); validationRepair != nil {
		// A skipped decoded item never entered the buffer, so line-grounding
		// repair construction cannot see it. Keep that local schema debt as the
		// current action-required repair; accepted siblings stay committed and
		// need not be re-emitted.
		repair = mergeEmitEvidenceValidationRepairs(validationRepair, relationEndpointRepair)
	}
	if repair == nil || (repair.Metadata != nil && repair.Metadata["repair_status"] != types.ToolRepairStatusActionRequired) {
		if surfaceReview != nil {
			repair = surfaceReview
		}
	}
	if len(built) == 0 && len(duplicateItems) > 0 {
		repair = emitEvidenceDuplicateNoopRepair(len(duplicateItems))
	}
	// ToolRepair is a typed cross-stage carrier, but the same-turn model only
	// receives ToolResult.Summary.  Surface an exact completion-blocking item
	// repair immediately so the model can correct the named row before an
	// unrelated successful emit obscures the debt.  This is producer-authored
	// JSON guidance, not request/answer prose inspection and not relation
	// synthesis.
	if repair != nil && repair.Code == types.ToolRepairCodeEvidenceItemValidation &&
		repair.Metadata != nil && repair.Metadata["repair_status"] == types.ToolRepairStatusActionRequired &&
		repair.Metadata["completion_blocking"] == "true" && strings.TrimSpace(repair.Hint) != "" &&
		!strings.Contains(summary, repair.Hint) {
		summary = strings.TrimRight(summary, "\n") + "\n\nAction-required typed evidence repair:\n" + repair.Hint + "\n"
	}
	recordToolRuntimeTiming(&runtimeTimings, "summary_repair_render", summaryStart, len(built)+len(duplicateItems))
	return types.ToolResult{
		ToolName:  t.Name(),
		Repair:    attachToolJSONSurfaceMetadata(t.Name(), repair),
		Success:   true,
		Summary:   summary,
		Timestamp: now,
	}, nil
}

func attachEmitEvidenceSourceGraphResolver(ctx *types.BusContext, gc *ground.Context) {
	if ctx == nil || gc == nil || ctx.MultiGraph == nil {
		return
	}
	resolver, ok := ctx.MultiGraph.(interface {
		SourceGraphFile(string) (*repomap.Graph, *repomap.FileInfo, string, bool)
	})
	if !ok || resolver == nil {
		return
	}
	gc.SourceGraphFile = resolver.SourceGraphFile
}

// stabilizeUnprovenCallAnchorAuthority prevents a source mention from entering
// the answer context as a typed call edge. Exact call
// authority requires all of: line scope, a canonical call predicate, explicit
// caller/callee endpoints, and a direct invocation of the callee on that exact
// line proven by repomap or already-read source. Sparse genuine calls remain
// supported because normalizeCallEvidenceDirection runs first and fills this
// tuple from repomap.
//
// When the tuple cannot be proven, the source span is still useful as a text
// reference. Clearing relation fields before the downgrade is essential:
// otherwise a non-call predicate/object could be rendered as a directed edge
// even though ClaimFormOf correctly changed to text_reference_fact.
func stabilizeUnprovenCallAnchorAuthority(it *types.EvidenceItem, gc *ground.Context) bool {
	if it == nil || it.AnchorKind != types.AnchorCall ||
		types.ClaimFormOf(*it) != types.ClaimCallEdge {
		return false
	}
	predicate := strings.ToLower(strings.TrimSpace(it.Predicate))
	caller := strings.TrimSpace(it.Subject)
	callee := strings.TrimSpace(it.Object)
	parserProvesCall := false
	ownerGraphAvailable := false
	ownerCallerProven := false
	if gc != nil && caller != "" && callee != "" {
		_, fi, _, _, ok := ground.ResolveSourceGraphFile(gc, it.Source)
		if ok {
			ownerGraphAvailable = true
			actualCaller := enclosingCallableSymbolName(fi, it.LineStart)
			ownerCallerProven = qualifiedCallEndpointEqual(actualCaller, caller)
			rel, ok := findCallRelationAtLineForCandidates(fi, it.LineStart, []string{callee})
			parserProvesCall = ok && ownerCallerProven &&
				qualifiedCallEndpointEqual(rel.ToEP.Name, callee)
		}
	}
	lineProvesCall := ground.LineHasDirectCallToTarget(gc, it.Source, it.LineStart, callee)
	directionProven := parserProvesCall || (lineProvesCall && (!ownerGraphAvailable || ownerCallerProven))
	if it.Scope == types.ScopeLine && it.LineStart > 0 && it.LineEnd == it.LineStart &&
		callLikePredicates[predicate] &&
		caller != "" && callee != "" &&
		directionProven {
		return false
	}
	it.AnchorKind = types.AnchorTextReference
	it.Subject = ""
	it.Predicate = ""
	it.Object = ""
	it.OwnerSymbol = ""
	return true
}

func emitEvidenceGroundContextTimingPhase(gc *ground.Context) string {
	if gc == nil {
		return "ground_context"
	}
	switch strings.TrimSpace(gc.CacheStatus) {
	case "cache_hit":
		return "ground_context_cache_hit"
	case "cache_miss":
		return "ground_context_cache_miss"
	default:
		return "ground_context"
	}
}

func repairEmitEvidenceKnownCompatFields(raw json.RawMessage) (json.RawMessage, []string, bool) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return raw, nil, false
	}
	itemsRaw, ok := probe["items"]
	if !ok {
		return raw, nil, false
	}
	itemsTrimmed := bytes.TrimSpace(itemsRaw)
	if len(itemsTrimmed) == 0 || itemsTrimmed[0] != '[' {
		return raw, nil, false
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(itemsRaw, &items); err != nil {
		return raw, nil, false
	}
	var paths []string
	for i := range items {
		if _, ok := items[i]["field"]; ok {
			delete(items[i], "field")
			paths = append(paths, fmt.Sprintf("items[%d].field", i))
		}
		if repairEmitEvidenceRedundantSupportRefs(items[i]) {
			paths = append(paths, fmt.Sprintf("items[%d].support_refs(redundant source:line)", i))
		}
		paths = append(paths, repairEmitEvidenceItemConstraintSidecars(items[i], i)...)
	}
	items, adjacentPaths := repairEmitEvidenceAdjacentItemMetadataFragments(items)
	paths = append(paths, adjacentPaths...)
	if len(items) == 1 {
		for _, field := range emitEvidenceItemOnlyCompatFields {
			value, exists := probe[field]
			if !exists {
				continue
			}
			if _, already := items[0][field]; already {
				continue
			}
			items[0][field] = value
			delete(probe, field)
			paths = append(paths, fmt.Sprintf("$.%s->items[0].%s", field, field))
		}
	}
	for i := range items {
		if repairEmitEvidenceLoadBearingSummaryString(items[i]) {
			paths = append(paths, fmt.Sprintf("items[%d].load_bearing_summary string->bool", i))
		}
	}
	if len(paths) == 0 {
		return raw, nil, false
	}
	patchedItems, err := json.Marshal(items)
	if err != nil {
		return raw, nil, false
	}
	probe["items"] = patchedItems
	patched, err := json.Marshal(probe)
	if err != nil {
		return raw, nil, false
	}
	return patched, paths, true
}

// repairEmitEvidenceAdjacentItemMetadataFragments repairs a narrow malformed-
// JSON shape produced by some function-calling models: an optional item field
// is emitted as the next array object instead of as a member of the evidence
// object immediately before it, for example:
//
//	[{"kind":"direct", ...}, {"salience":"load_bearing"}]
//
// The repair is lossless only when the fragment contains exclusively known
// item-metadata fields, follows a substantive evidence object, and none of the
// fields already exists on that object. Every other shape is left untouched so
// strict decode/per-item validation remains fail-loud rather than guessing an
// owner. The rule is language- and content-agnostic; it never reads request or
// answer prose.
func repairEmitEvidenceAdjacentItemMetadataFragments(items []map[string]json.RawMessage) ([]map[string]json.RawMessage, []string) {
	if len(items) < 2 {
		return items, nil
	}
	allowed := make(map[string]bool, len(emitEvidenceItemOnlyCompatFields))
	for _, field := range emitEvidenceItemOnlyCompatFields {
		allowed[field] = true
	}
	out := make([]map[string]json.RawMessage, 0, len(items))
	outIndexes := make([]int, 0, len(items))
	var paths []string
	for index, item := range items {
		if len(item) == 0 || !emitEvidenceMetadataOnlyFragment(item, allowed) || len(out) == 0 {
			out = append(out, item)
			outIndexes = append(outIndexes, index)
			continue
		}
		previous := out[len(out)-1]
		if !emitEvidenceSubstantiveItem(previous, allowed) {
			out = append(out, item)
			outIndexes = append(outIndexes, index)
			continue
		}
		conflict := false
		for field := range item {
			if _, exists := previous[field]; exists {
				conflict = true
				break
			}
		}
		if conflict {
			out = append(out, item)
			outIndexes = append(outIndexes, index)
			continue
		}
		ownerIndex := outIndexes[len(outIndexes)-1]
		for field, value := range item {
			previous[field] = value
			paths = append(paths, fmt.Sprintf("items[%d].%s->items[%d].%s", index, field, ownerIndex, field))
		}
	}
	return out, paths
}

func emitEvidenceMetadataOnlyFragment(item map[string]json.RawMessage, allowed map[string]bool) bool {
	for field := range item {
		if !allowed[field] {
			return false
		}
	}
	return len(item) > 0
}

func emitEvidenceSubstantiveItem(item map[string]json.RawMessage, metadata map[string]bool) bool {
	for field := range item {
		if !metadata[field] {
			return true
		}
	}
	return false
}

// repairEmitEvidenceRedundantSupportRefs absorbs one recurring cross-tool
// carrier mix-up only when it is provably lossless. emit_evidence already
// carries one exact source:line per item; an item-local support_refs array that
// contains only labels ending in that same source:line adds no fact or
// association. Removing that invalid duplicate lets strict decode continue.
// A ref to any other location remains untouched and therefore fails loud as an
// unknown field — it may carry a relationship that belongs in a later
// aggregate_facts member_set and must not be guessed away here.
func repairEmitEvidenceRedundantSupportRefs(item map[string]json.RawMessage) bool {
	rawRefs, ok := item["support_refs"]
	if !ok {
		return false
	}
	var refs []string
	if err := json.Unmarshal(rawRefs, &refs); err != nil || len(refs) == 0 {
		return false
	}
	var source string
	if err := json.Unmarshal(item["source"], &source); err != nil || strings.TrimSpace(source) == "" {
		return false
	}
	var line FlexInt
	if err := json.Unmarshal(item["line_start"], &line); err != nil || line.Int() <= 0 {
		return false
	}
	want := strings.TrimSpace(source) + ":" + strconv.Itoa(line.Int())
	for _, ref := range refs {
		ref = strings.TrimSpace(strings.Trim(ref, "`"))
		if ref != want && !strings.HasSuffix(ref, " "+want) {
			return false
		}
	}
	delete(item, "support_refs")
	return true
}

func repairEmitEvidenceItemConstraintSidecars(item map[string]json.RawMessage, index int) []string {
	if item == nil {
		return nil
	}
	var paths []string
	for _, wrapper := range emitEvidenceConstraintSidecarFields {
		raw, ok := item[wrapper]
		if !ok {
			continue
		}
		var sidecar map[string]json.RawMessage
		if err := json.Unmarshal(raw, &sidecar); err != nil {
			continue
		}
		promoted := 0
		for _, field := range emitEvidenceConstraintSidecarPromotableFields {
			value, ok := sidecar[field]
			if !ok {
				continue
			}
			if _, exists := item[field]; exists {
				continue
			}
			item[field] = value
			promoted++
			paths = append(paths, fmt.Sprintf("items[%d].%s.%s->items[%d].%s", index, wrapper, field, index, field))
		}
		delete(item, wrapper)
		if promoted == 0 {
			paths = append(paths, fmt.Sprintf("items[%d].%s ignored", index, wrapper))
		}
	}
	return paths
}

var emitEvidenceConstraintSidecarFields = []string{
	"field_constraints",
	"fieldConstraints",
}

var emitEvidenceConstraintSidecarPromotableFields = []string{
	"scope",
	"source",
	"line_start",
	"line_end",
	"anchor_kind",
	"anchor_symbol",
}

func repairEmitEvidenceLoadBearingSummaryString(item map[string]json.RawMessage) bool {
	if item == nil {
		return false
	}
	raw, ok := item["load_bearing_summary"]
	if !ok {
		return false
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return false
	}
	text = strings.TrimSpace(text)
	if parsed, err := strconv.ParseBool(strings.ToLower(text)); err == nil {
		item["load_bearing_summary"], _ = json.Marshal(parsed)
		return true
	}
	if text == "" {
		item["load_bearing_summary"], _ = json.Marshal(false)
		return true
	}
	if existingRaw, ok := item["summary"]; ok {
		var existing string
		if err := json.Unmarshal(existingRaw, &existing); err == nil {
			existing = strings.TrimSpace(existing)
			if existing == "" {
				item["summary"], _ = json.Marshal(text)
			} else if !strings.Contains(existing, text) {
				item["summary"], _ = json.Marshal(existing + " " + text)
			}
		}
	} else {
		item["summary"], _ = json.Marshal(text)
	}
	item["load_bearing_summary"], _ = json.Marshal(true)
	return true
}

var emitEvidenceItemOnlyCompatFields = []string{
	"salience",
	"load_bearing_summary",
	"context_role_hint",
	"diagram_role_hint",
}

var emitEvidenceMisplacedHints = []MisplacedFieldHint{
	{
		Field:          "salience",
		ContainerNames: []string{"the top-level tool payload"},
		CorrectPaths:   []string{"items[i].salience"},
	},
	{
		Field:          "load_bearing_summary",
		ContainerNames: []string{"the top-level tool payload"},
		CorrectPaths:   []string{"items[i].load_bearing_summary"},
	},
	{
		Field:          "context_role_hint",
		ContainerNames: []string{"the top-level tool payload"},
		CorrectPaths:   []string{"items[i].context_role_hint"},
	},
	{
		Field:          "diagram_role_hint",
		ContainerNames: []string{"the top-level tool payload"},
		CorrectPaths:   []string{"items[i].diagram_role_hint"},
	},
}

// buildEmitEvidenceItemWithSwap wraps buildEmitEvidenceItem with the
// 2026-04-17 line_end<line_start auto-swap. If the decoded item has
// line_end < line_start AND line_end > 0, swap the two values before
// delegating. Records the item index into autoSwapped so the caller
// can surface a "double-check the range" warning. All other
// validation errors flow through buildEmitEvidenceItem unchanged.
func buildEmitEvidenceItemWithSwap(in *emitEvidenceItem, index int, workDir string, gc *ground.Context, autoSwapped *[]int, compatRepairs *[]string) (types.EvidenceItem, error) {
	if in.LineStart.Int() > 0 && in.LineEnd.Int() > 0 && in.LineEnd.Int() < in.LineStart.Int() {
		// Obvious transposition typo — repair rather than reject.
		swappedStart, swappedEnd := in.LineEnd, in.LineStart
		in.LineStart, in.LineEnd = swappedStart, swappedEnd
		*autoSwapped = append(*autoSwapped, index)
	}
	repairEmitEvidenceItemShape(in, index, compatRepairs)
	repairMissingEmitEvidenceSource(in, index, gc, compatRepairs)
	repairMissingEmitEvidenceAnchorSymbol(in, index, gc, compatRepairs)
	return buildEmitEvidenceItem(*in, index, workDir)
}

func emitEvidenceToolValueSoftSkipReason(ctx *types.BusContext, in emitEvidenceItem, index int, perr error) (string, bool) {
	if perr == nil {
		return "", false
	}
	errText := perr.Error()
	if types.EvidenceScope(strings.ToLower(strings.TrimSpace(in.Scope))) == types.ScopeFile &&
		strings.Contains(errText, "scope=file requires file_role_label") &&
		emitEvidenceLooksLikeDirectoryMeasurement(in) {
		return fmt.Sprintf("items[%d]: directory/file-set measurement `%s` has no file-identity role; it belongs in aggregate_facts", index, strings.TrimSpace(in.Source)), true
	}
	if !in.LoadBearingSummary {
		return "", false
	}
	if !strings.Contains(errText, "scope=line requires line_start > 0") &&
		!strings.Contains(errText, "source is required") &&
		!strings.Contains(errText, "does not look like a repo-relative file path") {
		return "", false
	}
	payloadParts := []string{
		strings.TrimSpace(in.Snippet),
		strings.TrimSpace(in.AnchorSymbol),
		strings.TrimSpace(in.Summary),
	}
	for _, payload := range payloadParts {
		if payload == "" {
			continue
		}
		if value, ok := types.DeterministicCountProofInteger(payload); ok {
			return fmt.Sprintf("items[%d]: command-derived scalar/count `%d` has no source file:line anchor", index, value), true
		}
	}
	if reason, ok := emitEvidenceHistoryMetadataSoftSkipReason(ctx, in, index); ok {
		return reason, true
	}
	return "", false
}

func emitEvidenceHistoryMetadataSoftSkipReason(ctx *types.BusContext, in emitEvidenceItem, index int) (string, bool) {
	if ctx == nil || ctx.AnalysisIR == nil || !ctx.AnalysisIR.RequestModel.Predicates.IsHistoryLookup {
		return "", false
	}
	if !emitEvidenceLooksLikeVCSMetadata(in) {
		return "", false
	}
	label := strings.TrimSpace(in.AnchorSymbol)
	if label == "" {
		label = strings.TrimSpace(in.Subject)
	}
	if label == "" {
		label = strings.TrimSpace(in.Summary)
	}
	if label == "" {
		label = "history metadata"
	}
	return fmt.Sprintf("items[%d]: VCS/history metadata `%s` has no repo file:line anchor", index, logging.Truncate(label, 80)), true
}

func emitEvidenceLooksLikeVCSMetadata(in emitEvidenceItem) bool {
	source := normalizeEmitEvidenceVCSMetadataSource(in.Source)
	if emitEvidenceSourceIsVCSMetadata(source) {
		return true
	}
	if source == "" || source == "exec_command" || source == "shell" || source == "command" {
		return emitEvidencePayloadLooksLikeVCSMetadata(in)
	}
	return false
}

func normalizeEmitEvidenceVCSMetadataSource(source string) string {
	s := strings.TrimSpace(strings.ReplaceAll(source, `\`, `/`))
	s = strings.Trim(s, "`\"'")
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	return s
}

func emitEvidenceSourceIsVCSMetadata(source string) bool {
	switch source {
	case "git_log", "git log", "git_show", "git show", "git_diff", "git diff",
		"git_history_search", "git history search", "exec_command_git_history",
		"vcs_metadata", "vcs_diff":
		return true
	}
	return strings.HasPrefix(source, "git ") ||
		strings.HasPrefix(source, "git_") ||
		strings.HasPrefix(source, "tool:git") ||
		strings.HasPrefix(source, "exec_command: git ") ||
		strings.Contains(source, "git_history")
}

func emitEvidencePayloadLooksLikeVCSMetadata(in emitEvidenceItem) bool {
	for _, payload := range []string{
		in.AnchorSymbol,
		in.Subject,
		in.Object,
		in.Snippet,
		in.Summary,
	} {
		payload = strings.TrimSpace(payload)
		if payload == "" {
			continue
		}
		lower := strings.ToLower(payload)
		if emitEvidenceCommitHashTokenRe.MatchString(payload) ||
			strings.Contains(lower, "git log") ||
			strings.Contains(lower, "git show") ||
			strings.Contains(lower, "git diff") ||
			strings.Contains(lower, "commit ") ||
			strings.Contains(lower, "commit:") ||
			strings.Contains(lower, "author:") {
			return true
		}
	}
	return false
}

func emitEvidenceLooksLikeDirectoryMeasurement(in emitEvidenceItem) bool {
	source := strings.TrimSpace(in.Source)
	if source == "" || path.Ext(path.Base(filepath.ToSlash(source))) != "" {
		return false
	}
	if strings.TrimSpace(in.FileRoleLabel) != "" {
		return false
	}
	return strings.TrimSpace(in.Summary) != "" ||
		strings.TrimSpace(in.Subject) != "" ||
		strings.TrimSpace(in.Object) != ""
}

func renderEmitEvidenceCommandScalarSoftSkipSummary(skipped []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "emit_evidence accepted 0 source evidence item(s); skipped %d command/VCS support item(s).\n\n", len(skipped))
	for _, s := range skipped {
		fmt.Fprintf(&b, "  - %s\n", s)
	}
	b.WriteString("\nDerived scalar/count, directory measurement, and VCS/history outputs are not source-line evidence. Do not invent a file:line anchor for them. Carry verified scalar/count/list answers through emit_investigation_complete.aggregate_facts when that is the requested principal shape; for non-scalar history summaries or diagnostics, carry the VCS finding in emit_investigation_complete.reason and keep commit/count metadata aggregates as supporting_coverage.\n")
	return b.String()
}

func emitEvidenceExternalObservationSoftSkipReason(ctx *types.BusContext, in emitEvidenceItem, index int) (string, bool) {
	source := strings.TrimSpace(in.Source)
	kind := emitEvidenceExternalObservationSourceKindForContext(ctx, source)
	if kind == "" {
		return "", false
	}
	anchor := strings.TrimSpace(firstNonEmptyString([]string{in.AnchorSymbol, in.Subject, in.Object}))
	if anchor == "" {
		anchor = "(no anchor)"
	}
	line := in.LineStart.Int()
	location := source
	if line > 0 {
		location = fmt.Sprintf("%s:%d", source, line)
	}
	return fmt.Sprintf("items[%d]: %s @ %s is a %s external observation, not a current-source read_file anchor", index, anchor, location, kind), true
}

func emitEvidenceSourceIsExternalObservationURI(source string) bool {
	source = strings.ToLower(strings.TrimSpace(source))
	return strings.HasPrefix(source, "mcp://") || strings.HasPrefix(source, "mcp:/")
}

// emitEvidenceExternalObservationSourceKindForContext is the context-aware
// classifier. It first asks the shared trace_query blob-ref registry
// (Q5-A P1-2 ②): a blob the escape lane opened is runtime state, and the
// model may name it by BASENAME — which RuntimeArtifactPathKind's
// extension shapes below would miss. A hit soft-reroutes the item into
// the external-observation lane (reusing the runtime_artifact wording),
// so blob-sourced rows never reach GroundItem's current-source grounder.
// This is the same registry builtin.go readFileTypedSourcePath and
// extractor.go extractorEvidenceRepoSource consult — one matcher, four
// surfaces, no drift.
func emitEvidenceExternalObservationSourceKindForContext(ctx *types.BusContext, source string) string {
	if ctx != nil && ctx.Mutable != nil {
		if _, ok := ctx.Mutable.ResolveTraceQueryBlobRef(source); ok {
			return "runtime_artifact"
		}
	}
	return emitEvidenceExternalObservationSourceKind(source)
}

func emitEvidenceExternalObservationSourceKind(source string) string {
	lower := strings.ToLower(strings.TrimSpace(source))
	runtimeKind := types.RuntimeArtifactPathKind(source)
	switch {
	case emitEvidenceSourceIsExternalObservationURI(lower):
		return "resource"
	case runtimeKind == "log":
		return "runtime_log"
	case runtimeKind != "":
		return "runtime_artifact"
	case lower == "runtime log (unresolved)":
		return "runtime_log"
	case lower == "runtime trace (unresolved)" || lower == "runtime artifact (unresolved)":
		return "runtime_artifact"
	case lower == string(types.TypedDenialExternalPerfStallUnresolved):
		return "runtime_artifact"
	default:
		return ""
	}
}

func renderEmitEvidenceExternalObservationSoftSkipSummary(ctx *types.BusContext, skipped []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "emit_evidence accepted 0 source evidence item(s); skipped %d external observation item(s).\n\n", len(skipped))
	for _, s := range skipped {
		fmt.Fprintf(&b, "  - %s\n", s)
	}
	b.WriteString("\nRuntime logs/traces, MCP resources, connector rows, web pages, and other external observations are first-class evidence in the external observation lane, not current-source read_file evidence. Preserve them through emit_investigation_complete.reason plus aggregate_facts; do not retry emit_evidence for these rows.\n")
	// When the completion citation floor is already waived for this turn
	// (explicit exclusion, artifact-only checkout, accepted waiver, external
	// log/trace, or typed runtime observations), say so: without this line
	// the model reads "don't retry emit_evidence" here and "needs ≥N
	// citations" from the completion gate as a contradiction and loops
	// between the two surfaces (trace_repl.log 2026-07-02).
	if emitEvidenceCompletionCitationFloorWaived(ctx) {
		b.WriteString("This turn's completion does not require current-source citations: once the runtime observations answer the question, call emit_investigation_complete directly with the conclusion in reason plus aggregate_facts.\n")
	}
	return b.String()
}

func emitEvidenceCompletionCitationFloorWaived(ctx *types.BusContext) bool {
	if ctx == nil {
		return false
	}
	if _, ok := explicitCurrentSourceExclusionCompletionBypassLabel(ctx); ok {
		return true
	}
	if _, ok := zeroCurrentSourceRepoCompletionBypassLabel(ctx); ok {
		return true
	}
	// Only the aggregate-facts-INDEPENDENT bypasses may back this promise:
	// the trace_query / origin-specific branches are re-evaluated at
	// completion time against the actual aggregate_facts payload, so
	// promising "no citations needed" from a nil-facts evaluation here
	// could be contradicted by the gate one call later.
	if _, ok := repoGroundingBypassLabel(ctx); ok {
		return true
	}
	return false
}

func emitEvidenceExternalObservationRepair() *types.ToolRepair {
	return &types.ToolRepair{
		Code: types.ToolRepairCodeEvidenceExternalObservationToClosure,
		Hint: "MCP/resource/runtime/log/trace rows are external observations, not current-source read_file evidence. Carry verified rows through emit_investigation_complete.reason and aggregate_facts.",
		Fields: []string{
			"emit_investigation_complete.reason",
			"emit_investigation_complete.aggregate_facts",
		},
		Metadata: map[string]string{
			"repair_status": types.ToolRepairStatusAdvisory,
		},
	}
}

func emitEvidenceAbsenceCompletionRepairApplies(in emitEvidenceItem) bool {
	kind := strings.ToLower(strings.TrimSpace(firstNonEmptyString([]string{in.EvidenceKind, in.LegacyKind})))
	scope := types.EvidenceScope(strings.ToLower(strings.TrimSpace(in.Scope)))
	if kind == string(types.EvidenceAbsent) && scope != types.ScopeNegative {
		return true
	}
	if scope != types.ScopeNegative {
		return false
	}
	if kind != string(types.EvidenceAbsent) {
		return true
	}
	if in.NegativeQuery == nil ||
		strings.TrimSpace(in.NegativeQuery.File) == "" ||
		strings.TrimSpace(in.NegativeQuery.Pattern) == "" {
		return true
	}
	nscope := types.NegativeScope(strings.ToLower(strings.TrimSpace(in.NegativeScope)))
	if !nscope.IsValid() {
		return true
	}
	return nscope == types.NegativeScopeSection && strings.TrimSpace(in.NegativeQuery.Section) == ""
}

func emitEvidenceAbsenceCompletionRepair() *types.ToolRepair {
	return &types.ToolRepair{
		Code: types.ToolRepairCodeEvidenceAbsenceToCompletion,
		Hint: "Absence claims are not line evidence unless they are emitted as scope=negative with negative_query and negative_scope. For whole-answer absence, remove these emit_evidence rows and carry the conclusion through emit_investigation_complete(result_kind=\"absence\", absence_justification=...).",
		Fields: []string{
			"emit_evidence.items[].scope",
			"emit_evidence.items[].evidence_kind",
			"emit_evidence.items[].negative_query",
			"emit_evidence.items[].negative_scope",
			"emit_investigation_complete.result_kind",
			"emit_investigation_complete.absence_justification",
		},
		Metadata: map[string]string{
			"repair_status": types.ToolRepairStatusActionRequired,
			"lane":          "completion_absence",
		},
	}
}

func repairEmitEvidenceItemShape(in *emitEvidenceItem, index int, compatRepairs *[]string) {
	if in == nil {
		return
	}
	kindText := strings.TrimSpace(in.EvidenceKind)
	kindField := "evidence_kind"
	if kindText == "" {
		kindText = strings.TrimSpace(in.LegacyKind)
		kindField = "kind"
	}
	if anchorKind, ok := emitAnchorKinds[strings.ToLower(kindText)]; ok {
		if strings.TrimSpace(in.AnchorKind) == "" {
			in.AnchorKind = string(anchorKind)
		}
		mapped := evidenceKindForAnchorShape(anchorKind, *in)
		if strings.TrimSpace(in.EvidenceKind) != "" {
			in.EvidenceKind = string(mapped)
		} else {
			in.LegacyKind = string(mapped)
		}
		if compatRepairs != nil {
			*compatRepairs = append(*compatRepairs,
				fmt.Sprintf("items[%d].%s=%q was an anchor_kind; moved semantic kind to %q and kept anchor_kind=%q",
					index, kindField, kindText, mapped, in.AnchorKind))
		}
	}
	repairEvidenceKindValueInAnchorKind(in, index, compatRepairs)
	repairMissingEvidenceKindFromAnchorShape(in, index, compatRepairs)
	repairDirectEvidenceKindFromAnchorShape(in, index, compatRepairs)
	repairMisplacedScopeSemanticValue(in, index, compatRepairs)
	repairLineRangeScopeMissingEnd(in, index, compatRepairs)

	scope := types.EvidenceScope(strings.ToLower(strings.TrimSpace(in.Scope)))
	if scope == types.ScopeFile && in.LineStart.Int() > 0 && emitEvidenceItemHasLineAnchorShape(*in) {
		oldScope := in.Scope
		if in.LineEnd.Int() > in.LineStart.Int() {
			in.Scope = string(types.ScopeLineRange)
		} else {
			in.Scope = string(types.ScopeLine)
		}
		if compatRepairs != nil {
			*compatRepairs = append(*compatRepairs,
				fmt.Sprintf("items[%d].scope=%q carried line anchor fields at line_start=%d; treated as scope=%q",
					index, oldScope, in.LineStart.Int(), in.Scope))
		}
	}
}

func repairMissingEvidenceKindFromAnchorShape(in *emitEvidenceItem, index int, compatRepairs *[]string) {
	if in == nil ||
		strings.TrimSpace(in.EvidenceKind) != "" ||
		strings.TrimSpace(in.LegacyKind) != "" ||
		!emitEvidenceItemHasLineCoordinateShape(*in) {
		return
	}
	anchorKind, ok := findAnchorKind(strings.ToLower(strings.TrimSpace(in.AnchorKind)))
	if !ok {
		return
	}
	mapped := evidenceKindForAnchorShape(anchorKind, *in)
	if mapped == "" || mapped == types.EvidenceAbsent {
		return
	}
	in.EvidenceKind = string(mapped)
	if compatRepairs != nil {
		*compatRepairs = append(*compatRepairs,
			fmt.Sprintf("items[%d].evidence_kind was missing; inferred %q from anchor_kind=%q and typed fields",
				index, mapped, anchorKind))
	}
}

func repairDirectEvidenceKindFromAnchorShape(in *emitEvidenceItem, index int, compatRepairs *[]string) {
	if in == nil || !emitEvidenceItemHasLineCoordinateShape(*in) {
		return
	}
	kindText := strings.TrimSpace(in.EvidenceKind)
	kindField := "evidence_kind"
	if kindText == "" {
		kindText = strings.TrimSpace(in.LegacyKind)
		kindField = "kind"
	}
	if !strings.EqualFold(kindText, string(types.EvidenceDirect)) {
		return
	}
	anchorKind, ok := findAnchorKind(strings.ToLower(strings.TrimSpace(in.AnchorKind)))
	if !ok {
		return
	}
	mapped := evidenceKindForAnchorShape(anchorKind, *in)
	if mapped == "" || mapped == types.EvidenceAbsent || mapped == types.EvidenceDirect {
		return
	}
	if strings.TrimSpace(in.EvidenceKind) != "" || strings.TrimSpace(in.LegacyKind) == "" {
		in.EvidenceKind = string(mapped)
		kindField = "evidence_kind"
	} else {
		in.LegacyKind = string(mapped)
	}
	if compatRepairs != nil {
		*compatRepairs = append(*compatRepairs,
			fmt.Sprintf("items[%d].%s=%q conflicted with anchor_kind=%q; treated as %q from typed anchor shape",
				index, kindField, kindText, anchorKind, mapped))
	}
}

func repairLineRangeScopeMissingEnd(in *emitEvidenceItem, index int, compatRepairs *[]string) {
	if in == nil ||
		!strings.EqualFold(strings.TrimSpace(in.Scope), string(types.ScopeLineRange)) ||
		!emitEvidenceItemHasLineCoordinateShape(*in) ||
		in.LineEnd.Int() > in.LineStart.Int() {
		return
	}
	in.Scope = string(types.ScopeLine)
	if compatRepairs != nil {
		*compatRepairs = append(*compatRepairs,
			fmt.Sprintf("items[%d].scope=line_range lacked a valid line_end; treated as scope=line at line_start=%d",
				index, in.LineStart.Int()))
	}
}

func repairMissingEmitEvidenceSource(in *emitEvidenceItem, index int, gc *ground.Context, compatRepairs *[]string) {
	if in == nil ||
		strings.TrimSpace(in.Source) != "" ||
		in.LineStart.Int() <= 0 ||
		gc == nil {
		return
	}
	if source := uniqueString(pathSlotSourceCandidates(*in, gc)); source != "" {
		in.Source = source
		if compatRepairs != nil {
			slot := "subject/object"
			if strings.TrimSpace(in.AnchorSymbol) != "" {
				slot += " plus anchor_symbol"
			}
			*compatRepairs = append(*compatRepairs,
				fmt.Sprintf("items[%d].source was missing; inferred %q from path-valued %s at exact read_file line %d",
					index, source, slot, in.LineStart.Int()))
		}
		return
	}
	if strings.TrimSpace(in.AnchorSymbol) == "" {
		return
	}
	var candidates []string
	for source := range gc.LineIndex {
		if _, ok := ground.VerifyLineAnchor(gc, source, in.LineStart.Int(), in.AnchorSymbol, 0); ok {
			candidates = append(candidates, source)
		}
	}
	if source := uniqueString(candidates); source != "" {
		in.Source = source
		if compatRepairs != nil {
			*compatRepairs = append(*compatRepairs,
				fmt.Sprintf("items[%d].source was missing; inferred %q from exact read_file line %d and anchor_symbol=%q",
					index, source, in.LineStart.Int(), strings.TrimSpace(in.AnchorSymbol)))
		}
	}
}

func pathSlotSourceCandidates(in emitEvidenceItem, gc *ground.Context) []string {
	if gc == nil || in.LineStart.Int() <= 0 {
		return nil
	}
	var out []string
	for _, raw := range []string{in.Subject, in.Object} {
		candidate := canonicalEvidencePathSlot(raw, gc)
		if candidate == "" {
			continue
		}
		if strings.TrimSpace(in.AnchorSymbol) != "" {
			if _, ok := ground.VerifyLineAnchor(gc, candidate, in.LineStart.Int(), in.AnchorSymbol, 0); ok {
				out = append(out, candidate)
			}
			continue
		}
		if fileLines, ok := gc.LineIndex[candidate]; ok {
			if _, ok := fileLines[in.LineStart.Int()]; ok {
				out = append(out, candidate)
			}
		}
	}
	return out
}

func canonicalEvidencePathSlot(raw string, gc *ground.Context) string {
	raw = strings.Trim(strings.TrimSpace(raw), "`\"'")
	if raw == "" || !emitLooksLikePath(raw) {
		return ""
	}
	candidate := ground.CanonicalRepoRelative(raw, "")
	if gc != nil && strings.TrimSpace(gc.RepoRoot) != "" {
		candidate = ground.CanonicalRepoRelative(raw, gc.RepoRoot)
	}
	candidate = strings.Trim(strings.TrimSpace(strings.ReplaceAll(candidate, `\`, `/`)), "/")
	if candidate == "" || candidate == "." {
		return ""
	}
	if gc == nil {
		return ""
	}
	if _, ok := gc.LineIndex[candidate]; ok {
		return candidate
	}
	return ""
}

func repairMissingEmitEvidenceAnchorSymbol(in *emitEvidenceItem, index int, gc *ground.Context, compatRepairs *[]string) {
	if in == nil ||
		strings.TrimSpace(in.AnchorSymbol) != "" ||
		strings.TrimSpace(in.AnchorKind) == "" ||
		!emitEvidenceItemHasLineCoordinateShape(*in) {
		return
	}
	anchorKind, ok := findAnchorKind(strings.ToLower(strings.TrimSpace(in.AnchorKind)))
	if !ok {
		return
	}
	if candidate, source := inferEmitEvidenceAnchorSymbol(*in, anchorKind, gc); candidate != "" {
		in.AnchorSymbol = candidate
		if compatRepairs != nil {
			*compatRepairs = append(*compatRepairs,
				fmt.Sprintf("items[%d].anchor_symbol was missing; inferred %q from %s at %s:%d",
					index, candidate, source, strings.TrimSpace(in.Source), in.LineStart.Int()))
		}
	}
}

func inferEmitEvidenceAnchorSymbol(in emitEvidenceItem, anchorKind types.AnchorKind, gc *ground.Context) (string, string) {
	if candidate := uniqueString(emitEvidenceAnchorCandidatesFromGraph(in, anchorKind, gc)); candidate != "" {
		return candidate, "repomap exact-line metadata"
	}
	if candidate := uniqueString(emitEvidenceAnchorCandidatesFromTypedFields(in, gc)); candidate != "" {
		return candidate, "typed fields corroborated by the read line"
	}
	return "", ""
}

func emitEvidenceAnchorCandidatesFromGraph(in emitEvidenceItem, anchorKind types.AnchorKind, gc *ground.Context) []string {
	if gc == nil || gc.Graph == nil {
		return nil
	}
	fi := emitEvidenceGraphFileInfo(gc, in.Source)
	if fi == nil {
		return nil
	}
	line := in.LineStart.Int()
	if line <= 0 {
		return nil
	}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	switch anchorKind {
	case types.AnchorDefinition:
		for _, sym := range fi.Symbols {
			if sym.Line == line {
				add(sym.Name)
			}
		}
	case types.AnchorCall:
		for _, rel := range fi.Relations {
			if rel.Line != line || rel.Kind != "call" {
				continue
			}
			add(rel.ToEP.Name)
		}
	case types.AnchorImport:
		for _, imp := range fi.Imports {
			if imp.Line != line {
				continue
			}
			add(imp.Alias)
			add(imp.Path)
		}
	}
	return out
}

func emitEvidenceGraphFileInfo(gc *ground.Context, source string) *repomap.FileInfo {
	if gc == nil {
		return nil
	}
	if _, fi, _, _, ok := ground.ResolveSourceGraphFile(gc, source); ok {
		return fi
	}
	if gc.Graph == nil {
		return nil
	}
	candidates := []string{
		strings.TrimSpace(source),
		filepath.ToSlash(strings.TrimSpace(source)),
		strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(source)), "./"),
	}
	if gc.ActiveSetPath != nil {
		if mapped := gc.ActiveSetPath(source); strings.TrimSpace(mapped) != "" {
			candidates = append(candidates, mapped)
		}
	}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		if fi := gc.Graph.FileIndex[candidate]; fi != nil {
			return fi
		}
	}
	return nil
}

func emitEvidenceAnchorCandidatesFromTypedFields(in emitEvidenceItem, gc *ground.Context) []string {
	if gc == nil || !ground.HasLineInIndex(gc, in.Source, in.LineStart.Int()) {
		return nil
	}
	var fields []string
	fields = append(fields,
		in.Object,
		in.Subject,
		in.Condition,
		in.Snippet,
	)
	fields = append(fields, in.SurfaceTerms...)
	fields = append(fields, in.Summary)
	var out []string
	for _, field := range fields {
		for _, token := range emitEvidenceIdentifierCandidates(field) {
			if _, ok := ground.VerifyLineAnchor(gc, in.Source, in.LineStart.Int(), token, 0); ok {
				out = append(out, token)
			}
		}
	}
	return out
}

func emitEvidenceIdentifierCandidates(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	matches := emitEvidenceIdentifierCandidateRe.FindAllString(text, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		match = strings.Trim(match, ".")
		if !emitEvidenceLooksLikeAnchorCandidate(match) {
			continue
		}
		if strings.ContainsAny(match, ".:") {
			tail := emitEvidenceQualifiedIdentifierTail(match)
			if tail != "" && emitEvidenceLooksLikeAnchorCandidate(tail) {
				out = append(out, tail)
			}
			continue
		}
		out = append(out, match)
	}
	return out
}

var emitEvidenceIdentifierCandidateRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*(?:[.:][A-Za-z_][A-Za-z0-9_]*)*`)

func emitEvidenceQualifiedIdentifierTail(raw string) string {
	raw = strings.TrimSpace(strings.Trim(raw, ".:"))
	if raw == "" {
		return ""
	}
	raw = strings.ReplaceAll(raw, "::", ".")
	if idx := strings.LastIndex(raw, "."); idx >= 0 {
		raw = raw[idx+1:]
	}
	return strings.TrimSpace(strings.Trim(raw, ".:"))
}

func emitEvidenceLooksLikeAnchorCandidate(s string) bool {
	if len(s) < 2 {
		return false
	}
	lower := strings.ToLower(s)
	switch lower {
	case "the", "and", "for", "with", "from", "into", "that", "this", "then", "than",
		"true", "false", "nil", "null", "none", "line", "lines", "file", "files",
		"function", "method", "type", "struct", "class", "interface", "returns", "return",
		"calls", "call", "source", "scope", "summary", "direct", "mechanism", "relationship":
		return false
	}
	hasUpper, hasDigit, hasUnderscore, hasQualifier := false, false, false, false
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= '0' && r <= '9':
			hasDigit = true
		case r == '_':
			hasUnderscore = true
		case r == '.' || r == ':':
			hasQualifier = true
		}
	}
	return hasUpper || hasDigit || hasUnderscore || hasQualifier
}

func uniqueString(in []string) string {
	seen := map[string]bool{}
	unique := ""
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		if unique != "" {
			return ""
		}
		unique = s
	}
	return unique
}

func repairMisplacedScopeSemanticValue(in *emitEvidenceItem, index int, compatRepairs *[]string) {
	if in == nil {
		return
	}
	rawScope := strings.TrimSpace(in.Scope)
	if rawScope == "" {
		return
	}
	scopeKey := strings.ToLower(rawScope)
	if types.EvidenceScope(scopeKey).IsValid() {
		return
	}
	if !emitEvidenceItemHasLineCoordinateShape(*in) {
		return
	}

	var mappedKind types.EvidenceKind
	var mappedAnchor types.AnchorKind
	mapped := false
	if kind, ok := emitEvidenceAllowedKinds[scopeKey]; ok && kind != types.EvidenceAbsent {
		mappedKind = kind
		mapped = true
	} else if anchorKind, ok := emitAnchorKinds[scopeKey]; ok {
		mappedAnchor = anchorKind
		mappedKind = evidenceKindForAnchorShape(anchorKind, *in)
		mapped = true
	}
	if !mapped {
		return
	}

	if in.LineEnd.Int() > in.LineStart.Int() {
		in.Scope = string(types.ScopeLineRange)
	} else {
		in.Scope = string(types.ScopeLine)
	}
	if strings.TrimSpace(in.EvidenceKind) == "" && strings.TrimSpace(in.LegacyKind) == "" {
		in.EvidenceKind = string(mappedKind)
	}
	if mappedAnchor != "" && strings.TrimSpace(in.AnchorKind) == "" && strings.TrimSpace(in.AnchorSymbol) != "" {
		in.AnchorKind = string(mappedAnchor)
	}
	if compatRepairs != nil {
		detail := fmt.Sprintf("items[%d].scope=%q was a semantic/location value; treated as scope=%q",
			index, rawScope, in.Scope)
		if mappedAnchor != "" && strings.TrimSpace(in.AnchorKind) == string(mappedAnchor) {
			detail += fmt.Sprintf(" with anchor_kind=%q", mappedAnchor)
		}
		if strings.TrimSpace(in.EvidenceKind) == string(mappedKind) || strings.TrimSpace(in.LegacyKind) == string(mappedKind) {
			detail += fmt.Sprintf(" and evidence_kind=%q", mappedKind)
		}
		*compatRepairs = append(*compatRepairs, detail)
	}
}

func repairEvidenceKindValueInAnchorKind(in *emitEvidenceItem, index int, compatRepairs *[]string) {
	if in == nil {
		return
	}
	anchorKindKey := strings.ToLower(strings.TrimSpace(in.AnchorKind))
	evidenceKind, ok := emitEvidenceAllowedKinds[anchorKindKey]
	if !ok || evidenceKind == types.EvidenceAbsent {
		return
	}
	if !emitEvidenceItemHasGroundableLineShape(*in) {
		return
	}
	existingKind := strings.TrimSpace(in.EvidenceKind)
	existingKindField := "evidence_kind"
	if existingKind == "" {
		existingKind = strings.TrimSpace(in.LegacyKind)
		existingKindField = "kind"
	}
	if existingKind == "" ||
		strings.EqualFold(existingKind, string(types.EvidenceDirect)) ||
		strings.EqualFold(existingKind, string(evidenceKind)) {
		if strings.TrimSpace(in.EvidenceKind) != "" || strings.TrimSpace(in.LegacyKind) == "" {
			in.EvidenceKind = string(evidenceKind)
			existingKindField = "evidence_kind"
		} else {
			in.LegacyKind = string(evidenceKind)
		}
	}
	in.AnchorKind = string(types.AnchorTextReference)
	if compatRepairs != nil {
		*compatRepairs = append(*compatRepairs,
			fmt.Sprintf("items[%d].anchor_kind=%q was an evidence_kind; used anchor_kind=%q and %s=%q",
				index, anchorKindKey, in.AnchorKind, existingKindField, strings.TrimSpace(firstNonEmpty(in.EvidenceKind, in.LegacyKind))))
	}
}

func evidenceKindForAnchorShape(anchorKind types.AnchorKind, in emitEvidenceItem) types.EvidenceKind {
	switch anchorKind {
	case types.AnchorCondition:
		return types.EvidenceConditional
	case types.AnchorCall, types.AnchorCallback, types.AnchorArgument:
		if strings.TrimSpace(in.Object) != "" {
			return types.EvidenceRelationship
		}
		return types.EvidenceMechanism
	default:
		return types.EvidenceDirect
	}
}

func emitEvidenceItemHasGroundableLineShape(in emitEvidenceItem) bool {
	return strings.TrimSpace(in.Source) != "" &&
		in.LineStart.Int() > 0 &&
		strings.TrimSpace(in.AnchorSymbol) != "" &&
		strings.TrimSpace(in.Summary) != ""
}

func emitEvidenceItemHasLineCoordinateShape(in emitEvidenceItem) bool {
	return strings.TrimSpace(in.Source) != "" && in.LineStart.Int() > 0
}

func emitEvidenceItemHasLineAnchorShape(in emitEvidenceItem) bool {
	return strings.TrimSpace(in.AnchorKind) != "" ||
		strings.TrimSpace(in.AnchorSymbol) != "" ||
		strings.TrimSpace(in.Snippet) != ""
}

// buildEmitEvidenceItem validates a single decoded item and converts
// it into a types.EvidenceItem with stable ID and producer stamped.
// All validation is structural, never wordlist-based.
//
// 2026-05+ scope-axis: the input MUST set scope; per-scope required
// fields are validated; the resulting EvidenceItem.ValidateScope
// guarantees ground-layer dispatch can rely on the right bundle.
func buildEmitEvidenceItem(in emitEvidenceItem, index int, workDir string) (types.EvidenceItem, error) {
	scope := types.EvidenceScope(strings.ToLower(strings.TrimSpace(in.Scope)))
	// Empty scope defaults to ScopeLine — covers in-process Go-side
	// producers that build emitEvidenceItem literals without a wire
	// payload. emit_evidence's JSON schema marks scope as REQUIRED
	// for the LLM-facing path, so production LLM emits always carry
	// scope explicitly; this fallback is for tests and direct Go
	// callers, not the LLM channel.
	if scope == "" {
		scope = types.ScopeLine
	}
	if !scope.IsValid() {
		return types.EvidenceItem{}, fmt.Errorf(
			"items[%d]: scope is required and must be one of %v; got %q",
			index, types.AllEvidenceScopes(), in.Scope)
	}
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
		return types.EvidenceItem{}, fmt.Errorf(
			"items[%d]: unknown evidence_kind %q (allowed: %s)",
			index, kindText, strings.Join(emitEvidenceAllowedKindNames(), ", "))
	}
	// Source field — required for every scope EXCEPT crossfile (which
	// uses CrossfileQuery.Files instead). Even ScopeNegative requires
	// a source: the absence is anchored to a specific file.
	source := strings.TrimSpace(in.Source)
	if scope != types.ScopeCrossfile {
		if source == "" {
			return types.EvidenceItem{}, fmt.Errorf("items[%d]: source is required for scope=%s", index, scope)
		}
		if !emitLooksLikePath(source) {
			return types.EvidenceItem{}, fmt.Errorf("items[%d]: source %q does not look like a repo-relative file path", index, in.Source)
		}
		if isInsideWorkDir(source, workDir) {
			return types.EvidenceItem{}, fmt.Errorf("items[%d]: source %q lives inside the per-trace WorkDir (%s) — that is a tool-output blob, not a repo file.", index, in.Source, workDir)
		}
	}

	lineStartN := in.LineStart.Int()
	lineEndN := in.LineEnd.Int()
	var anchorKind types.AnchorKind
	var anchorSymbol string

	switch scope {
	case types.ScopeLine:
		if lineStartN <= 0 {
			return types.EvidenceItem{}, fmt.Errorf("items[%d]: scope=line requires line_start > 0", index)
		}
		if lineEndN > 0 && lineEndN < lineStartN {
			return types.EvidenceItem{}, fmt.Errorf("items[%d]: line_end (%d) is before line_start (%d)", index, lineEndN, lineStartN)
		}
		var err error
		anchorKind, anchorSymbol, err = parseAnchorFields(in, index)
		if err != nil {
			return types.EvidenceItem{}, err
		}

	case types.ScopeLineRange:
		if lineStartN <= 0 || lineEndN <= lineStartN {
			return types.EvidenceItem{}, fmt.Errorf("items[%d]: scope=line_range requires line_start > 0 and line_end > line_start; got %d-%d", index, lineStartN, lineEndN)
		}
		// AnchorKind/Symbol are optional for ranges (the range itself
		// is the anchor); accept whatever the LLM provided.
		if strings.TrimSpace(in.AnchorKind) != "" {
			var err error
			anchorKind, anchorSymbol, err = parseAnchorFields(in, index)
			if err != nil {
				return types.EvidenceItem{}, err
			}
		}

	case types.ScopeSection:
		if strings.TrimSpace(in.SectionPath) == "" {
			return types.EvidenceItem{}, fmt.Errorf("items[%d]: scope=section requires section_path", index)
		}
		// LineStart/LineEnd are optional for section (the parsed
		// section tells the grounder its own line range).

	case types.ScopeFile:
		if lineStartN != 0 {
			return types.EvidenceItem{}, fmt.Errorf("items[%d]: scope=file must have line_start=0; got %d (file-identity anchor — no specific line)", index, lineStartN)
		}
		role := types.FileRoleLabel(strings.ToLower(strings.TrimSpace(in.FileRoleLabel)))
		if !role.IsValid() {
			return types.EvidenceItem{}, fmt.Errorf("items[%d]: scope=file requires file_role_label one of %v; got %q", index, types.AllFileRoleLabels(), in.FileRoleLabel)
		}

	case types.ScopeCrossfile:
		if in.CrossfileQuery == nil || len(in.CrossfileQuery.Files) == 0 {
			return types.EvidenceItem{}, fmt.Errorf("items[%d]: scope=crossfile requires crossfile_query with at least 1 file", index)
		}
		if len(in.CrossfileQuery.Files) > 5 {
			return types.EvidenceItem{}, fmt.Errorf("items[%d]: scope=crossfile crossfile_query.files capped at 5; got %d", index, len(in.CrossfileQuery.Files))
		}
		if strings.TrimSpace(in.CrossfileQuery.Pattern) == "" {
			return types.EvidenceItem{}, fmt.Errorf("items[%d]: scope=crossfile requires crossfile_query.pattern", index)
		}
		if in.CrossfileAssertion == nil || strings.TrimSpace(in.CrossfileAssertion.Kind) == "" {
			return types.EvidenceItem{}, fmt.Errorf("items[%d]: scope=crossfile requires crossfile_assertion with kind", index)
		}

	case types.ScopeNegative:
		if kind != types.EvidenceAbsent {
			return types.EvidenceItem{}, fmt.Errorf("items[%d]: scope=negative requires evidence_kind=absent; got %q", index, kind)
		}
		if in.NegativeQuery == nil || strings.TrimSpace(in.NegativeQuery.File) == "" || strings.TrimSpace(in.NegativeQuery.Pattern) == "" {
			return types.EvidenceItem{}, fmt.Errorf("items[%d]: scope=negative requires negative_query with file and pattern", index)
		}
		nscope := types.NegativeScope(strings.ToLower(strings.TrimSpace(in.NegativeScope)))
		if !nscope.IsValid() {
			return types.EvidenceItem{}, fmt.Errorf("items[%d]: scope=negative requires negative_scope one of %v; got %q", index, types.AllNegativeScopes(), in.NegativeScope)
		}
		if nscope == types.NegativeScopeSection && strings.TrimSpace(in.NegativeQuery.Section) == "" {
			return types.EvidenceItem{}, fmt.Errorf("items[%d]: scope=negative + negative_scope=section requires negative_query.section", index)
		}
	}

	if kind == types.EvidenceRelationship && strings.TrimSpace(in.Object) == "" {
		return types.EvidenceItem{}, fmt.Errorf("items[%d]: relationship items require object", index)
	}
	if kind == types.EvidenceRegistration &&
		(strings.TrimSpace(in.Subject) == "" || strings.TrimSpace(in.Object) == "") {
		return types.EvidenceItem{}, fmt.Errorf(
			"items[%d]: registration items require both subject and object on the actual binding expression; cite the line that visibly binds the slot/receiver to the target, not the surrounding module, factory, or container definition. For a source form registry.add(wrapper(target)), use anchor_kind=call, anchor_symbol=add, subject=registry, and object=wrapper(target). If the source instead selects a factory result, emit its branch guard as conditional/condition and its concrete return as direct/return",
			index,
		)
	}
	if kind == types.EvidenceConditional {
		if scope != types.ScopeLine || anchorKind != types.AnchorCondition {
			return types.EvidenceItem{}, fmt.Errorf(
				"items[%d]: conditional items require scope=line and anchor_kind=condition on the actual guard; emit any guarded invocation separately as relationship/call",
				index,
			)
		}
		if strings.TrimSpace(in.Condition) == "" {
			return types.EvidenceItem{}, fmt.Errorf(
				"items[%d]: conditional items require the exact non-empty condition from the cited guard line",
				index,
			)
		}
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
	salience, ok := types.ParseEvidenceSalience(in.Salience)
	if !ok {
		return types.EvidenceItem{}, fmt.Errorf("items[%d]: salience=%q is not one of %v", index, in.Salience, types.EvidenceSalienceStrings())
	}
	lineStart := lineStartN
	lineEnd := lineEndN
	if scope == types.ScopeLine && lineEnd == 0 {
		lineEnd = lineStart
	}

	item := types.EvidenceItem{
		Kind:                 kind,
		Subject:              subject,
		Predicate:            predicate,
		Object:               object,
		Summary:              summary,
		Condition:            condition,
		Source:               source,
		LineStart:            lineStart,
		LineEnd:              lineEnd,
		Confidence:           0.78, // matches parseEvidenceLine's confidence floor
		Producer:             EmitEvidenceProducer,
		ContextRole:          contextRole,
		DiagramRole:          diagramRole,
		RequestedDiagramRole: diagramRole,
		AnchorKind:           anchorKind,
		AnchorSymbol:         anchorSymbol,
		Snippet:              snippet,
		Scope:                scope,
		LoadBearingSummary:   in.LoadBearingSummary,
		Salience:             salience,
		SurfaceTerms:         normalizeEvidenceSurfaceTerms(in.SurfaceTerms),
	}

	// Reject load_bearing_summary=true on items whose Summary is empty —
	// an empty summary cannot carry a load-bearing scalar, and accepting
	// the flag would invite future readers to assume a non-empty pathway.
	if item.LoadBearingSummary && strings.TrimSpace(summary) == "" {
		return types.EvidenceItem{}, fmt.Errorf(
			"items[%d]: load_bearing_summary=true requires a non-empty summary "+
				"(set the flag only when the summary text holds a scalar the answer must reproduce verbatim)",
			index)
	}

	// Wire scope-specific bundles.
	switch scope {
	case types.ScopeSection:
		item.SectionPath = strings.TrimSpace(in.SectionPath)
	case types.ScopeFile:
		item.FileRoleLabel = types.FileRoleLabel(strings.ToLower(strings.TrimSpace(in.FileRoleLabel)))
	case types.ScopeCrossfile:
		item.CrossfileQuery = &types.CrossfileQuery{
			Files:   append([]string(nil), in.CrossfileQuery.Files...),
			Pattern: strings.TrimSpace(in.CrossfileQuery.Pattern),
			Context: strings.TrimSpace(in.CrossfileQuery.Context),
		}
		item.CrossfileAssertion = &types.CrossfileAssertion{
			Kind:  types.CrossfileAssertionKind(strings.ToLower(strings.TrimSpace(in.CrossfileAssertion.Kind))),
			Count: in.CrossfileAssertion.Count,
		}
	case types.ScopeNegative:
		item.NegativeQuery = &types.NegativeQuery{
			File:    strings.TrimSpace(in.NegativeQuery.File),
			Pattern: strings.TrimSpace(in.NegativeQuery.Pattern),
			Section: strings.TrimSpace(in.NegativeQuery.Section),
		}
		item.NegativeScope = types.NegativeScope(strings.ToLower(strings.TrimSpace(in.NegativeScope)))
	}

	// Defense-in-depth: validator catches inconsistencies the
	// per-scope branch above might have missed (e.g. invalid
	// CrossfileAssertion.Kind value combinations).
	if err := item.ValidateScope(); err != nil {
		return types.EvidenceItem{}, fmt.Errorf("items[%d]: %v", index, err)
	}

	item.ID = types.StableEvidenceID(item)
	return item, nil
}

// parseAnchorFields validates anchor_kind + anchor_symbol for the
// line-shaped scopes (Line and optionally LineRange).
func parseAnchorFields(in emitEvidenceItem, index int) (types.AnchorKind, string, error) {
	preAnchorKindKey := strings.ToLower(strings.TrimSpace(in.AnchorKind))
	if _, evidenceNameCollision := emitEvidenceAllowedKinds[preAnchorKindKey]; evidenceNameCollision {
		return "", "", fmt.Errorf(
			"items[%d]: anchor_kind %q is invalid because %q belongs to evidence_kind, not anchor_kind. Use evidence_kind=%q and anchor_kind in {%s}.",
			index,
			strings.TrimSpace(in.AnchorKind),
			strings.TrimSpace(in.AnchorKind),
			strings.TrimSpace(in.AnchorKind),
			strings.Join(emitAnchorKindNames(), ", "),
		)
	}
	if preAnchorKindKey == "" {
		return "", "", fmt.Errorf("items[%d]: anchor_kind is required (one of: %s)", index, strings.Join(emitAnchorKindNames(), ", "))
	}
	anchorKind, ok := findAnchorKind(preAnchorKindKey)
	if !ok {
		return "", "", fmt.Errorf("items[%d]: unknown anchor_kind %q (allowed: %s)", index, in.AnchorKind, strings.Join(emitAnchorKindNames(), ", "))
	}
	anchorSymbol := strings.TrimSpace(in.AnchorSymbol)
	if anchorSymbol == "" {
		return "", "", fmt.Errorf("items[%d]: anchor_symbol is required for line-shaped scopes — the identifier the grounder should find at source:line_start", index)
	}
	return anchorKind, anchorSymbol, nil
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
	if ev.AnchorKind != types.AnchorTextReference &&
		evidenceLooksIllustrative(ev, gc) &&
		!configFileSurfaceCanCarryDiagramRole(ev) {
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
	if evidenceLooksIllustrative(ev, gc) && !configFileSurfaceCanCarryDiagramRole(ev) {
		return types.EvidenceDiagramRoleUnknown
	}
	if role := types.ConfigTraceSurfaceDiagramRoleInFiles(contract, ev, requiredFiles); role != types.EvidenceDiagramRoleUnknown {
		switch ev.ContextRole {
		case types.EvidenceContextRoleIllustrativeOnly:
			if role == types.EvidenceDiagramRoleConfig && configFileSurfaceCanCarryDiagramRole(ev) {
				return role
			}
		case types.EvidenceContextRoleAbsenceSupport:
			if role == types.EvidenceDiagramRoleConfig && types.LooksLikeConfigFilePath(ev.Source) {
				return role
			}
		default:
			return role
		}
	}
	return types.EvidenceDiagramRoleUnknown
}

func configFileSurfaceCanCarryDiagramRole(ev types.EvidenceItem) bool {
	return ev.DiagramRole == types.EvidenceDiagramRoleConfig &&
		types.LooksLikeConfigFilePath(ev.Source)
}

func appendDiagramRoleValidationNote(ev *types.EvidenceItem, requested types.EvidenceDiagramRole, contract *types.ExactResolutionContract, requiredFiles []string) bool {
	if ev == nil || requested == types.EvidenceDiagramRoleUnknown || requested == ev.DiagramRole {
		return false
	}
	note := ""
	switch requested {
	case types.EvidenceDiagramRoleConfig:
		if types.LooksLikeAuxiliaryEvidencePath(ev.Source) && !types.LooksLikeConfigFilePath(ev.Source) {
			note = "diagram_role_hint=config was ignored because non-config doc/test/example anchors are never precedence-grade config-file evidence. Keep this item as context only."
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
	if ev.Source == "" {
		return false
	}
	if gc != nil && len(gc.LineIndex) > 0 && ev.LineStart > 0 {
		if fileLines, ok := gc.LineIndex[ev.Source]; ok {
			if ground.LineLooksCommentOnly(fileLines, ev.LineStart, ev.Source) {
				return true
			}
			return evidencePathDefaultsToIllustrativeText(ev.Source)
		}
	}
	return evidencePathDefaultsToIllustrativeText(ev.Source)
}

func evidencePathDefaultsToIllustrativeText(source string) bool {
	switch types.ClassifySourcePathRole(source) {
	case types.SourcePathRoleDocumentation, types.SourcePathRolePromptSupport:
		return true
	default:
		return false
	}
}

func evidenceCanBeDefining(ev types.EvidenceItem) bool {
	if ev.Source == "" {
		return false
	}
	switch ev.AnchorKind {
	case types.AnchorDefinition, types.AnchorAssignment, types.AnchorInitializer, types.AnchorImport, types.AnchorReturn, types.AnchorStringLiteral:
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
		ev.Subject, ev.Predicate, ev.Object, ev.AnchorSymbol, ev.Condition, ev.Snippet)
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
		ev.Subject, ev.Predicate, ev.Object, ev.AnchorSymbol, ev.Condition, ev.Snippet) &&
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

// normalizeCallbackHandoffEvidence repairs the common schema-category error
// where a model marks a callable value passed to an executor/framework as a
// direct call. The rewrite is permitted only when the already-read exact line
// structurally proves the receiving invocation and non-invoked callable, and
// the same line does not also prove a direct call to that callable.
func normalizeCallbackHandoffEvidence(it *types.EvidenceItem, gc *ground.Context) bool {
	if it == nil || gc == nil || (it.AnchorKind != types.AnchorCall && it.AnchorKind != types.AnchorCallback) {
		return false
	}
	target := strings.TrimSpace(firstNonEmpty(it.AnchorSymbol, it.Object))
	if target == "" || ground.LineHasDirectCallToTarget(gc, it.Source, it.LineStart, target) {
		return false
	}
	receiver, callable, ok := ground.DetectCallbackHandoffAtLine(gc, it.Source, it.LineStart, target)
	if !ok {
		return false
	}
	// An explicitly typed callback row with contradictory endpoints stays
	// fail-closed; only fill/canonicalise compatible identities.
	if it.AnchorKind == types.AnchorCallback {
		if strings.TrimSpace(it.Subject) != "" && !emitEndpointIdentityCompatible(it.Subject, receiver) {
			return false
		}
		if strings.TrimSpace(it.Object) != "" && !emitEndpointIdentityCompatible(it.Object, callable) {
			return false
		}
	}
	originalKind := it.AnchorKind
	it.AnchorKind = types.AnchorCallback
	it.Kind = types.EvidenceRelationship
	it.Subject = receiver
	it.Predicate = "passes callback"
	it.Object = callable
	it.AnchorSymbol = callable
	appendGroundingNoteOnce(it, fmt.Sprintf(
		"exact source line classified %s as callback handoff %q -> %q; this proves callable-value transfer, not direct execution",
		originalKind, receiver, callable))
	return true
}

func emitEndpointIdentityCompatible(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}
	return left == right || types.CallChainEndpointCompatible(left, right) || types.CallChainEndpointCompatible(right, left)
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
	if it == nil || gc == nil || it.AnchorKind != types.AnchorCall {
		return false
	}
	predicate := strings.ToLower(strings.TrimSpace(it.Predicate))
	// The decoder uses the evidence kind as a compatibility placeholder when a
	// sparse row omits predicate. Treat that sentinel like an absent predicate;
	// an explicit non-call predicate still stays outside this normalizer.
	stampMissingCallPredicate := (predicate == "" || predicate == string(types.EvidenceRelationship)) &&
		it.Kind == types.EvidenceRelationship
	if !stampMissingCallPredicate && !callLikePredicates[predicate] {
		return false
	}
	caller, callee, ok := exactCallEvidenceDirection(it, gc)
	if !ok {
		return false
	}
	changed := false
	if stampMissingCallPredicate {
		// AnchorCall is already grounded against the exact parser/read-line
		// relation below. A sparse relationship row therefore has enough typed
		// authority to carry the canonical predicate; leaving it blank discards
		// an otherwise exact caller -> callee edge from downstream graphs.
		it.Predicate = "calls"
		changed = true
	}
	if strings.TrimSpace(it.Subject) != caller {
		it.Subject = caller
		changed = true
	}
	if strings.TrimSpace(it.Object) != callee {
		it.Object = callee
		changed = true
	}
	// AnchorCall's explicit anchor is the callee identity. This is not a
	// guess from prose: callee came from the exact parser relation at the
	// cited source:line, falling back to the already-read line expression
	// only when the graph could not resolve the receiver. Keeping the
	// canonical callee in AnchorSymbol lets the immediately-following
	// GroundItem validate the same typed edge downstream gates consume.
	if strings.TrimSpace(it.AnchorSymbol) != callee {
		it.AnchorSymbol = callee
		changed = true
	}
	if changed {
		logging.Debug("[emit_evidence] normalized call direction at %s:%d -> %s %s %s",
			it.Source, it.LineStart, it.Subject, it.Predicate, it.Object)
	}
	return changed
}

// exactCallEvidenceDirection returns the parser/read-line-owned caller and
// callee for the cited call expression without consulting the model's
// predicate. normalizeCallEvidenceDirection uses it only for call-like rows;
// the evidence audit also uses it to explain how a semantic/mismatched call
// row can be re-emitted after fail-closed downgrading. Returning the tuple is
// guidance, not authority: the caller must still explicitly emit the exact
// relationship row before downstream relation consumers can use it.
func exactCallEvidenceDirection(it *types.EvidenceItem, gc *ground.Context) (string, string, bool) {
	if it == nil || gc == nil || it.AnchorKind != types.AnchorCall || it.LineStart <= 0 {
		return "", "", false
	}
	graph, fi, _, source, ok := ground.ResolveSourceGraphFile(gc, it.Source)
	if !ok {
		return "", "", false
	}
	candidates := emitPreferredCallTargetNames(it)
	caller := enclosingCallableSymbolName(fi, it.LineStart)
	callee := ""
	// Prefer a graph-resolved semantic target (for example a Java field
	// receiver `service.schedule` resolved to `VisitService.schedule`) over the
	// byte-exact source expression. The source expression remains the fallback
	// when the index cannot resolve a unique target; unresolved dynamic calls
	// therefore keep their original bounded identity instead of being guessed.
	if rel, found := findCallRelationAtLineForCandidates(fi, it.LineStart, candidates); found {
		if target := graph.ResolveCallTarget(fi, *rel); target != nil {
			callee = qualifiedEvidenceSymbolName(target)
		}
	}
	if callee == "" {
		if exact, found := sourceLineCallTargetForCandidates(gc, source, it.LineStart, candidates); found {
			callee = exact
		} else if rel, found := findCallRelationAtLineForCandidates(fi, it.LineStart, candidates); found {
			callee = callRelationTargetName(graph, fi, rel)
		}
	}
	if caller == "" || callee == "" {
		return "", "", false
	}
	return caller, callee, true
}

// exactUniqueCallEvidenceDirectionAtLine resolves a call tuple without using
// the submitted anchor kind or prose endpoints.  It is intentionally stricter
// than findCallRelationAtLine's historical first-match behavior: a line with
// two parser-owned calls has no unique operation and cannot drive a hard
// companion repair.
func exactUniqueCallEvidenceDirectionAtLine(it types.EvidenceItem, gc *ground.Context) (string, string, bool) {
	if gc == nil || it.LineStart <= 0 {
		return "", "", false
	}
	_, fi, _, _, ok := ground.ResolveSourceGraphFile(gc, it.Source)
	if !ok || fi == nil {
		return "", "", false
	}
	var only *repomap.Relation
	for i := range fi.Relations {
		rel := &fi.Relations[i]
		if rel.Kind != "call" || rel.Line != it.LineStart {
			continue
		}
		if only != nil {
			return "", "", false
		}
		only = rel
	}
	if only == nil || strings.TrimSpace(only.ToEP.Name) == "" {
		return "", "", false
	}
	call := it
	call.AnchorKind = types.AnchorCall
	call.AnchorSymbol = strings.TrimSpace(only.ToEP.Name)
	call.Subject = ""
	call.Predicate = "calls"
	call.Object = call.AnchorSymbol
	return exactCallEvidenceDirection(&call, gc)
}

// realignExplicitCallEvidenceLine repairs a bounded line-number error only
// when the model already supplied a complete typed caller -> callee pair and
// an already-read same-file source line plus repomap relation prove that exact
// direction.  This is stricter than nearest_call: callee proximity alone is
// insufficient, so a recursive call cannot steal a wrapper -> helper edge.
//
// The function reads no user request, evidence summary, final-answer prose, or
// case identity.  It is therefore safe to run before the hard grounding gate
// across every supported source language.
func realignExplicitCallEvidenceLine(it *types.EvidenceItem, gc *ground.Context) bool {
	if it == nil || gc == nil || it.AnchorKind != types.AnchorCall || it.LineStart <= 0 {
		return false
	}
	predicate := strings.ToLower(strings.TrimSpace(it.Predicate))
	if !callLikePredicates[predicate] {
		return false
	}
	caller := strings.TrimSpace(it.Subject)
	callee := strings.TrimSpace(it.Object)
	if caller == "" || callee == "" {
		return false
	}
	_, fi, _, source, ok := ground.ResolveSourceGraphFile(gc, it.Source)
	if !ok {
		return false
	}
	if explicitDirectedCallMatchesLine(gc, fi, source, it.LineStart, caller, callee) {
		return false
	}

	const maxDistance = 40
	bestLine, bestDistance := 0, maxDistance+1
	for _, rel := range fi.Relations {
		if rel.Kind != "call" || rel.Line <= 0 {
			continue
		}
		distance := rel.Line - it.LineStart
		if distance < 0 {
			distance = -distance
		}
		if distance > maxDistance || distance > bestDistance {
			continue
		}
		if !qualifiedCallEndpointEqual(rel.ToEP.Name, callee) ||
			!qualifiedCallEndpointEqual(enclosingCallableSymbolName(fi, rel.Line), caller) {
			continue
		}
		if _, sourceOK := sourceLineCallTargetForCandidates(gc, source, rel.Line, []string{callee}); !sourceOK {
			continue
		}
		if distance < bestDistance || bestLine == 0 || rel.Line < bestLine {
			bestLine, bestDistance = rel.Line, distance
		}
	}
	if bestLine <= 0 || bestLine == it.LineStart {
		return false
	}
	originalLine := it.LineStart
	it.LineStart = bestLine
	if it.Scope == types.ScopeLine || it.LineEnd == originalLine {
		it.LineEnd = bestLine
	}
	appendGroundingNoteOnce(it, fmt.Sprintf(
		"explicit typed call edge %q -> %q was realigned from line %d to exact same-file callsite line %d before grounding; both caller and callee plus the already-read source line agreed",
		caller, callee, originalLine, bestLine))
	return true
}

func explicitDirectedCallMatchesLine(gc *ground.Context, fi *repomap.FileInfo, source string, line int, caller, callee string) bool {
	if gc == nil || fi == nil || line <= 0 {
		return false
	}
	actualCaller := enclosingCallableSymbolName(fi, line)
	if !qualifiedCallEndpointEqual(actualCaller, caller) {
		return false
	}
	rel, ok := findCallRelationAtLineForCandidates(fi, line, []string{callee})
	if !ok || !qualifiedCallEndpointEqual(rel.ToEP.Name, callee) {
		return false
	}
	_, sourceOK := sourceLineCallTargetForCandidates(gc, source, line, []string{callee})
	return sourceOK
}

func qualifiedCallEndpointEqual(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	normalize := func(s string) string {
		s = strings.ReplaceAll(s, "->", ".")
		s = strings.ReplaceAll(s, "::", ".")
		return strings.Trim(s, ".")
	}
	na, nb := normalize(a), normalize(b)
	if na == nb {
		return true
	}
	aQualified := strings.Contains(na, ".")
	bQualified := strings.Contains(nb, ".")
	if aQualified && bQualified {
		return false
	}
	return emitLastDotSegment(na) == emitLastDotSegment(nb)
}

func stabilizeStringLiteralIdentifierAnchor(it *types.EvidenceItem, gc *ground.Context) bool {
	if it == nil || gc == nil || it.Scope != types.ScopeLine || it.AnchorKind != types.AnchorStringLiteral {
		return false
	}
	if it.GroundingStatus == types.GroundingGrounded || strings.TrimSpace(it.AnchorSymbol) == "" {
		return false
	}
	if _, ok := ground.VerifyLineAnchor(gc, it.Source, it.LineStart, it.AnchorSymbol, 0); !ok {
		return false
	}
	if lineLooksIdentifierBindingAtAnchor(gc, it.Source, it.LineStart, it.AnchorSymbol) {
		it.AnchorKind = types.AnchorDefinition
		it.GroundingStatus = ""
		it.GroundingTier = ""
		it.GroundingNote = ""
		return true
	}
	if !lineLooksIdentifierReferenceAtAnchor(gc, it.Source, it.LineStart, it.AnchorSymbol) {
		return false
	}
	it.AnchorKind = types.AnchorTextReference
	it.GroundingStatus = ""
	it.GroundingTier = ""
	it.GroundingNote = ""
	return true
}

func normalizeRegistrationInitializerAnchor(it *types.EvidenceItem, gc *ground.Context) string {
	if it == nil || gc == nil || it.Scope != types.ScopeLine || it.Kind != types.EvidenceRegistration {
		return ""
	}
	if it.AnchorKind != types.AnchorTextReference && it.AnchorKind != types.AnchorAssignment {
		return ""
	}
	line := evidenceVisibleLineText(gc, it.Source, it.LineStart)
	if !lineLooksLikeInitializerRegistration(line, *it) {
		return ""
	}
	it.AnchorKind = types.AnchorInitializer
	it.GroundingStatus = ""
	it.GroundingTier = ""
	it.GroundingNote = ""
	return "semantic registration evidence was treated as an initializer anchor because the already-read source line visibly assigns or initializes the registered member/value"
}

func evidenceVisibleLineText(gc *ground.Context, source string, line int) string {
	if gc == nil || line <= 0 || source == "" {
		return ""
	}
	candidates := []string{source}
	if canonical := ground.CanonicalContextPath(gc, source); canonical != "" && canonical != source {
		candidates = append(candidates, canonical)
	}
	for _, candidate := range candidates {
		if fileLines := gc.LineIndex[candidate]; len(fileLines) > 0 {
			if text := strings.TrimSpace(fileLines[line]); text != "" {
				return text
			}
		}
	}
	return ""
}

func lineLooksLikeInitializerRegistration(line string, it types.EvidenceItem) bool {
	line = strings.TrimSpace(line)
	if line == "" || !lineHasInitializerAssignmentSyntax(line) {
		return false
	}
	for _, term := range []string{it.AnchorSymbol, it.Object, it.Subject} {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		if strings.Contains(line, term) {
			return true
		}
	}
	return false
}

func lineHasInitializerAssignmentSyntax(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	if strings.Contains(line, " = ") || strings.Contains(line, "\t=\t") ||
		strings.Contains(line, "\t= ") || strings.Contains(line, " =\t") {
		return true
	}
	if strings.HasPrefix(line, ".") && strings.Contains(line, "=") {
		return true
	}
	return strings.Contains(line, ":")
}

func lineLooksIdentifierBindingAtAnchor(gc *ground.Context, source string, line int, anchor string) bool {
	if gc == nil || source == "" || line <= 0 || strings.TrimSpace(anchor) == "" {
		return false
	}
	source = ground.CanonicalContextPath(gc, source)
	text := strings.TrimSpace(gc.LineIndex[source][line])
	if text == "" {
		return false
	}
	idx := indexIdentifierToken(text, anchor)
	if idx < 0 {
		return false
	}
	before := strings.TrimSpace(text[:idx])
	after := strings.TrimSpace(text[idx+len(anchor):])
	if after == "" {
		return false
	}
	if strings.HasPrefix(after, "(") || strings.HasPrefix(after, ".") || strings.HasPrefix(after, "->") || strings.HasPrefix(after, "::") {
		return false
	}
	if hasDeclarationPrefix(before) {
		return true
	}
	if eq := strings.Index(after, "="); eq >= 0 {
		openParen := strings.Index(after, "(")
		return openParen < 0 || eq < openParen
	}
	return false
}

func lineLooksIdentifierReferenceAtAnchor(gc *ground.Context, source string, line int, anchor string) bool {
	if gc == nil || source == "" || line <= 0 || strings.TrimSpace(anchor) == "" {
		return false
	}
	source = ground.CanonicalContextPath(gc, source)
	text := strings.TrimSpace(gc.LineIndex[source][line])
	if text == "" {
		return false
	}
	idx := indexIdentifierToken(text, anchor)
	if idx < 0 {
		return false
	}
	after := strings.TrimSpace(text[idx+len(anchor):])
	// Calls have their own anchor kind and direction normalisation path.
	// Do not silently launder a malformed string-literal call target into a
	// broad text reference here.
	return !strings.HasPrefix(after, "(")
}

func indexIdentifierToken(text, anchor string) int {
	if text == "" || anchor == "" {
		return -1
	}
	for start := 0; start < len(text); {
		idx := strings.Index(text[start:], anchor)
		if idx < 0 {
			return -1
		}
		idx += start
		end := idx + len(anchor)
		if (idx == 0 || !isIdentifierByte(text[idx-1])) && (end == len(text) || !isIdentifierByte(text[end])) {
			return idx
		}
		start = end
	}
	return -1
}

func hasDeclarationPrefix(before string) bool {
	if before == "" {
		return false
	}
	fields := strings.Fields(before)
	if len(fields) == 0 {
		return false
	}
	switch fields[len(fields)-1] {
	case "type", "func", "class", "interface", "enum", "struct", "const", "var", "let", "def", "function", "trait", "namespace":
		return true
	default:
		return false
	}
}

func isIdentifierByte(b byte) bool {
	return (b >= 'A' && b <= 'Z') ||
		(b >= 'a' && b <= 'z') ||
		(b >= '0' && b <= '9') ||
		b == '_' ||
		b == '$'
}

func emitPreferredCallTargetNames(it *types.EvidenceItem) []string {
	if it == nil {
		return nil
	}
	seen := make(map[string]bool, 3)
	out := make([]string, 0, 3)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	// Object is the wire contract's callee lane. Prefer it over a malformed
	// caller-shaped anchor, then fall back to the explicit anchor and subject
	// so inverted legacy rows can still be repaired by an exact same-line
	// parser relation.
	add(it.Object)
	add(it.AnchorSymbol)
	add(it.Subject)
	return out
}

func sourceLineCallTargetForCandidates(gc *ground.Context, source string, line int, candidates []string) (string, bool) {
	if gc == nil || source == "" || line <= 0 || len(candidates) == 0 {
		return "", false
	}
	lines := gc.LineIndex[source]
	if len(lines) == 0 {
		return "", false
	}
	text := strings.TrimSpace(lines[line])
	if text == "" {
		return "", false
	}
	for _, target := range sourceLineCallTargets(text) {
		for _, candidate := range candidates {
			if callExpressionTargetMatchesCandidate(target, candidate) {
				return target, true
			}
		}
	}
	return "", false
}

func sourceLineCallTargets(line string) []string {
	var out []string
	seen := make(map[string]bool)
	for i := 0; i < len(line); i++ {
		if line[i] != '(' {
			continue
		}
		j := i - 1
		for j >= 0 && (line[j] == ' ' || line[j] == '\t') {
			j--
		}
		end := j + 1
		for j >= 0 && isCallTargetByte(line[j]) {
			j--
		}
		start := j + 1
		if start >= end {
			continue
		}
		target := cleanCallExpressionTarget(line[start:end])
		if target == "" || seen[target] {
			continue
		}
		seen[target] = true
		out = append(out, target)
	}
	return out
}

func isCallTargetByte(b byte) bool {
	return (b >= 'A' && b <= 'Z') ||
		(b >= 'a' && b <= 'z') ||
		(b >= '0' && b <= '9') ||
		b == '_' ||
		b == '$' ||
		b == '.' ||
		b == ':' ||
		b == '-' ||
		b == '>' ||
		b == '<' ||
		b == ']' ||
		b == '['
}

func cleanCallExpressionTarget(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimLeft(raw, ".:")
	raw = strings.TrimPrefix(raw, "->")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return stripTrailingGenericArgs(raw)
}

func stripTrailingGenericArgs(raw string) string {
	raw = strings.TrimSpace(raw)
	for {
		if !strings.HasSuffix(raw, "]") && !strings.HasSuffix(raw, ">") {
			return raw
		}
		open, close := byte(0), byte(0)
		switch {
		case strings.HasSuffix(raw, "]"):
			open, close = '[', ']'
		case strings.HasSuffix(raw, ">"):
			open, close = '<', '>'
		}
		depth := 0
		for i := len(raw) - 1; i >= 0; i-- {
			switch raw[i] {
			case close:
				depth++
			case open:
				depth--
				if depth == 0 {
					if i == 0 {
						return raw
					}
					raw = strings.TrimSpace(raw[:i])
					goto next
				}
			}
		}
		return raw
	next:
	}
}

func callExpressionTargetMatchesCandidate(target, candidate string) bool {
	target = strings.TrimSpace(target)
	candidate = strings.TrimSpace(candidate)
	if target == "" || candidate == "" {
		return false
	}
	if target == candidate {
		return true
	}
	normalizedTarget := normalizeCallExpressionTarget(target)
	normalizedCandidate := normalizeCallExpressionTarget(candidate)
	if normalizedTarget == normalizedCandidate {
		return true
	}
	targetTail := types.NormalizedSurfaceSymbolTail(normalizedTarget)
	candidateTail := types.NormalizedSurfaceSymbolTail(normalizedCandidate)
	return targetTail != "" && targetTail == candidateTail
}

func normalizeCallExpressionTarget(raw string) string {
	raw = stripTrailingGenericArgs(strings.TrimSpace(raw))
	raw = strings.ReplaceAll(raw, "->", ".")
	raw = strings.ReplaceAll(raw, "::", ".")
	for strings.Contains(raw, "..") {
		raw = strings.ReplaceAll(raw, "..", ".")
	}
	return strings.Trim(raw, ".")
}

func stampEvidenceOwnerSymbol(it *types.EvidenceItem, gc *ground.Context) bool {
	owner := enclosingEvidenceCallableOwner(it, gc)
	// Call-site ownership is consumed later as a typed endpoint-identity
	// binding. Preserve the parser's package/receiver-qualified callable when
	// available; unlike Subject, OwnerSymbol is system-stamped after grounding
	// and cannot be supplied by the model. Other line-local evidence keeps the
	// historical short owner because it is used for intra-file mismatch checks.
	if it != nil && it.AnchorKind == types.AnchorCall {
		if qualified := enclosingEvidenceQualifiedCallableOwner(it, gc); qualified != "" {
			owner = qualified
		}
	}
	if owner == "" || strings.TrimSpace(it.OwnerSymbol) == owner {
		return false
	}
	it.OwnerSymbol = owner
	return true
}

func stabilizeLineLocalCallableOwner(it *types.EvidenceItem, gc *ground.Context) bool {
	if it == nil {
		return false
	}
	switch it.AnchorKind {
	case types.AnchorCondition, types.AnchorReturn, types.AnchorAssignment, types.AnchorInitializer:
	default:
		return false
	}
	if gc == nil {
		return false
	}
	_, fi, _, _, ok := ground.ResolveSourceGraphFile(gc, it.Source)
	if !ok {
		return false
	}
	owner := enclosingEvidenceCallableOwner(it, gc)
	if owner == "" {
		return false
	}
	changed := false
	if strings.TrimSpace(it.OwnerSymbol) != owner {
		it.OwnerSymbol = owner
		changed = true
	}
	if conflict := conflictingLineLocalCallableClaim(*it, fi, owner); conflict != "" {
		it.Kind = types.EvidenceUnresolved
		it.Confidence = 0
		it.GroundingStatus = types.GroundingUngrounded
		it.GroundingTier = ""
		if appendGroundingNoteOnce(it, fmt.Sprintf(
			"this %s anchor lives inside `%s`, but the item attributes it to `%s`. Use the enclosing callable as the owner for this line-local statement, or drop the speculative cross-function claim. Do NOT repair this item.",
			it.AnchorKind,
			owner,
			conflict,
		)) {
			changed = true
		}
		changed = true
	}
	return changed
}

func stabilizeStatementLocalAnchorClaim(it *types.EvidenceItem, gc *ground.Context) bool {
	if it == nil || gc == nil {
		return false
	}
	switch it.AnchorKind {
	case types.AnchorCondition:
		return stabilizeConditionAnchorClaim(it, gc)
	default:
		return false
	}
}

// stabilizeAssignmentEndpointAuthority prevents a lexically grounded
// assignment from laundering unrelated model-authored endpoints into a
// directed relation. Initializers may contain nested named members on one
// source line; those remain local facts and the downstream exact endpoint
// matcher independently withholds relation authority when their tuple is not
// unambiguous. The parser never widens beyond the observed line.
func stabilizeAssignmentEndpointAuthority(it *types.EvidenceItem) string {
	if it == nil || it.Scope != types.ScopeLine ||
		it.AnchorKind != types.AnchorAssignment ||
		it.Kind == types.EvidenceRegistration ||
		strings.TrimSpace(it.Subject) == "" || strings.TrimSpace(it.Object) == "" ||
		(it.GroundingStatus != types.GroundingGrounded && it.GroundingStatus != types.GroundingRecovered) {
		return ""
	}
	receiver, value, parsed := types.AssignmentEvidenceEndpoints(*it)
	if !parsed || types.AssignmentEvidenceEndpointsMatch(*it) {
		return ""
	}
	originalAnchor := it.AnchorKind
	it.AnchorKind = types.AnchorTextReference
	it.Subject = ""
	it.Predicate = ""
	it.Object = ""
	it.OwnerSymbol = ""
	return fmt.Sprintf(
		"anchor_kind=%s relation authority was removed: the exact grounded line owns receiver=%q and value=%q; re-emit subject/object with those endpoints only if this directed transfer is load-bearing",
		originalAnchor, receiver, value,
	)
}

func stabilizeCallAnchorRelationEndpointAuthority(it *types.EvidenceItem, gc *ground.Context) bool {
	if it == nil || gc == nil || it.Producer != EmitEvidenceProducer ||
		it.Scope != types.ScopeLine || it.AnchorKind != types.AnchorCall ||
		strings.TrimSpace(it.Object) == "" {
		return false
	}
	if it.GroundingStatus != types.GroundingGrounded && it.GroundingStatus != types.GroundingRecovered {
		return false
	}

	// Exact call predicates have already gone through
	// normalizeCallEvidenceDirection, which derives both endpoints from the
	// parser relation/enclosing callable at this exact line.  Registration is a
	// distinct claim form even if its predicate happens to say "calls", so it
	// still needs the registered endpoint on the cited source surface.
	predicate := strings.ToLower(strings.TrimSpace(it.Predicate))
	if it.Kind != types.EvidenceRegistration && callLikePredicates[predicate] {
		return false
	}
	line := evidenceVisibleLineText(gc, it.Source, it.LineStart)
	if evidenceEndpointVisibleOnSourceLine(line, it.Object) {
		return false
	}

	originalKind := it.Kind
	it.Kind = types.EvidenceUnresolved
	it.Confidence = 0
	it.GroundingStatus = types.GroundingUngrounded
	it.GroundingTier = ""
	appendGroundingNoteOnce(it, fmt.Sprintf(
		"the exact %s:%d call anchor proves %q, but relation object %q is not present on that source line and has no parser-owned endpoint authority; cite the source line that contains the object/binding, emit the exact call edge, or drop the speculative relation. Do NOT repair this item by moving it to a wrapper call",
		it.Source, it.LineStart, it.AnchorSymbol, it.Object,
	))
	logging.Debug("[emit_evidence] downgraded %s call-anchor relation with absent object endpoint at %s:%d: %s",
		originalKind, it.Source, it.LineStart, it.Object)
	return true
}

func stabilizeRegistrationEndpointAuthority(it *types.EvidenceItem, gc *ground.Context) bool {
	if it == nil || gc == nil || it.Producer != EmitEvidenceProducer ||
		it.Kind != types.EvidenceRegistration ||
		(it.Scope != types.ScopeLine && it.Scope != types.ScopeLineRange) {
		return false
	}
	if it.GroundingStatus != types.GroundingGrounded && it.GroundingStatus != types.GroundingRecovered {
		return false
	}
	object := strings.TrimSpace(it.Object)
	if object == "" {
		return false // runtime shape validation rejects new sparse rows earlier
	}
	if evidenceEndpointVisibleOnGroundedSpan(gc, *it, object) {
		return false
	}
	if claimed := decoratorSurfaceTermFromLabel(object); claimed != "" {
		if attachedDecoratorSurfaceTermSet(*it, gc)[claimed] {
			return false
		}
	}

	it.Kind = types.EvidenceUnresolved
	it.Confidence = 0
	it.GroundingStatus = types.GroundingUngrounded
	it.GroundingTier = ""
	appendGroundingNoteOnce(it, fmt.Sprintf(
		"registration object %q is absent from the grounded binding surface at %s:%d; cite the actual registry/decorator/table binding. If this is factory selection, emit the branch guard as conditional/condition and the concrete return as direct/return. Do not keep the definition or wrapper line as registration proof",
		object, it.Source, it.LineStart,
	))
	return true
}

// emitEvidenceRegistrationBindingRepair is a producer-owned recipe for a
// registration row that cited a nearby definition/wrapper while the already
// read source contains one unique binding call for the same endpoint.  It is
// deliberately only guidance: the downgraded row stays ungrounded and no
// registration edge exists until the model emits this exact tuple itself.
type emitEvidenceRegistrationBindingRepair struct {
	itemIndex   int
	source      string
	line        int
	anchor      string
	registry    string
	boundObject string
}

func emitEvidenceRequiredRegistrationBindingRepair(
	ctx *types.BusContext,
	item types.EvidenceItem,
	itemIndex int,
	gc *ground.Context,
) (emitEvidenceRegistrationBindingRepair, bool) {
	if ctx == nil || ctx.AnalysisIR == nil || gc == nil || item.Kind != types.EvidenceRegistration ||
		item.Scope != types.ScopeLine || item.LineStart <= 0 || strings.TrimSpace(item.Object) == "" {
		return emitEvidenceRegistrationBindingRepair{}, false
	}
	rm := ctx.AnalysisIR.RequestModel
	family := types.ResolveQuestionFamily(rm)
	if family == types.QFRootCauseTrace || rm.Intent == types.IntentRootCause {
		return emitEvidenceRegistrationBindingRepair{}, false
	}
	requiredShape := (family == types.QFCallChain && rm.Predicates.IsCrossComponent) ||
		rm.PredicateAxis == types.AxisRegister ||
		types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) == types.ReqRegistration
	if !requiredShape {
		return emitEvidenceRegistrationBindingRepair{}, false
	}

	line, anchor, registry, object, ok := uniqueNearbyRegistrationBindingCall(gc, item)
	if !ok {
		return emitEvidenceRegistrationBindingRepair{}, false
	}
	return emitEvidenceRegistrationBindingRepair{
		itemIndex: itemIndex, source: item.Source, line: line,
		anchor: anchor, registry: registry, boundObject: object,
	}, true
}

// uniqueNearbyRegistrationBindingCall inspects only source text that the model
// has already received.  It recognizes the cross-language structural shape
// receiver.bindingCall(wrapper(endpoint)) (including macro wrappers), without
// framework names or request/final-answer prose.  Ambiguity fails open.
func uniqueNearbyRegistrationBindingCall(gc *ground.Context, item types.EvidenceItem) (int, string, string, string, bool) {
	if gc == nil || item.LineStart <= 0 {
		return 0, "", "", "", false
	}
	visibleSource := ground.CanonicalContextPath(gc, item.Source)
	lines := gc.LineIndex[visibleSource]
	if len(lines) == 0 {
		lines = gc.LineIndex[item.Source]
	}
	if len(lines) == 0 {
		return 0, "", "", "", false
	}
	fi := emitEvidenceGraphFileInfo(gc, item.Source)
	wantOwner := enclosingCallableSymbolName(fi, item.LineStart)
	type candidate struct {
		line, score              int
		anchor, registry, object string
	}
	var candidates []candidate
	for lineNo, text := range lines {
		if lineNo < item.LineStart-40 || lineNo > item.LineStart+40 {
			continue
		}
		if wantOwner != "" && enclosingCallableSymbolName(fi, lineNo) != wantOwner {
			continue
		}
		anchor, registry, object, score, ok := registrationBindingCallOnLine(text, item.Object)
		if ok {
			candidates = append(candidates, candidate{lineNo, score, anchor, registry, object})
		}
	}
	if len(candidates) != 1 {
		// A declaration-shaped registration row can accurately name the
		// exported container without naming the body expression that binds an
		// endpoint into it.  Only inspect the unique parser-owned callable
		// selected by the structured definition anchor, and only accept one
		// receiver call in that already-read body.  This is deliberately
		// narrower than scanning nearby text or guessing from framework names.
		if len(candidates) != 0 {
			return 0, "", "", "", false
		}
		start, end, ok := registrationDefinitionCallableRange(fi, item)
		if !ok {
			return 0, "", "", "", false
		}
		for lineNo, text := range lines {
			if lineNo < start || lineNo > end {
				continue
			}
			for _, row := range receiverRegistrationBindingCallsOnLine(text) {
				candidates = append(candidates, candidate{
					line: lineNo, score: row.score,
					anchor: row.anchor, registry: row.registry, object: row.object,
				})
			}
		}
		if len(candidates) != 1 {
			return 0, "", "", "", false
		}
	}
	best := candidates[0]
	return best.line, best.anchor, best.registry, best.object, true
}

func registrationDefinitionCallableRange(fi *repomap.FileInfo, item types.EvidenceItem) (int, int, bool) {
	matched := selectedDefinitionCallable(fi, item)
	if matched == nil {
		return 0, 0, false
	}
	end := matched.EndLine
	if end < matched.Line {
		end = matched.Line
	}
	return matched.Line, end, true
}

func selectedDefinitionCallable(fi *repomap.FileInfo, item types.EvidenceItem) *repomap.Symbol {
	if fi == nil || item.AnchorKind != types.AnchorDefinition || item.LineStart <= 0 {
		return nil
	}
	want := types.NormalizedSurfaceSymbolTail(item.AnchorSymbol)
	if want == "" {
		want = types.NormalizedSurfaceSymbolTail(item.Subject)
	}
	if want == "" {
		return nil
	}
	var matched *repomap.Symbol
	for i := range fi.Symbols {
		sym := &fi.Symbols[i]
		if sym.Kind != "function" && sym.Kind != "method" {
			continue
		}
		if types.NormalizedSurfaceSymbolTail(sym.Name) != want || sym.Line <= 0 {
			continue
		}
		end := sym.EndLine
		if end < sym.Line {
			end = sym.Line
		}
		// Permit a bounded decorator/attribute prefix immediately before the
		// parser-owned declaration, but never a broad nearby-file search.
		if item.LineStart < sym.Line-4 || item.LineStart > end {
			continue
		}
		if matched != nil {
			return nil
		}
		matched = sym
	}
	return matched
}

type receiverRegistrationBindingCall struct {
	anchor, registry, object string
	score                    int
}

// receiverRegistrationBindingCallsOnLine discovers a structural
// receiver.bindingCall(argument) tuple without assuming a framework or
// endpoint spelling. Multiple distinct arguments remain ambiguous and fail
// open at the caller. Quoted strings are masked, so display/config text cannot
// become endpoint authority.
func receiverRegistrationBindingCallsOnLine(line string) []receiverRegistrationBindingCall {
	masked := maskQuotedSourceForBinding(strings.TrimSpace(line))
	if masked == "" {
		return nil
	}
	identifiers := sourceIdentifierTokens(masked)
	seen := make(map[string]bool)
	out := make([]receiverRegistrationBindingCall, 0, 1)
	for _, endpoint := range identifiers {
		anchor, registry, object, score, ok := registrationBindingCallOnLine(line, endpoint)
		if !ok || registry == anchor {
			continue
		}
		key := anchor + "\x00" + registry + "\x00" + object
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, receiverRegistrationBindingCall{
			anchor: anchor, registry: registry, object: object, score: score,
		})
	}
	return out
}

func sourceIdentifierTokens(masked string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, 8)
	for i := 0; i < len(masked); {
		if !isIdentifierByte(masked[i]) || (masked[i] >= '0' && masked[i] <= '9') {
			i++
			continue
		}
		start := i
		for i < len(masked) && isIdentifierByte(masked[i]) {
			i++
		}
		token := masked[start:i]
		if token != "" && !seen[token] {
			seen[token] = true
			out = append(out, token)
		}
	}
	return out
}

const autoPairedSelectedDefinitionBodyCallLimit = 24

// autoPairSelectedDefinitionBodyCallEvidence projects exact parser relations
// out of model-selected callable definitions. Admission is entirely typed:
// either QFCallChain + active endpoint profile, or a mechanism question with
// one required function/purpose answer dimension; plus a citable definition
// row, a unique parser callable at that source location, parser provenance,
// and exact read-line coverage. The mechanism lane excludes definitions that
// the model explicitly classified as related/absence/illustrative context, so
// a nearby helper cannot acquire principal-looking body facts merely because
// it was read. This helper never reads request/reasoning/summary/final prose
// and does not require the model to include a diagram.
func autoPairSelectedDefinitionBodyCallEvidence(
	ctx *types.BusContext,
	built []types.EvidenceItem,
	gc *ground.Context,
) []types.EvidenceItem {
	if ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil || gc == nil || len(built) == 0 {
		return nil
	}
	rm := ctx.AnalysisIR.RequestModel
	callChainLane := types.ResolveQuestionFamily(rm) == types.QFCallChain &&
		rm.CallChainEndpointProfile.Active()
	mechanismLane := types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) == types.ReqMechanism &&
		requestedFunctionOrPurposeDimensionRequired(rm.RequestedAnswerDimensions)
	if !callChainLane && !mechanismLane {
		return nil
	}
	seenCallable := make(map[string]bool)
	seenCall := make(map[string]bool)
	existingEvidence := append([]types.EvidenceItem(nil), ctx.Mutable.EmittedEvidence()...)
	existingEvidence = append(existingEvidence, built...)
	out := make([]types.EvidenceItem, 0, 4)
	for _, selected := range built {
		if selected.AnchorKind != types.AnchorDefinition || !selected.IsCitable() ||
			(selected.Scope != types.ScopeLine && selected.Scope != types.ScopeLineRange) {
			continue
		}
		if mechanismLane && !callChainLane {
			switch selected.ContextRole {
			case types.EvidenceContextRoleRelatedContext,
				types.EvidenceContextRoleAbsenceSupport,
				types.EvidenceContextRoleIllustrativeOnly:
				continue
			}
		}
		graph, fi, _, visibleSource, ok := ground.ResolveSourceGraphFile(gc, selected.Source)
		if !ok || graph == nil || fi == nil {
			continue
		}
		callable := selectedDefinitionCallable(fi, selected)
		if callable == nil {
			continue
		}
		callableKey := canonicalRelationSourcePath(visibleSource) + "\x00" + strconv.Itoa(callable.Line) + "\x00" + strings.TrimSpace(callable.Name)
		if seenCallable[callableKey] {
			continue
		}
		seenCallable[callableKey] = true
		caller := qualifiedEvidenceSymbolNameInFile(fi, callable)
		if caller == "" {
			continue
		}
		end := callable.EndLine
		if end < callable.Line {
			end = callable.Line
		}
		for i := range fi.Relations {
			rel := &fi.Relations[i]
			if rel.Kind != "call" || rel.Line < callable.Line || rel.Line > end ||
				(rel.Provenance != repomap.ProvenanceTreeSitter && rel.Provenance != repomap.ProvenanceCangjieParser) ||
				strings.TrimSpace(evidenceVisibleLineText(gc, visibleSource, rel.Line)) == "" {
				continue
			}
			callee := callRelationTargetName(graph, fi, rel)
			if callee == "" || strings.EqualFold(caller, callee) {
				continue
			}
			obligation := emitEvidenceRelationRepairObligation{
				EvidenceKind: types.EvidenceRelationship,
				AnchorKind:   types.AnchorCall,
				Source:       visibleSource,
				Line:         rel.Line,
				Subject:      caller,
				Object:       callee,
			}
			if emitEvidenceRelationRepairObligationsSatisfied(
				[]emitEvidenceRelationRepairObligation{obligation}, existingEvidence,
			) {
				continue
			}
			key := strings.ToLower(fmt.Sprintf("%s:%d:%s:%s:%s", visibleSource, rel.Line, caller, callee, rel.ResolvedBy))
			if seenCall[key] {
				continue
			}
			seenCall[key] = true
			item := types.EvidenceItem{
				Kind:            types.EvidenceRelationship,
				Subject:         caller,
				Predicate:       "calls",
				Object:          callee,
				Source:          visibleSource,
				LineStart:       rel.Line,
				LineEnd:         rel.Line,
				Confidence:      rel.Confidence,
				Producer:        types.EvidenceProducerRepoMapSelectedCallableBodyCall,
				Summary:         fmt.Sprintf("parser-authored call in model-selected callable: `%s` calls `%s` (extractor=%s)", caller, callee, rel.ResolvedBy),
				Scope:           types.ScopeLine,
				AnchorKind:      types.AnchorCall,
				AnchorSymbol:    strings.TrimSpace(rel.ToEP.Name),
				OwnerSymbol:     caller,
				DerivedFrom:     []string{selected.ID},
				GroundingStatus: types.GroundingGrounded,
				GroundingTier:   types.TierLineText,
				GroundingNote:   "parser-owned call from an already-read model-selected callable body",
			}
			item.ID = types.StableEvidenceID(item)
			out = append(out, item)
			if len(out) >= autoPairedSelectedDefinitionBodyCallLimit {
				return out
			}
		}
	}
	return out
}

// requestedFunctionOrPurposeDimensionRequired consumes only the normalized
// closed-enum presentation carrier. It is deliberately an enrichment signal:
// it can expose parser-proved operation facts from a model-selected definition
// but cannot reject completion, choose a principal member, or alter an answer.
func requestedFunctionOrPurposeDimensionRequired(profile *types.RequestedAnswerDimensionProfile) bool {
	if profile == nil || !profile.Active() {
		return false
	}
	for _, dimension := range profile.Dimensions {
		if dimension.Required && dimension.Role == types.RequestedAnswerDimensionFunctionOrPurpose {
			return true
		}
	}
	return false
}

func registrationBindingCallOnLine(line, endpoint string) (string, string, string, int, bool) {
	line = strings.TrimSpace(line)
	endpoint = strings.TrimSpace(endpoint)
	if line == "" || endpoint == "" {
		return "", "", "", 0, false
	}
	masked := maskQuotedSourceForBinding(line)
	endpointPos := indexIdentifierToken(masked, endpoint)
	if endpointPos < 0 {
		return "", "", "", 0, false
	}
	pairs := sourceParenPairs(masked)
	type callCandidate struct {
		anchor, registry, object string
		score                    int
	}
	var best callCandidate
	for open, close := range pairs {
		if endpointPos <= open || endpointPos >= close {
			continue
		}
		targetStart, target := sourceCallTargetBeforeParen(masked, open)
		if target == "" {
			continue
		}
		arg, ok := sourceCallArgumentContaining(line, masked, open+1, close, endpointPos)
		if !ok {
			continue
		}
		registry := sourceCallReceiver(target)
		anchor := sourceCallLeaf(target)
		if registry == "" {
			registry = anchor
		}
		arg = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(arg), "?"))
		if anchor == "" || registry == "" || arg == "" {
			continue
		}
		score := close - targetStart
		if registry != anchor {
			score += 10000
		}
		if strings.TrimSpace(arg) != endpoint {
			score += 1000
		}
		if score > best.score {
			best = callCandidate{anchor: anchor, registry: registry, object: arg, score: score}
		}
	}
	if best.score == 0 {
		return "", "", "", 0, false
	}
	return best.anchor, best.registry, best.object, best.score, true
}

func maskQuotedSourceForBinding(line string) string {
	out := []byte(line)
	quote := byte(0)
	escaped := false
	for i := 0; i < len(out); i++ {
		b := out[i]
		if quote != 0 {
			if escaped {
				escaped = false
				out[i] = ' '
				continue
			}
			if b == '\\' {
				escaped = true
				out[i] = ' '
				continue
			}
			if b == quote {
				quote = 0
			}
			out[i] = ' '
			continue
		}
		if b == '\'' || b == '"' || b == '`' {
			quote = b
			out[i] = ' '
		}
	}
	return string(out)
}

func sourceParenPairs(masked string) map[int]int {
	pairs := make(map[int]int)
	stack := make([]int, 0, 4)
	for i := 0; i < len(masked); i++ {
		switch masked[i] {
		case '(':
			stack = append(stack, i)
		case ')':
			if len(stack) == 0 {
				continue
			}
			open := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			pairs[open] = i
		}
	}
	return pairs
}

func sourceCallTargetBeforeParen(masked string, open int) (int, string) {
	j := open - 1
	for j >= 0 && (masked[j] == ' ' || masked[j] == '\t') {
		j--
	}
	end := j + 1
	for j >= 0 && (isCallTargetByte(masked[j]) || masked[j] == '!') {
		j--
	}
	start := j + 1
	if start >= end {
		return 0, ""
	}
	target := strings.TrimSuffix(cleanCallExpressionTarget(masked[start:end]), "!")
	return start, target
}

func sourceCallArgumentContaining(original, masked string, start, end, endpointPos int) (string, bool) {
	argStart := start
	depth := 0
	for i := start; i <= end; i++ {
		atEnd := i == end
		if !atEnd {
			switch masked[i] {
			case '(', '[', '{':
				depth++
			case ')', ']', '}':
				if depth > 0 {
					depth--
				}
			}
		}
		if atEnd || (masked[i] == ',' && depth == 0) {
			if endpointPos >= argStart && endpointPos < i {
				return strings.TrimSpace(original[argStart:i]), true
			}
			argStart = i + 1
		}
	}
	return "", false
}

func sourceCallReceiver(target string) string {
	idx, _ := lastSourceCallSeparator(target)
	if idx > 0 {
		return strings.TrimSpace(target[:idx])
	}
	return ""
}

func sourceCallLeaf(target string) string {
	idx, sepLen := lastSourceCallSeparator(target)
	if idx >= 0 {
		return strings.TrimSpace(target[idx+sepLen:])
	}
	return strings.TrimSpace(target)
}

func lastSourceCallSeparator(target string) (int, int) {
	idx, sepLen := -1, 0
	for _, sep := range []string{"->", "::", "."} {
		if candidate := strings.LastIndex(target, sep); candidate > idx {
			idx, sepLen = candidate, len(sep)
		}
	}
	// Lua's method-call separator is a single ':'. Do not let either byte of
	// a C++/Rust/Cangjie '::' qualifier override the typed two-byte separator.
	for i := len(target) - 1; i >= 0; i-- {
		if target[i] != ':' || (i > 0 && target[i-1] == ':') || (i+1 < len(target) && target[i+1] == ':') {
			continue
		}
		if i > idx {
			idx, sepLen = i, 1
		}
		break
	}
	return idx, sepLen
}

func evidenceEndpointVisibleOnGroundedSpan(gc *ground.Context, item types.EvidenceItem, endpoint string) bool {
	start, end := item.LineStart, item.LineEnd
	if start <= 0 {
		return false
	}
	if end < start {
		end = start
	}
	// Evidence line ranges are already bounded at the schema/grounding layer.
	// Inspect only their exact observed source span; do not widen into nearby
	// source or model-authored snippets.
	for line := start; line <= end; line++ {
		if evidenceEndpointVisibleOnSourceLine(evidenceVisibleLineText(gc, item.Source, line), endpoint) {
			return true
		}
	}
	return false
}

func evidenceEndpointVisibleOnSourceLine(line, endpoint string) bool {
	line = strings.TrimSpace(line)
	endpoint = strings.TrimSpace(endpoint)
	if line == "" || endpoint == "" {
		return false
	}
	for _, candidate := range []string{
		endpoint,
		strings.Trim(endpoint, "`'\""),
		types.NormalizedSurfaceSymbolTail(endpoint),
	} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if evidenceEndpointCandidateVisible(line, candidate) {
			return true
		}
	}
	return false
}

func evidenceEndpointCandidateVisible(line, candidate string) bool {
	bareIdentifier := true
	for i := 0; i < len(candidate); i++ {
		if !isIdentifierByte(candidate[i]) {
			bareIdentifier = false
			break
		}
	}
	if bareIdentifier {
		return indexIdentifierToken(line, candidate) >= 0
	}
	// Qualified names, decorators, and quoted registration keys carry their
	// own exact punctuation boundaries.  Match those byte-exactly; their
	// normalized identifier tail is checked separately by the caller.
	return strings.Contains(line, candidate)
}

func stabilizeConditionAnchorClaim(it *types.EvidenceItem, gc *ground.Context) bool {
	if it == nil || gc == nil || it.Source == "" || it.LineStart <= 0 {
		return false
	}
	claim := normalizeStatementLocalAnchorClaim(it.Condition)
	if claim == "" {
		return false
	}
	lineText := statementLocalAnchorWindowText(gc, it.Source, it.LineStart, 0)
	if conditionClaimsStructurallyEquivalent(it.Condition, lineText) {
		return false
	}
	window := normalizeStatementLocalAnchorClaim(statementLocalAnchorWindowText(gc, it.Source, it.LineStart, 2))
	if window == "" {
		return false
	}
	if strings.Contains(window, claim) || strings.Contains(claim, window) {
		return false
	}
	if conditionClaimCorroboratedBySnippet(it, window) {
		return false
	}
	it.Kind = types.EvidenceUnresolved
	it.Confidence = 0
	it.GroundingStatus = types.GroundingUngrounded
	it.GroundingTier = ""
	appendGroundingNoteOnce(it,
		"this condition anchor is not supported by the grounded source lines at the cited location. Keep condition anchors tied to the actual current-code guard text shown by the read_file gutter or drop the speculative conditional claim. Do NOT repair this item.",
	)
	return true
}

// normalizeConditionEvidenceAnchorSymbol repairs only the navigation token of
// an already line-equivalent typed condition. The condition remains the
// semantic authority; a model-supplied label from the guarded body must not
// make the true guard ungrounded when the same typed condition carries a
// unique visible identifier on the cited line.
func normalizeConditionEvidenceAnchorSymbol(it *types.EvidenceItem, gc *ground.Context) string {
	if it == nil || gc == nil || it.AnchorKind != types.AnchorCondition || it.Source == "" || it.LineStart <= 0 {
		return ""
	}
	lineText := statementLocalAnchorWindowText(gc, it.Source, it.LineStart, 0)
	if !conditionClaimsStructurallyEquivalent(it.Condition, lineText) {
		return ""
	}
	if _, ok := ground.VerifyLineAnchor(gc, it.Source, it.LineStart, strings.TrimSpace(it.AnchorSymbol), 0); ok {
		return ""
	}
	candidates := emitEvidenceIdentifierCandidates(it.Condition)
	seen := map[string]bool{}
	visible := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		if _, ok := ground.VerifyLineAnchor(gc, it.Source, it.LineStart, candidate, 0); ok {
			visible = append(visible, candidate)
		}
	}
	if len(visible) == 0 {
		return ""
	}
	// Prefer the last visible typed-field identifier: for selector chains this
	// is the outer method/member (x.Flags().Changed(...) -> Changed), while it
	// remains only a line-navigation anchor rather than a semantic call claim.
	replacement := visible[len(visible)-1]
	previous := strings.TrimSpace(it.AnchorSymbol)
	if previous == replacement {
		return ""
	}
	it.AnchorSymbol = replacement
	return fmt.Sprintf("condition anchor_symbol %q was not visible on the cited guard; normalized it to visible typed-condition token %q", previous, replacement)
}

func conditionClaimsStructurallyEquivalent(left, right string) bool {
	leftExpr, leftNegated, leftOK := canonicalAtomicBooleanCondition(left)
	rightExpr, rightNegated, rightOK := canonicalAtomicBooleanCondition(right)
	return leftOK && rightOK && leftExpr == rightExpr && leftNegated == rightNegated
}

var (
	conditionBooleanSuffixRe = regexp.MustCompile(`(?is)^(.*?)\s*(==|!=)\s*(true|false)\s*$`)
	conditionBooleanPrefixRe = regexp.MustCompile(`(?is)^(true|false)\s*(==|!=)\s*(.*?)\s*$`)
)

// canonicalAtomicBooleanCondition recognizes exact syntactic equivalences for
// one atomic boolean expression only. In particular, !x, not x, x == false,
// false == x, x != true, and language-level unless x share the same carrier.
// It deliberately rejects top-level compound/comparison expressions so this
// remains a precise grounding signal rather than a general theorem prover.
func canonicalAtomicBooleanCondition(raw string) (expr string, negated bool, ok bool) {
	body, keywordNegated := trimConditionStatementSyntax(raw)
	if body == "" {
		return "", false, false
	}
	negated = keywordNegated
	body = trimBalancedConditionParens(body)
	if match := conditionBooleanSuffixRe.FindStringSubmatch(body); len(match) == 4 {
		body = trimBalancedConditionParens(match[1])
		literal := strings.EqualFold(strings.TrimSpace(match[3]), "true")
		comparisonNegated := (match[2] == "==" && !literal) || (match[2] == "!=" && literal)
		negated = negated != comparisonNegated
	} else if match := conditionBooleanPrefixRe.FindStringSubmatch(body); len(match) == 4 {
		body = trimBalancedConditionParens(match[3])
		literal := strings.EqualFold(strings.TrimSpace(match[1]), "true")
		comparisonNegated := (match[2] == "==" && !literal) || (match[2] == "!=" && literal)
		negated = negated != comparisonNegated
	} else {
		trimmed := strings.TrimSpace(body)
		for {
			switch {
			case strings.HasPrefix(trimmed, "!") && !strings.HasPrefix(trimmed, "!="):
				negated = !negated
				trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "!"))
			case conditionStartsWithKeyword(trimmed, "not"):
				negated = !negated
				trimmed = strings.TrimSpace(trimmed[len("not"):])
			default:
				body = trimBalancedConditionParens(trimmed)
				goto normalized
			}
		}
	}

normalized:
	if body == "" || conditionHasTopLevelBinaryOperator(body) {
		return "", false, false
	}
	expr = normalizeStatementLocalAnchorClaim(body)
	if expr == "" {
		return "", false, false
	}
	return expr, negated, true
}

func trimConditionStatementSyntax(raw string) (body string, negated bool) {
	body = strings.TrimSpace(raw)
	for _, keyword := range []string{"if", "when", "while", "guard"} {
		if conditionStartsWithKeyword(body, keyword) {
			body = strings.TrimSpace(body[len(keyword):])
			break
		}
	}
	if conditionStartsWithKeyword(body, "unless") {
		body = strings.TrimSpace(body[len("unless"):])
		negated = true
	}
	body = strings.TrimSpace(body)
	if strings.HasSuffix(strings.ToLower(body), " then") {
		body = strings.TrimSpace(body[:len(body)-len(" then")])
	}
	for len(body) > 0 {
		last := body[len(body)-1]
		if last != '{' && last != ':' {
			break
		}
		body = strings.TrimSpace(body[:len(body)-1])
	}
	return body, negated
}

func conditionStartsWithKeyword(raw, keyword string) bool {
	raw = strings.TrimSpace(raw)
	if len(raw) < len(keyword) || !strings.EqualFold(raw[:len(keyword)], keyword) {
		return false
	}
	if len(raw) == len(keyword) {
		return true
	}
	next := raw[len(keyword)]
	return next == ' ' || next == '\t' || next == '\r' || next == '\n' || next == '('
}

func trimBalancedConditionParens(raw string) string {
	raw = strings.TrimSpace(raw)
	for len(raw) >= 2 && raw[0] == '(' && raw[len(raw)-1] == ')' && conditionOuterParensBalanced(raw) {
		raw = strings.TrimSpace(raw[1 : len(raw)-1])
	}
	return raw
}

func conditionOuterParensBalanced(raw string) bool {
	depth := 0
	quote := byte(0)
	escaped := false
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' || ch == '`' {
			quote = ch
			continue
		}
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 && i != len(raw)-1 {
				return false
			}
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0 && quote == 0
}

func conditionHasTopLevelBinaryOperator(raw string) bool {
	depth := 0
	quote := byte(0)
	escaped := false
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' || ch == '`' {
			quote = ch
			continue
		}
		switch ch {
		case '(', '[', '{':
			depth++
			continue
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth != 0 {
			continue
		}
		if i+1 < len(raw) {
			pair := raw[i : i+2]
			switch pair {
			case "&&", "||", "==", "!=", "<=", ">=":
				return true
			}
		}
		if ch == '<' || ch == '>' {
			return true
		}
		if conditionWordOperatorAt(raw, i, "and") || conditionWordOperatorAt(raw, i, "or") {
			return true
		}
	}
	return false
}

func conditionWordOperatorAt(raw string, index int, word string) bool {
	if index < 0 || index+len(word) > len(raw) || !strings.EqualFold(raw[index:index+len(word)], word) {
		return false
	}
	leftBoundary := index == 0 || !isConditionIdentifierByte(raw[index-1])
	right := index + len(word)
	rightBoundary := right == len(raw) || !isConditionIdentifierByte(raw[right])
	return leftBoundary && rightBoundary
}

func isConditionIdentifierByte(ch byte) bool {
	return ch == '_' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9'
}

func conditionClaimCorroboratedBySnippet(it *types.EvidenceItem, normalizedWindow string) bool {
	if it == nil || strings.TrimSpace(normalizedWindow) == "" {
		return false
	}
	if !conditionClaimUsesExplicitOmission(it.Condition) {
		return false
	}
	fragments := conditionClaimOmissionFragments(it.Condition)
	if len(fragments) == 0 {
		return false
	}
	for _, fragment := range fragments {
		if !strings.Contains(normalizedWindow, fragment) {
			return false
		}
	}
	return true
}

func conditionClaimUsesExplicitOmission(condition string) bool {
	return strings.Contains(condition, "...") || strings.Contains(condition, "…")
}

func conditionClaimOmissionFragments(condition string) []string {
	condition = strings.ReplaceAll(condition, "…", "...")
	rawParts := strings.Split(condition, "...")
	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		part = normalizeStatementLocalAnchorClaim(part)
		if len(part) < 3 {
			continue
		}
		parts = append(parts, part)
	}
	return parts
}

func statementLocalAnchorWindowText(gc *ground.Context, source string, line, span int) string {
	if gc == nil || line <= 0 {
		return ""
	}
	file := strings.TrimSpace(strings.ReplaceAll(source, `\`, `/`))
	if file == "" {
		return ""
	}
	lines := gc.LineIndex[file]
	if len(lines) == 0 {
		return ""
	}
	if span < 0 {
		span = 0
	}
	parts := make([]string, 0, span+1)
	for current := line; current <= line+span; current++ {
		text := strings.TrimSpace(lines[current])
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, " ")
}

func normalizeStatementLocalAnchorClaim(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"`", "",
		"'", "",
		"\"", "",
		"(", "",
		")", "",
		"{", "",
		"}", "",
		"[", "",
		"]", "",
		",", "",
		":", "",
		";", "",
		"\t", "",
		"\r", "",
		"\n", "",
		" ", "",
	)
	return replacer.Replace(s)
}

func enclosingEvidenceCallableOwner(it *types.EvidenceItem, gc *ground.Context) string {
	if it == nil || gc == nil || it.Source == "" || it.LineStart <= 0 {
		return ""
	}
	switch it.AnchorKind {
	case types.AnchorCall, types.AnchorCallback, types.AnchorArgument, types.AnchorCondition, types.AnchorReturn, types.AnchorAssignment, types.AnchorInitializer, types.AnchorStringLiteral:
	default:
		return ""
	}
	_, fi, _, _, ok := ground.ResolveSourceGraphFile(gc, it.Source)
	if !ok {
		return ""
	}
	return enclosingCallableSymbolName(fi, it.LineStart)
}

func enclosingEvidenceQualifiedCallableOwner(it *types.EvidenceItem, gc *ground.Context) string {
	if it == nil || gc == nil || it.Source == "" || it.LineStart <= 0 {
		return ""
	}
	switch it.AnchorKind {
	case types.AnchorCall, types.AnchorCallback, types.AnchorArgument, types.AnchorCondition, types.AnchorReturn,
		types.AnchorAssignment, types.AnchorInitializer, types.AnchorStringLiteral:
	default:
		return ""
	}
	_, fi, _, _, ok := ground.ResolveSourceGraphFile(gc, it.Source)
	if !ok {
		return ""
	}
	return qualifiedEvidenceSymbolNameInFile(fi, enclosingCallableSymbol(fi, it.LineStart))
}

func stampEvidenceTypedIdentityBindings(it *types.EvidenceItem, gc *ground.Context) bool {
	if it == nil || gc == nil || !it.IsCitable() {
		return false
	}
	changed := false
	if owner := enclosingEvidenceQualifiedCallableOwner(it, gc); owner != "" && it.OwnerIdentity != owner {
		it.OwnerIdentity = owner
		changed = true
	}
	if it.AnchorKind != types.AnchorDefinition {
		graph, fi, _, _, ok := ground.ResolveSourceGraphFile(gc, it.Source)
		var bindings []types.EvidenceDeclaredIdentityBinding
		if ok && fi != nil {
			bindings = parserDeclaredIdentityBindingsForOperation(graph, fi, *it)
		}
		if !sameEvidenceDeclaredIdentityBindings(it.DeclaredIdentityBindings, bindings) {
			it.DeclaredIdentityBindings = bindings
			changed = true
		}
		return changed
	}
	_, fi, _, _, ok := ground.ResolveSourceGraphFile(gc, it.Source)
	if !ok || fi == nil {
		return changed
	}
	anchorTail := types.NormalizedSurfaceSymbolTail(it.AnchorSymbol)
	subjectTail := types.NormalizedSurfaceSymbolTail(it.Subject)
	var matched *repomap.Symbol
	for idx := range fi.Symbols {
		sym := &fi.Symbols[idx]
		if strings.TrimSpace(sym.DeclaredType) == "" || sym.Line <= 0 || it.LineStart < sym.Line ||
			(sym.EndLine >= sym.Line && it.LineStart > sym.EndLine) {
			continue
		}
		nameTail := types.NormalizedSurfaceSymbolTail(sym.Name)
		if nameTail == "" || (nameTail != anchorTail && nameTail != subjectTail) {
			continue
		}
		if matched != nil {
			// Multiple static declarations matching the same cited surface are
			// ambiguous. Do not publish a typed alias.
			return changed
		}
		matched = sym
	}
	if matched == nil {
		return changed
	}
	binding := strings.TrimSpace(matched.Name)
	owner := strings.TrimSpace(matched.Parent)
	if owner != "" {
		binding = owner + "." + binding
	}
	if it.DeclaredBinding != binding {
		it.DeclaredBinding = binding
		changed = true
	}
	if declaredType := strings.TrimSpace(matched.DeclaredType); it.DeclaredType != declaredType {
		it.DeclaredType = declaredType
		changed = true
	}
	if it.DeclaredOwner != owner {
		it.DeclaredOwner = owner
		changed = true
	}
	return changed
}

func parserDeclaredIdentityBindingsForOperation(graph *repomap.Graph, fi *repomap.FileInfo, it types.EvidenceItem) []types.EvidenceDeclaredIdentityBinding {
	if fi == nil || strings.TrimSpace(it.OwnerIdentity) == "" ||
		(strings.TrimSpace(it.Subject) == "" && strings.TrimSpace(it.Object) == "") {
		return nil
	}
	byBinding := make(map[string]types.EvidenceDeclaredIdentityBinding)
	ambiguous := make(map[string]bool)
	seenDeclarations := make(map[string]bool)
	add := func(sym repomap.Symbol, local bool) {
		name := strings.TrimSpace(sym.Name)
		owner := strings.TrimSpace(sym.Parent)
		declaredType := strings.TrimSpace(sym.DeclaredType)
		if name == "" || owner == "" || declaredType == "" ||
			(!local && !parserDeclarationSharesOperationNamespace(graph, fi, sym)) ||
			(!types.AnswerCodeIdentityOwnsEndpoint(owner, it.OwnerIdentity) &&
				!types.AnswerCodeIdentitySurfacesCompatible(owner, it.OwnerIdentity)) {
			return
		}
		if !types.AnswerCodeIdentityContainsExactSegment(it.Subject, name) &&
			!types.AnswerCodeIdentityContainsExactSegment(it.Object, name) {
			return
		}
		declarationPath := strings.TrimSpace(strings.ReplaceAll(sym.File, `\`, "/"))
		if declarationPath == "" && local {
			declarationPath = strings.TrimSpace(strings.ReplaceAll(fi.RelPath, `\`, "/"))
		}
		declarationKey := strings.Join([]string{
			declarationPath, owner, name, declaredType,
		}, "\x00")
		if seenDeclarations[declarationKey] {
			return
		}
		seenDeclarations[declarationKey] = true
		binding := types.EvidenceDeclaredIdentityBinding{
			Binding: owner + "." + name,
			Type:    declaredType,
			Owner:   owner,
		}
		if prior, ok := byBinding[binding.Binding]; ok {
			if prior.Type != binding.Type || prior.Owner != binding.Owner {
				ambiguous[binding.Binding] = true
			}
			return
		}
		byBinding[binding.Binding] = binding
	}
	for idx := range fi.Symbols {
		add(fi.Symbols[idx], true)
	}
	// Methods and their receiver fields commonly live in different source
	// files (Go package receivers are the production witness).  Once the
	// operation owner is parser-stamped, consult only exact endpoint segments
	// in the graph's name index and admit declarations from the same language
	// namespace.  This is an identity bridge only: the independently grounded
	// argument/assignment remains the sole relation authority.
	if graph != nil {
		for _, name := range parserOperationEndpointIdentifiers(it.Subject, it.Object) {
			for _, candidate := range graph.SymbolDefs[name] {
				if candidate != nil {
					add(*candidate, false)
				}
			}
		}
	}
	bindings := make([]types.EvidenceDeclaredIdentityBinding, 0, len(byBinding))
	for key, binding := range byBinding {
		if !ambiguous[key] {
			bindings = append(bindings, binding)
		}
	}
	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].Binding != bindings[j].Binding {
			return bindings[i].Binding < bindings[j].Binding
		}
		if bindings[i].Type != bindings[j].Type {
			return bindings[i].Type < bindings[j].Type
		}
		return bindings[i].Owner < bindings[j].Owner
	})
	return bindings
}

func parserOperationEndpointIdentifiers(values ...string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, value := range values {
		for _, token := range strings.FieldsFunc(value, func(r rune) bool {
			return r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r)
		}) {
			token = strings.TrimSpace(token)
			if token == "" || seen[token] {
				continue
			}
			seen[token] = true
			out = append(out, token)
		}
	}
	return out
}

func parserDeclarationSharesOperationNamespace(graph *repomap.Graph, operationFile *repomap.FileInfo, declaration repomap.Symbol) bool {
	if operationFile == nil {
		return false
	}
	operationPath := strings.TrimSpace(strings.ReplaceAll(operationFile.RelPath, `\`, "/"))
	declarationPath := strings.TrimSpace(strings.ReplaceAll(declaration.File, `\`, "/"))
	if declarationPath == "" {
		return false
	}
	if declarationPath == operationPath {
		return true
	}
	if graph == nil {
		return false
	}
	declarationFile := graph.FileIndex[declarationPath]
	if declarationFile == nil || declarationFile.Language != operationFile.Language {
		return false
	}
	operationPackage := strings.TrimSpace(operationFile.Package)
	declarationPackage := strings.TrimSpace(declarationFile.Package)
	if operationPackage != "" || declarationPackage != "" {
		return operationPackage != "" && operationPackage == declarationPackage
	}
	// Languages without a parser-owned package/module identity remain bounded
	// to one directory.  A same-named class in another directory cannot become
	// a hard carrier identity merely because the whole repository is indexed.
	return path.Clean(path.Dir(operationPath)) == path.Clean(path.Dir(declarationPath))
}

func sameEvidenceDeclaredIdentityBindings(left, right []types.EvidenceDeclaredIdentityBinding) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func conflictingLineLocalCallableClaim(it types.EvidenceItem, fi *repomap.FileInfo, owner string) string {
	if fi == nil {
		return ""
	}
	ownerTail := types.NormalizedSurfaceSymbolTail(owner)
	if ownerTail == "" {
		return ""
	}
	callableTails := fileCallableTailSet(fi)
	if len(callableTails) == 0 {
		return ""
	}
	if tail := callableTailForOwnerCheck(strings.TrimSpace(it.Subject), ownerTail, callableTails); tail != "" && tail != ownerTail {
		return strings.TrimSpace(it.Subject)
	}
	if tail := callableTailForOwnerCheck(strings.TrimSpace(it.AnchorSymbol), ownerTail, callableTails); tail != "" && tail != ownerTail {
		return strings.TrimSpace(it.AnchorSymbol)
	}
	return ""
}

func callableTailForOwnerCheck(raw, ownerTail string, callableTails map[string]bool) string {
	tail := types.NormalizedSurfaceSymbolTail(raw)
	if tail == "" {
		return ""
	}
	if tail == ownerTail {
		return tail
	}
	if callableTails[tail] {
		return tail
	}
	return ""
}

func fileCallableTailSet(fi *repomap.FileInfo) map[string]bool {
	if fi == nil {
		return nil
	}
	out := make(map[string]bool)
	for i := range fi.Symbols {
		sym := &fi.Symbols[i]
		switch sym.Kind {
		case "function", "method":
		default:
			continue
		}
		for _, raw := range []string{qualifiedEvidenceSymbolName(sym), strings.TrimSpace(sym.Name)} {
			tail := types.NormalizedSurfaceSymbolTail(raw)
			if tail != "" {
				out[tail] = true
			}
		}
	}
	return out
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
		if anchorSymbol == "" || relName == anchorSymbol || relName == shortAnchor {
			return rel, true
		}
	}
	if anchorSymbol == "" && fallback != nil {
		return fallback, true
	}
	return nil, false
}

// findCallRelationAtLineForCandidates locates the first call
// relation at `line` whose target name matches ANY of `candidates`,
// in a SINGLE pass over fi.Relations. The previous implementation
// called findCallRelationAtLine once per candidate (O(K × N) with
// K=len(candidates), N=len(fi.Relations)). Most LLM-emitted evidence
// supplies 2-3 candidates (AnchorSymbol / Object / Subject) and
// fi.Relations is typically tens to a few hundred entries, so the
// quadratic shape is small but free to remove.
//
// Match semantics preserved verbatim from findCallRelationAtLine:
//   - Empty / whitespace candidate slot is skipped.
//   - Match accepts both the full candidate AND its last dot segment
//     (so `pkg.Method` candidates also match a relation whose
//     ToEP.Name is just `Method`).
//   - When candidates yield no match the helper returns false rather
//     than substituting a different same-line call target.
//
// The no-match path is intentionally stricter than the legacy fallback:
// a wrong callee is worse than leaving the LLM's already-grounded
// candidate untouched.
func findCallRelationAtLineForCandidates(fi *repomap.FileInfo, line int, candidates []string) (*repomap.Relation, bool) {
	if fi == nil || line <= 0 {
		return nil, false
	}
	if len(candidates) == 0 {
		return findCallRelationAtLine(fi, line, "")
	}
	candSet := make(map[string]bool, 2*len(candidates))
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		candSet[c] = true
		// Also store the last dot segment so `pkg.Method` candidates
		// match a relation whose ToEP.Name is just `Method`. Mirrors
		// findCallRelationAtLine's shortAnchor logic.
		if short := emitLastDotSegment(c); short != "" && short != c {
			candSet[short] = true
		}
	}
	if len(candSet) == 0 {
		return findCallRelationAtLine(fi, line, "")
	}
	for i := range fi.Relations {
		rel := &fi.Relations[i]
		if rel.Kind != "call" || rel.Line != line {
			continue
		}
		relName := strings.TrimSpace(rel.ToEP.Name)
		if candSet[relName] {
			return rel, true
		}
	}
	return nil, false
}

func enclosingCallableSymbolName(fi *repomap.FileInfo, line int) string {
	return qualifiedEvidenceSymbolName(enclosingCallableSymbol(fi, line))
}

func enclosingCallableSymbol(fi *repomap.FileInfo, line int) *repomap.Symbol {
	if fi == nil || line <= 0 {
		return nil
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
	return best
}

// qualifiedEvidenceSymbolNameInFile preserves receiver/parent qualification
// first, then adds the parser-owned package/module for top-level callables.
// This is identity metadata, not a display rewrite. Separator choice follows
// the typed source language; downstream endpoint comparison accepts the same
// closed '.', '::', and '#' forms used by the diagram contract.
func qualifiedEvidenceSymbolNameInFile(fi *repomap.FileInfo, sym *repomap.Symbol) string {
	name := qualifiedEvidenceSymbolName(sym)
	if fi == nil || sym == nil || name == "" || strings.TrimSpace(sym.Receiver) != "" || strings.TrimSpace(sym.Parent) != "" {
		return name
	}
	pkg := strings.TrimSpace(fi.Package)
	if pkg == "" {
		return name
	}
	separator := "."
	switch strings.ToLower(strings.TrimSpace(fi.Language)) {
	case "cangjie", "rust", "cpp", "c++":
		separator = "::"
	}
	return strings.TrimRight(pkg, ".:") + separator + name
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
	return name
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
	best, width := -1, 0
	for _, sep := range []string{".", "::", "->"} {
		if idx := strings.LastIndex(s, sep); idx >= 0 && idx+len(sep) < len(s) && idx > best {
			best, width = idx, len(sep)
		}
	}
	if best >= 0 {
		return s[best+width:]
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

func filterNoopDuplicateEmitEvidence(existing, items []types.EvidenceItem, reports []ground.Report) ([]types.EvidenceItem, []ground.Report, []types.EvidenceItem) {
	if len(existing) == 0 || len(items) == 0 {
		return items, reports, nil
	}
	seen := make(map[string]types.EvidenceItem, len(existing)+len(items))
	seenRevision := make(map[string]types.EvidenceItem, len(existing)+len(items))
	for _, item := range existing {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = types.StableEvidenceID(item)
			item.ID = id
		}
		seen[types.EvidenceStableMergeKey(item)] = item
		if key := types.EvidenceRevisionKey(item); key != "" {
			seenRevision[key] = item
		}
	}
	kept := make([]types.EvidenceItem, 0, len(items))
	keptReports := make([]ground.Report, 0, len(reports))
	duplicates := make([]types.EvidenceItem, 0)
	for i, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = types.StableEvidenceID(item)
			item.ID = id
		}
		prior, ok := seen[types.EvidenceStableMergeKey(item)]
		if !ok {
			if key := types.EvidenceRevisionKey(item); key != "" {
				prior, ok = seenRevision[key]
			}
		}
		if ok && emitEvidenceNoopDuplicate(prior, item) {
			duplicates = append(duplicates, item)
			continue
		}
		kept = append(kept, item)
		if i < len(reports) {
			keptReports = append(keptReports, reports[i])
		} else {
			keptReports = append(keptReports, ground.Report{
				ItemID:       item.ID,
				Status:       item.GroundingStatus,
				Tier:         item.GroundingTier,
				OriginalLine: item.LineStart,
				AdjustedLine: item.LineStart,
				Note:         item.GroundingNote,
			})
		}
		seen[types.EvidenceStableMergeKey(item)] = item
		if key := types.EvidenceRevisionKey(item); key != "" {
			seenRevision[key] = item
		}
	}
	return kept, keptReports, duplicates
}

func emitEvidenceAmendedItems(existing, items []types.EvidenceItem) []types.EvidenceItem {
	if len(existing) == 0 || len(items) == 0 {
		return nil
	}
	seen := make(map[string]types.EvidenceItem, len(existing))
	seenRevision := make(map[string]types.EvidenceItem, len(existing))
	for _, item := range existing {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = types.StableEvidenceID(item)
		}
		if id != "" {
			seen[types.EvidenceStableMergeKey(item)] = item
		}
		if key := types.EvidenceRevisionKey(item); key != "" {
			seenRevision[key] = item
		}
	}
	var amended []types.EvidenceItem
	for _, item := range items {
		prior, ok := seen[types.EvidenceStableMergeKey(item)]
		if !ok {
			if key := types.EvidenceRevisionKey(item); key != "" {
				prior, ok = seenRevision[key]
			}
		}
		if ok && !emitEvidenceNoopDuplicate(prior, item) {
			amended = append(amended, item)
		}
	}
	return amended
}

func emitEvidenceNoopDuplicate(existing, incoming types.EvidenceItem) bool {
	existingID := strings.TrimSpace(existing.ID)
	if existingID == "" {
		existingID = types.StableEvidenceID(existing)
	}
	incomingID := strings.TrimSpace(incoming.ID)
	if incomingID == "" {
		incomingID = types.StableEvidenceID(incoming)
	}
	if existingID == "" || existingID != incomingID {
		existingRevision := types.EvidenceRevisionKey(existing)
		incomingRevision := types.EvidenceRevisionKey(incoming)
		if existingRevision == "" || existingRevision != incomingRevision {
			return false
		}
	}
	if existing.AnchorKind != incoming.AnchorKind ||
		strings.TrimSpace(existing.AnchorSymbol) != strings.TrimSpace(incoming.AnchorSymbol) ||
		strings.TrimSpace(existing.OwnerSymbol) != strings.TrimSpace(incoming.OwnerSymbol) {
		return false
	}
	// The typed claim carrier is mutable correction metadata too. In
	// particular, subject/predicate/object may be added after a completion gate
	// reports that a grounded call anchor lacks an explicit directed edge. Do
	// not classify that correction as an exact duplicate merely because it
	// points at the same source line and callee token.
	if existing.Kind != incoming.Kind || existing.Scope != incoming.Scope ||
		strings.TrimSpace(existing.Subject) != strings.TrimSpace(incoming.Subject) ||
		strings.TrimSpace(existing.Predicate) != strings.TrimSpace(incoming.Predicate) ||
		strings.TrimSpace(existing.Object) != strings.TrimSpace(incoming.Object) ||
		strings.TrimSpace(existing.Condition) != strings.TrimSpace(incoming.Condition) ||
		strings.TrimSpace(existing.Snippet) != strings.TrimSpace(incoming.Snippet) {
		return false
	}
	if emitEvidenceGroundingRank(incoming.GroundingStatus) > emitEvidenceGroundingRank(existing.GroundingStatus) {
		return false
	}
	if existing.EvidenceRef == "" && incoming.EvidenceRef != "" {
		return false
	}
	if incoming.Confidence > existing.Confidence {
		return false
	}
	if !emitEvidenceStringSliceContainsAll(existing.SurfaceTerms, incoming.SurfaceTerms) ||
		!emitEvidenceStringSliceContainsAll(existing.DerivedFrom, incoming.DerivedFrom) {
		return false
	}
	if existing.ContextRole == types.EvidenceContextRoleUnknown && incoming.ContextRole != types.EvidenceContextRoleUnknown {
		return false
	}
	if existing.DiagramRole == types.EvidenceDiagramRoleUnknown && incoming.DiagramRole != types.EvidenceDiagramRoleUnknown {
		return false
	}
	if existing.RequestedDiagramRole == types.EvidenceDiagramRoleUnknown && incoming.RequestedDiagramRole != types.EvidenceDiagramRoleUnknown {
		return false
	}
	return true
}

func emitEvidenceGroundingRank(status types.GroundingStatus) int {
	switch status {
	case types.GroundingGrounded:
		return 3
	case types.GroundingRecovered:
		return 2
	case types.GroundingUngrounded:
		return 1
	default:
		return 0
	}
}

func emitEvidenceStringSliceContainsAll(haystack, needles []string) bool {
	if len(needles) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(haystack))
	for _, item := range haystack {
		item = strings.TrimSpace(item)
		if item != "" {
			set[item] = struct{}{}
		}
	}
	for _, item := range needles {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := set[item]; !ok {
			return false
		}
	}
	return true
}

func renderEmitEvidenceDuplicateNoopSummary(duplicates, allEvidence []types.EvidenceItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "emit_evidence accepted 0 new item(s); skipped %d duplicate item(s) already present in the evidence buffer.\n\n", len(duplicates))
	b.WriteString(renderEvidenceItemPreview(duplicates, "duplicate"))
	cumulative := evidenceGroundingTally(allEvidence)
	fmt.Fprintf(&b, "\nEvidence buffer (audit, cumulative): %d grounded / %d recovered / %d ungrounded across %d file(s).\n",
		cumulative.grounded, cumulative.recovered, cumulative.ungrounded, emitEvidenceSourceCount(allEvidence))
	b.WriteString("No new evidence was recorded. Do not re-emit exact same facts. If a prior row has wrong metadata, re-emit the same source/line/fact with corrected non-empty fields; otherwise emit genuinely new/enriched evidence from a different anchor, or call `emit_investigation_complete` if the investigation is ready to close.\n")
	return b.String()
}

func renderEmitEvidenceDuplicateSkipNote(duplicates []types.EvidenceItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Skipped %d duplicate item(s) already present in the evidence buffer; they were not recorded again:\n", len(duplicates))
	b.WriteString(renderEvidenceItemPreview(duplicates, "duplicate"))
	b.WriteString("Exact duplicate rows are audit context only; they do not require a consolidated re-emit. Corrected same-ID rows are accepted as amendments.\n")
	return b.String()
}

func renderEmitEvidenceAmendmentNote(items []types.EvidenceItem) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Updated %d existing evidence item(s) by stable evidence identity; answer-grade snapshots keep one merged row with the latest corrected metadata:\n", len(items))
	b.WriteString(renderEvidenceItemPreview(items, "amendment"))
	b.WriteString("This is safe for metadata corrections. Exact duplicates remain no-progress no-ops.\n")
	return b.String()
}

func renderEvidenceItemPreview(items []types.EvidenceItem, label string) string {
	if len(items) == 0 {
		return ""
	}
	label = strings.TrimSpace(label)
	if label == "" {
		label = "item"
	}
	const max = 5
	var b strings.Builder
	limit := len(items)
	if limit > max {
		limit = max
	}
	for i := 0; i < limit; i++ {
		it := items[i]
		semantic := evidenceSemantic(it)
		if semantic != "" {
			fmt.Fprintf(&b, "  [%d] %s %s %s @ %s:%d — %s\n",
				i+1, label, it.Kind, prefOrDash(it.AnchorSymbol), it.Source, it.LineStart, semantic)
		} else {
			fmt.Fprintf(&b, "  [%d] %s %s %s @ %s:%d\n",
				i+1, label, it.Kind, prefOrDash(it.AnchorSymbol), it.Source, it.LineStart)
		}
	}
	if len(items) > max {
		fmt.Fprintf(&b, "  ... %d more %s item(s)\n", len(items)-max, label)
	}
	return b.String()
}

func emitEvidenceDuplicateNoopRepair(count int) *types.ToolRepair {
	return &types.ToolRepair{
		Code: EmitEvidenceDuplicateNoopCode,
		Hint: fmt.Sprintf(
			"All %d emit_evidence item(s) were exact duplicates of already-recorded structured evidence. Do not re-emit exact rows; if metadata is wrong, re-emit the same source/line/fact with corrected non-empty fields, otherwise emit only genuinely new/enriched evidence or close with emit_investigation_complete.",
			count,
		),
		Metadata: map[string]string{
			"progress":        "none",
			"duplicate_count": strconv.Itoa(count),
		},
	}
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
//	Current batch: G grounded / R recovered / U ungrounded.
//	Evidence buffer (audit, cumulative): G grounded / R recovered / U ungrounded across N files.
//	Current actionable repair targets: <current ToolRepair target list or none>.
//	Cumulative repair audit: R recovered / U ungrounded still visible in the buffer.
//
// allEvidence is the mutable buffer after Append, used for the global
// audit tally so the LLM sees cumulative state across multiple
// emit_evidence calls without mistaking stale covered rows for work
// that requires a full consolidated re-emit.
func renderEmitSummary(ctx *types.BusContext, items []types.EvidenceItem, reports []ground.Report, allEvidence []types.EvidenceItem, validationRepairFields ...string) string {
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
		if it.Salience.IsSet() {
			fmt.Fprintf(&b, "      salience: %s\n", it.Salience)
		}
		if form := types.ClaimFormOf(it); form != types.ClaimUnknown {
			fmt.Fprintf(&b, "      claim_form: %s\n", form)
		}
		if boundary := types.EvidenceMechanismAuthorityBoundary(it); boundary != "" {
			fmt.Fprintf(&b, "      source authority: %s\n", boundary)
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
				fmt.Fprintf(&b, "        repair: if line %d is the intended proof, re-emit with an anchor_kind/anchor_symbol visible on that line; if the recovered symbol definition is the proof, cite line %d instead.\n",
					r.OriginalLine, r.AdjustedLine)
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
				fmt.Fprintf(&b, "        fix: (A) if this line was not just inspected, read_file %s near line %d  (B) if the gutter confirms the same proof, re-emit with a visible anchor_symbol  (C) if the location/symbol is stale or wrong-file, emit a grounded replacement from the correct visible line or drop the item\n", it.Source, line)
			}
		}
	}
	batch := evidenceGroundingTally(items)
	cumulative := evidenceGroundingTally(allEvidence)
	currentTargets := emitEvidenceRepairTargets(ctx, items, reports)
	audit, auditOnly := cumulativeEmitEvidenceRepairAuditTally(allEvidence)
	fmt.Fprintf(&b, "\nCurrent batch: %d grounded / %d recovered / %d ungrounded.\n",
		batch.grounded, batch.recovered, batch.ungrounded)
	fmt.Fprintf(&b, "Evidence buffer (audit, cumulative): %d grounded / %d recovered / %d ungrounded across %d file(s).\n",
		cumulative.grounded, cumulative.recovered, cumulative.ungrounded, emitEvidenceSourceCount(allEvidence))
	validationRepairFields = uniqueEmitEvidenceRepairFields(validationRepairFields)
	if len(currentTargets) == 0 && len(validationRepairFields) == 0 {
		fmt.Fprintf(&b, "Current actionable repair targets: none. ")
		if batch.recovered > 0 || batch.ungrounded > 0 {
			b.WriteString("Current non-grounded rows were recovered, covered by grounded siblings, or marked non-actionable. ")
		}
	} else if len(currentTargets) > 0 {
		fmt.Fprintf(&b, "Current actionable repair targets: %s. Treat these as audit candidates: repair them when current source confirms the row; if the line proves stale or wrong-file, emit a grounded replacement or omit the row before widening scope. ",
			renderToolRepairTargetsInline(currentTargets, 3, 4))
		if len(validationRepairFields) > 0 {
			fmt.Fprintf(&b, "Item-field repair paths: %s. Re-emit only the named skipped or relation-authority-downgraded item(s); accepted siblings are already committed. ",
				strings.Join(validationRepairFields, ", "))
		}
	} else {
		fmt.Fprintf(&b, "Current actionable repair targets: %s. Re-emit only the named skipped or relation-authority-downgraded item(s) with these exact schema fields corrected; accepted siblings are already committed. ",
			strings.Join(validationRepairFields, ", "))
	}
	b.WriteString("Do not re-emit a full consolidated evidence set just to change the cumulative audit tally.\n")
	if audit.recovered != 0 || audit.ungrounded != 0 || auditOnly != 0 {
		fmt.Fprintf(&b, "Cumulative repair audit: %d recovered / %d ungrounded still visible in the buffer",
			audit.recovered, audit.ungrounded)
		if auditOnly > 0 {
			fmt.Fprintf(&b, "; %d covered/non-actionable cumulative row(s) omitted from repair guidance", auditOnly)
		}
		b.WriteString(". This is audit context; next-step repair guidance comes from current item rows above and structured ToolRepair targets only.\n")
	}
	if shouldNudgeDiagramRoleHints(ctx, items) {
		b.WriteString("Config-precedence task detected: when an evidence item represents code defaults, a config-file layer (YAML/JSON/TOML/INI/etc.), a runtime binding layer, or a high-precedence override layer, set `diagram_role_hint` on that item so diagram rendering can reuse validated structure instead of inferring roles from prose.\n")
	}
	return b.String()
}

// emitEvidenceAssignmentEndpointRepair is a producer-owned, exact-line repair
// recipe for one assignment/initializer whose model-authored Subject/Object do
// not match the unique syntax-authored LHS/RHS tuple. It is never evidence and
// never mutates the accepted item back into a directed relation.
type emitEvidenceAssignmentEndpointRepair struct {
	itemIndex int
	anchor    types.AnchorKind
	receiver  string
	value     string
	source    string
	line      int
}

// emitEvidenceValueTransferClassificationRepair is an exact-line recipe for
// a model-submitted definition/mechanism row whose already-read source line is
// actually one simple assignment/initializer touching a requested diagram
// participant. It remains a repair recipe rather than evidence: only an
// explicit corrected model emit can publish the directed relation row.
type emitEvidenceValueTransferClassificationRepair struct {
	itemIndex int
	anchor    types.AnchorKind
	receiver  string
	value     string
	source    string
	line      int
}

// emitEvidenceCallEndpointRepair is the sibling exact-line repair recipe for
// a call-shaped observation whose submitted fields did not preserve the
// parser-owned caller -> callee tuple. The downgraded observation stays
// citable text until the model explicitly re-emits this relationship.
type emitEvidenceCallEndpointRepair struct {
	itemIndex int
	caller    string
	callee    string
	source    string
	line      int
	// callbackReceiverPair distinguishes an additional direct-call row from
	// the ordinary case where a malformed call row itself was downgraded.  In
	// the paired case the callback handoff remains accepted and must not be
	// rebuilt or replaced.
	callbackReceiverPair bool
}

type emitEvidenceArgumentFlowRepair struct {
	itemIndex int
	argument  string
	receiver  string
	source    string
	line      int
	// assignmentCallCompanion records that the exact argument was recovered
	// from a unique call nested in an accepted assignment/initializer row,
	// rather than from a separately submitted AnchorCall row.
	assignmentCallCompanion bool
}

const emitEvidenceRelationRepairObligationsMetadataKey = "relation_repair_obligations_v1"

// emitEvidenceRelationRepairObligation is the durable, syntax-owned identity
// of one exact relation repair.  ToolResult order is not sufficient authority:
// a later successful but unrelated emit_evidence call must not make an earlier
// required relation repair disappear.  Completion checks these keys against
// the current typed evidence pool; it never scans request/model/answer prose.
type emitEvidenceRelationRepairObligation struct {
	EvidenceKind types.EvidenceKind `json:"evidence_kind,omitempty"`
	AnchorKind   types.AnchorKind   `json:"anchor_kind"`
	Source       string             `json:"source"`
	Line         int                `json:"line"`
	Subject      string             `json:"subject"`
	Object       string             `json:"object"`
}

func registrationBindingRepairObligations(in []emitEvidenceRegistrationBindingRepair) []emitEvidenceRelationRepairObligation {
	out := make([]emitEvidenceRelationRepairObligation, 0, len(in))
	for _, row := range in {
		out = append(out, emitEvidenceRelationRepairObligation{
			EvidenceKind: types.EvidenceRegistration,
			AnchorKind:   types.AnchorCall,
			Source:       row.source,
			Line:         row.line,
			Subject:      row.registry,
			Object:       row.boundObject,
		})
	}
	return out
}

func valueTransferClassificationRepairObligations(in []emitEvidenceValueTransferClassificationRepair) []emitEvidenceRelationRepairObligation {
	out := make([]emitEvidenceRelationRepairObligation, 0, len(in))
	for _, row := range in {
		out = append(out, emitEvidenceRelationRepairObligation{
			AnchorKind: row.anchor,
			Source:     row.source,
			Line:       row.line,
			Subject:    row.receiver,
			Object:     row.value,
		})
	}
	return out
}

func assignmentEndpointRepairObligations(in []emitEvidenceAssignmentEndpointRepair) []emitEvidenceRelationRepairObligation {
	out := make([]emitEvidenceRelationRepairObligation, 0, len(in))
	for _, row := range in {
		out = append(out, emitEvidenceRelationRepairObligation{
			AnchorKind: row.anchor,
			Source:     row.source,
			Line:       row.line,
			Subject:    row.receiver,
			Object:     row.value,
		})
	}
	return out
}

func callEndpointRepairObligations(in []emitEvidenceCallEndpointRepair) []emitEvidenceRelationRepairObligation {
	out := make([]emitEvidenceRelationRepairObligation, 0, len(in))
	for _, row := range in {
		out = append(out, emitEvidenceRelationRepairObligation{
			AnchorKind: types.AnchorCall,
			Source:     row.source,
			Line:       row.line,
			Subject:    row.caller,
			Object:     row.callee,
		})
	}
	return out
}

func argumentFlowRepairObligations(in []emitEvidenceArgumentFlowRepair) []emitEvidenceRelationRepairObligation {
	out := make([]emitEvidenceRelationRepairObligation, 0, len(in))
	for _, row := range in {
		out = append(out, emitEvidenceRelationRepairObligation{
			EvidenceKind: types.EvidenceRelationship,
			AnchorKind:   types.AnchorArgument,
			Source:       row.source,
			Line:         row.line,
			Subject:      row.argument,
			Object:       row.receiver,
		})
	}
	return out
}

func encodeEmitEvidenceRelationRepairObligations(in []emitEvidenceRelationRepairObligation) string {
	if len(in) == 0 {
		return ""
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return ""
	}
	return string(raw)
}

func decodeEmitEvidenceRelationRepairObligations(raw string) ([]emitEvidenceRelationRepairObligation, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	var out []emitEvidenceRelationRepairObligation
	if json.Unmarshal([]byte(raw), &out) != nil || len(out) == 0 {
		return nil, false
	}
	for _, row := range out {
		if row.AnchorKind == "" || strings.TrimSpace(row.Source) == "" || row.Line <= 0 ||
			strings.TrimSpace(row.Subject) == "" || strings.TrimSpace(row.Object) == "" {
			return nil, false
		}
	}
	return out, true
}

func mergeEmitEvidenceRelationRepairObligationMetadata(dst, src *types.ToolRepair) {
	if dst == nil || src == nil || src.Metadata == nil {
		return
	}
	if dst.Metadata == nil {
		dst.Metadata = make(map[string]string)
	}
	left, _ := decodeEmitEvidenceRelationRepairObligations(dst.Metadata[emitEvidenceRelationRepairObligationsMetadataKey])
	right, _ := decodeEmitEvidenceRelationRepairObligations(src.Metadata[emitEvidenceRelationRepairObligationsMetadataKey])
	if len(right) == 0 {
		return
	}
	dst.Metadata[emitEvidenceRelationRepairObligationsMetadataKey] = encodeEmitEvidenceRelationRepairObligations(append(left, right...))
}

// emitEvidenceRequiredFlowAssignmentEndpointRepair promotes an already exact
// parser diagnosis into structured repair debt only when a required current-
// source flow diagram needs directed operation rows. Optional diagrams and
// Trace/root-cause lanes retain the ordinary non-blocking text-reference
// downgrade. The predicate is entirely typed; it does not inspect request,
// reasoning, summary, or final-answer prose.
func emitEvidenceRequiredFlowAssignmentEndpointRepair(ctx *types.BusContext, item types.EvidenceItem, itemIndex int) (emitEvidenceAssignmentEndpointRepair, bool) {
	if ctx == nil || ctx.AnalysisIR == nil {
		return emitEvidenceAssignmentEndpointRepair{}, false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.Intent == types.IntentTrace || types.ResolveQuestionFamily(rm) == types.QFRootCauseTrace ||
		rm.PredicateAxis != types.AxisFlow || rm.DiagramHint == nil || !rm.DiagramHint.Required {
		return emitEvidenceAssignmentEndpointRepair{}, false
	}
	if item.Scope != types.ScopeLine ||
		(item.AnchorKind != types.AnchorAssignment && item.AnchorKind != types.AnchorInitializer) ||
		(item.GroundingStatus != types.GroundingGrounded && item.GroundingStatus != types.GroundingRecovered) {
		return emitEvidenceAssignmentEndpointRepair{}, false
	}
	receiver, value, ok := types.AssignmentEvidenceEndpoints(item)
	if !ok || types.AssignmentEvidenceEndpointsMatch(item) {
		return emitEvidenceAssignmentEndpointRepair{}, false
	}
	return emitEvidenceAssignmentEndpointRepair{
		itemIndex: itemIndex,
		anchor:    item.AnchorKind,
		receiver:  receiver,
		value:     value,
		source:    item.Source,
		line:      item.LineStart,
	}, true
}

func emitEvidenceRequiredFlowValueTransferClassificationRepair(
	ctx *types.BusContext,
	item types.EvidenceItem,
	itemIndex int,
	gc *ground.Context,
) (emitEvidenceValueTransferClassificationRepair, bool) {
	if ctx == nil || ctx.AnalysisIR == nil || gc == nil || item.Scope != types.ScopeLine || item.LineStart <= 0 ||
		(item.GroundingStatus != types.GroundingGrounded && item.GroundingStatus != types.GroundingRecovered) {
		return emitEvidenceValueTransferClassificationRepair{}, false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.Intent == types.IntentTrace || types.ResolveQuestionFamily(rm) == types.QFRootCauseTrace ||
		rm.PredicateAxis != types.AxisFlow || rm.DiagramHint == nil || !rm.DiagramHint.Required {
		return emitEvidenceValueTransferClassificationRepair{}, false
	}
	line := strings.TrimSpace(evidenceVisibleLineText(gc, item.Source, item.LineStart))
	if line == "" {
		return emitEvidenceValueTransferClassificationRepair{}, false
	}
	// When the parser has an unambiguous value-transfer shape, it owns the
	// assignment-vs-member-initializer distinction. A lexically grounded row
	// can otherwise be accepted with anchor_kind=assignment on a Go/ArkTS/C
	// member initializer (or the inverse), yet fail every downstream flow
	// authority check without telling the model why. Publish the exact shape as
	// repair debt; never rewrite the accepted row or mint the relation here.
	if anchor, receiver, value, ok := exactASTValueTransferTuple(gc, item.Source, item.LineStart, line); ok {
		if item.AnchorKind == anchor {
			return emitEvidenceValueTransferClassificationRepair{}, false
		}
		if !requiredFlowTransferTouchesIncidentParticipant(rm, receiver, value) {
			return emitEvidenceValueTransferClassificationRepair{}, false
		}
		return emitEvidenceValueTransferClassificationRepair{
			itemIndex: itemIndex,
			anchor:    anchor,
			receiver:  receiver,
			value:     value,
			source:    item.Source,
			line:      item.LineStart,
		}, true
	}
	// Without a parser-owned shape, assignment/initializer rows stay in the
	// existing endpoint-validation lane. Guessing an alternate anchor kind from
	// line text would turn a noisy fallback into a completion-blocking gate.
	if item.AnchorKind == types.AnchorAssignment || item.AnchorKind == types.AnchorInitializer {
		return emitEvidenceValueTransferClassificationRepair{}, false
	}
	candidate := item
	candidate.Scope = types.ScopeLine
	candidate.Snippet = line
	candidate.AnchorKind = types.AnchorAssignment
	receiver, value, ok := types.AssignmentEvidenceEndpoints(candidate)
	anchor := types.AnchorAssignment
	if !ok {
		candidate.AnchorKind = types.AnchorInitializer
		receiver, value, ok = types.AssignmentEvidenceEndpoints(candidate)
		anchor = types.AnchorInitializer
	}
	if !ok || !requiredFlowTransferTouchesIncidentParticipant(rm, receiver, value) {
		return emitEvidenceValueTransferClassificationRepair{}, false
	}
	return emitEvidenceValueTransferClassificationRepair{
		itemIndex: itemIndex,
		anchor:    anchor,
		receiver:  receiver,
		value:     value,
		source:    item.Source,
		line:      item.LineStart,
	}, true
}

// exactASTValueTransferTuple returns one parser-owned value-transfer shape
// only when the indexed line has exactly one of the two mutually exclusive AST
// features. The conservative line parser then supplies the unique endpoints.
// Ambiguous/missing AST, unsupported syntax, and fallback-tier files return no
// signal so they can never drive a hard repair.
func exactASTValueTransferTuple(
	gc *ground.Context,
	source string,
	lineNumber int,
	lineText string,
) (anchor types.AnchorKind, receiver, value string, ok bool) {
	graph, _, graphSource, _, found := ground.ResolveSourceGraphFile(gc, source)
	if !found || graph == nil || lineNumber <= 0 || strings.TrimSpace(lineText) == "" {
		return "", "", "", false
	}
	var assignment, initializer bool
	for _, feature := range graph.LineFeaturesAt(graphSource, lineNumber) {
		switch feature {
		case repomap.LineFeatureAssignment:
			assignment = true
		case repomap.LineFeatureMemberInitializer:
			initializer = true
		}
	}
	if assignment == initializer { // both false or both true: no unique shape
		return "", "", "", false
	}
	anchor = types.AnchorAssignment
	if initializer {
		anchor = types.AnchorInitializer
	}
	candidate := types.EvidenceItem{AnchorKind: anchor, Snippet: strings.TrimSpace(lineText)}
	receiver, value, ok = types.AssignmentEvidenceEndpoints(candidate)
	if !ok {
		return "", "", "", false
	}
	return anchor, receiver, value, true
}

func requiredFlowTransferTouchesIncidentParticipant(rm types.RequestModel, receiver, value string) bool {
	if rm.DiagramHint == nil {
		return false
	}
	for _, participant := range rm.DiagramHint.Participants {
		if participant.Role != types.DiagramParticipantIncidentRequired {
			continue
		}
		surfaces := []string{strings.TrimSpace(participant.Identity)}
		surfaces = append(surfaces, types.DiagramParticipantIdentitySurfaces(rm, participant)...)
		for _, surface := range surfaces {
			if surface == "" {
				continue
			}
			if types.AnswerCodeIdentitySurfacesCompatible(surface, receiver) ||
				types.AnswerCodeIdentitySurfacesCompatible(surface, value) ||
				types.AnswerCodeSurfaceAppearsInText(receiver, surface) ||
				types.AnswerCodeSurfaceAppearsInText(value, surface) {
				return true
			}
		}
	}
	return false
}

func buildEmitEvidenceValueTransferClassificationRepair(in []emitEvidenceValueTransferClassificationRepair) *types.ToolRepair {
	if len(in) == 0 {
		return nil
	}
	fields := make([]string, 0, len(in)*6)
	rows := make([]string, 0, len(in))
	for _, row := range in {
		fields = append(fields,
			fmt.Sprintf("items[%d].evidence_kind", row.itemIndex),
			fmt.Sprintf("items[%d].anchor_kind", row.itemIndex),
			fmt.Sprintf("items[%d].anchor_symbol", row.itemIndex),
			fmt.Sprintf("items[%d].subject", row.itemIndex),
			fmt.Sprintf("items[%d].predicate", row.itemIndex),
			fmt.Sprintf("items[%d].object", row.itemIndex),
		)
		loc := row.source
		if row.line > 0 {
			loc = fmt.Sprintf("%s:%d", row.source, row.line)
		}
		rows = append(rows, fmt.Sprintf(
			"items[%d] @ %s requires evidence_kind=%q, anchor_kind=%q, anchor_symbol=%q, subject=%q (exact LHS receiver), predicate=%q, and object=%q (exact RHS value/source)",
			row.itemIndex, loc, string(types.EvidenceRelationship), string(row.anchor), row.receiver,
			row.receiver, "assigns", row.value,
		))
	}
	return &types.ToolRepair{
		Code:   types.ToolRepairCodeEvidenceItemValidation,
		Hint:   "The required source-flow diagram already has an exact value-transfer line, but the submitted row used a non-authoritative evidence or source-shape classification and therefore published no directed relation authority. Re-emit only the named item(s) with the parser-owned fields below; accepted context rows remain committed. The system is not promoting the row or drawing an edge: " + strings.Join(rows, "; "),
		Fields: uniqueEmitEvidenceRepairFields(fields),
		Metadata: map[string]string{
			"repair_status":       types.ToolRepairStatusActionRequired,
			"repair_scope":        "value_transfer_classification",
			"repair_stage":        "explorer",
			"completion_blocking": "true",
			emitEvidenceRelationRepairObligationsMetadataKey: encodeEmitEvidenceRelationRepairObligations(
				valueTransferClassificationRepairObligations(in)),
		},
	}
}

func buildEmitEvidenceAssignmentEndpointRepair(in []emitEvidenceAssignmentEndpointRepair) *types.ToolRepair {
	if len(in) == 0 {
		return nil
	}
	fields := make([]string, 0, len(in)*2)
	rows := make([]string, 0, len(in))
	for _, row := range in {
		fields = append(fields,
			fmt.Sprintf("items[%d].subject", row.itemIndex),
			fmt.Sprintf("items[%d].object", row.itemIndex),
		)
		loc := row.source
		if row.line > 0 {
			loc = fmt.Sprintf("%s:%d", row.source, row.line)
		}
		rows = append(rows, fmt.Sprintf(
			"items[%d] @ %s: keep anchor_kind=%q and re-emit subject=%q (exact LHS receiver) and object=%q (exact RHS value/source)",
			row.itemIndex, loc, string(row.anchor), row.receiver, row.value,
		))
	}
	fields = uniqueEmitEvidenceRepairFields(fields)
	return &types.ToolRepair{
		Code:   types.ToolRepairCodeEvidenceItemValidation,
		Hint:   "The required source-flow diagram still needs an exact directed operation row. The submitted assignment/initializer remained citable text, but its semantic endpoints were broader than the unique source tuple, so relation authority was removed. Re-emit only the named item(s), keeping the listed anchor_kind plus the same evidence_kind, anchor_symbol, predicate, source, line_start, scope, and snippet; change only subject/object to the exact fields below. Do not rebuild accepted siblings or rename endpoints to stage/component roles: " + strings.Join(rows, "; "),
		Fields: fields,
		Metadata: map[string]string{
			"repair_status":       types.ToolRepairStatusActionRequired,
			"repair_scope":        "assignment_endpoint_identity",
			"repair_stage":        "explorer",
			"completion_blocking": "true",
			emitEvidenceRelationRepairObligationsMetadataKey: encodeEmitEvidenceRelationRepairObligations(
				assignmentEndpointRepairObligations(in)),
		},
	}
}

func emitEvidenceRequiredRelationCallEndpointRepair(
	ctx *types.BusContext,
	item types.EvidenceItem,
	itemIndex int,
	caller, callee string,
	exactKnown bool,
) (emitEvidenceCallEndpointRepair, bool) {
	if ctx == nil || ctx.AnalysisIR == nil || !exactKnown || caller == "" || callee == "" {
		return emitEvidenceCallEndpointRepair{}, false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.Intent == types.IntentTrace || types.ResolveQuestionFamily(rm) == types.QFRootCauseTrace ||
		!emitEvidenceExactCallRelationRepairRequestShape(rm) {
		return emitEvidenceCallEndpointRepair{}, false
	}
	if item.Scope != types.ScopeLine || item.AnchorKind != types.AnchorCall || item.LineStart <= 0 ||
		(item.GroundingStatus != types.GroundingGrounded && item.GroundingStatus != types.GroundingRecovered) {
		return emitEvidenceCallEndpointRepair{}, false
	}
	return emitEvidenceCallEndpointRepair{
		itemIndex: itemIndex,
		caller:    caller,
		callee:    callee,
		source:    item.Source,
		line:      item.LineStart,
	}, true
}

// emitEvidenceRequiredCallbackReceiverCallRepair exposes the direct-call half
// of an exact callback expression as model-owned repair debt.  A line such as
//
//	await loop.run_in_executor(None, plugin.handle, payload)
//
// proves both run_pipeline -> loop.run_in_executor (call) and
// loop.run_in_executor -> plugin.handle (callback handoff).  Callback
// normalization intentionally publishes only the second row.  When the
// question's typed schema requires a call/flow relation, this helper asks the
// model to emit the first row too so a principal path cannot be split into two
// disconnected components.
//
// The gate reads only the request schema, the normalized typed evidence row,
// and a unique parser/read-line tuple.  It does not inspect user/model/final
// prose, and Runtime Trace stays on its independent causal-evidence contract.
func emitEvidenceRequiredCallbackReceiverCallRepair(
	ctx *types.BusContext,
	item types.EvidenceItem,
	itemIndex int,
	gc *ground.Context,
) (emitEvidenceCallEndpointRepair, bool) {
	if ctx == nil || ctx.AnalysisIR == nil || gc == nil || item.AnchorKind != types.AnchorCallback ||
		item.Scope != types.ScopeLine || item.LineStart <= 0 ||
		(item.GroundingStatus != types.GroundingGrounded && item.GroundingStatus != types.GroundingRecovered) {
		return emitEvidenceCallEndpointRepair{}, false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.Intent == types.IntentTrace || types.ResolveQuestionFamily(rm) == types.QFRootCauseTrace ||
		!emitEvidenceExactCallRelationRepairRequestShape(rm) {
		return emitEvidenceCallEndpointRepair{}, false
	}
	receiver, callable, ok := ground.DetectCallbackHandoffAtLine(
		gc, item.Source, item.LineStart, firstNonEmpty(item.AnchorSymbol, item.Object),
	)
	if !ok || !emitEndpointIdentityCompatible(item.Subject, receiver) ||
		!emitEndpointIdentityCompatible(item.Object, callable) {
		return emitEvidenceCallEndpointRepair{}, false
	}

	// Reuse the same exact caller/callee resolver as ordinary call evidence,
	// but target the receiving invocation instead of the callback argument.
	// Clearing Subject prevents the already-normalized receiver identity from
	// being considered as a candidate caller.
	direct := item
	direct.AnchorKind = types.AnchorCall
	direct.Subject = ""
	direct.Predicate = "calls"
	direct.Object = receiver
	direct.AnchorSymbol = receiver
	caller, callee, exactKnown := exactCallEvidenceDirection(&direct, gc)
	if !exactKnown || strings.TrimSpace(caller) == "" || strings.TrimSpace(callee) == "" {
		return emitEvidenceCallEndpointRepair{}, false
	}
	return emitEvidenceCallEndpointRepair{
		itemIndex:            itemIndex,
		caller:               caller,
		callee:               callee,
		source:               item.Source,
		line:                 item.LineStart,
		callbackReceiverPair: true,
	}, true
}

// filterSatisfiedCallbackReceiverCallRepairs avoids asking for an additional
// row when the model already emitted that exact direct call in the same or an
// earlier batch.  Ordinary downgraded-call repairs retain their existing
// behavior.  Satisfaction uses the same durable typed obligation matcher as
// emit_investigation_complete, never prose similarity.
func filterSatisfiedCallbackReceiverCallRepairs(
	in []emitEvidenceCallEndpointRepair,
	evidence []types.EvidenceItem,
) []emitEvidenceCallEndpointRepair {
	if len(in) == 0 {
		return nil
	}
	out := make([]emitEvidenceCallEndpointRepair, 0, len(in))
	for _, row := range in {
		if row.callbackReceiverPair && emitEvidenceRelationRepairObligationsSatisfied(
			callEndpointRepairObligations([]emitEvidenceCallEndpointRepair{row}), evidence,
		) {
			continue
		}
		out = append(out, row)
	}
	return out
}

// emitEvidenceRequiredCallArgumentFlowRepairs joins three exact typed facts:
// a grounded direct-call row, the parser's complete argument roster for that
// same invocation, and a static declaration binding that aligns one argument
// with an incident-required diagram participant. The join only creates repair
// debt. It never appends evidence, selects a diagram edge, or interprets user
// or answer prose.
func emitEvidenceRequiredCallArgumentFlowRepairs(
	ctx *types.BusContext,
	item types.EvidenceItem,
	itemIndex int,
	gc *ground.Context,
) []emitEvidenceArgumentFlowRepair {
	if ctx == nil || ctx.AnalysisIR == nil || gc == nil || item.Scope != types.ScopeLine ||
		item.AnchorKind != types.AnchorCall || item.LineStart <= 0 ||
		(item.GroundingStatus != types.GroundingGrounded && item.GroundingStatus != types.GroundingRecovered) {
		return nil
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.Intent == types.IntentTrace || types.ResolveQuestionFamily(rm) == types.QFRootCauseTrace ||
		rm.PredicateAxis != types.AxisFlow || rm.DiagramHint == nil || !rm.DiagramHint.Required {
		return nil
	}
	_, callee, exact := exactCallEvidenceDirection(&item, gc)
	if !exact || strings.TrimSpace(callee) == "" {
		return nil
	}
	return emitEvidenceArgumentFlowRepairsForExactCall(ctx, item, itemIndex, gc, callee, false)
}

// emitEvidenceRequiredAssignmentCallArgumentFlowRepairs provides the same
// exact argument-flow repair for a line submitted as assignment/initializer.
// A source line can legitimately carry both shapes; requiring the model to
// guess AnchorCall first made the available relation evidence depend on a
// schema-label choice.  The unique-call requirement is fail closed when a
// line contains nested/sibling calls.
func emitEvidenceRequiredAssignmentCallArgumentFlowRepairs(
	ctx *types.BusContext,
	item types.EvidenceItem,
	itemIndex int,
	gc *ground.Context,
) []emitEvidenceArgumentFlowRepair {
	if ctx == nil || ctx.AnalysisIR == nil || gc == nil || item.Scope != types.ScopeLine ||
		(item.AnchorKind != types.AnchorAssignment && item.AnchorKind != types.AnchorInitializer) ||
		item.LineStart <= 0 ||
		(item.GroundingStatus != types.GroundingGrounded && item.GroundingStatus != types.GroundingRecovered) {
		return nil
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.Intent == types.IntentTrace || types.ResolveQuestionFamily(rm) == types.QFRootCauseTrace ||
		rm.PredicateAxis != types.AxisFlow || rm.DiagramHint == nil || !rm.DiagramHint.Required {
		return nil
	}
	_, callee, exact := exactUniqueCallEvidenceDirectionAtLine(item, gc)
	if !exact || strings.TrimSpace(callee) == "" {
		return nil
	}
	return emitEvidenceArgumentFlowRepairsForExactCall(ctx, item, itemIndex, gc, callee, true)
}

func emitEvidenceArgumentFlowRepairsForExactCall(
	ctx *types.BusContext,
	item types.EvidenceItem,
	itemIndex int,
	gc *ground.Context,
	callee string,
	assignmentCallCompanion bool,
) []emitEvidenceArgumentFlowRepair {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	rm := ctx.AnalysisIR.RequestModel
	flows := ground.DetectArgumentFlowsAtLine(gc, item.Source, item.LineStart, callee)
	if len(flows) == 0 {
		return nil
	}
	out := make([]emitEvidenceArgumentFlowRepair, 0, len(flows))
	seenStageParticipant := make(map[string]bool)
	stagePrecedence := emitEvidenceVerifiedStageArgumentPrecedence(ctx, rm)
	for _, flow := range flows {
		candidate := item
		candidate.Kind = types.EvidenceRelationship
		candidate.AnchorKind = types.AnchorArgument
		candidate.AnchorSymbol = flow.Argument
		candidate.Subject = flow.Argument
		candidate.Predicate = "passes argument"
		candidate.Object = callee
		candidate.DeclaredBinding = ""
		candidate.DeclaredType = ""
		candidate.DeclaredOwner = ""
		candidate.DeclaredIdentityBindings = nil
		stampEvidenceTypedIdentityBindings(&candidate, gc)
		declaredParticipant := argumentFlowCandidateTouchesIncidentParticipant(rm, candidate)
		stageParticipant := argumentFlowCandidateVerifiedStageParticipant(rm, candidate.Subject, stagePrecedence)
		if !declaredParticipant && stageParticipant == "" {
			continue
		}
		if !declaredParticipant && seenStageParticipant[stageParticipant] {
			continue
		}
		if stageParticipant != "" {
			seenStageParticipant[stageParticipant] = true
		}
		out = append(out, emitEvidenceArgumentFlowRepair{
			itemIndex:               itemIndex,
			argument:                flow.Argument,
			receiver:                callee,
			source:                  item.Source,
			line:                    item.LineStart,
			assignmentCallCompanion: assignmentCallCompanion,
		})
	}
	return out
}

func emitEvidenceVerifiedStageArgumentPrecedence(ctx *types.BusContext, rm types.RequestModel) []stageauthority.PrecedenceRelation {
	if ctx == nil || rm.DiagramHint == nil {
		return nil
	}
	authority, ok := stageauthority.LoadReadMode(ctx.RepoRoot)
	if !ok {
		return nil
	}
	var evidence []types.EvidenceItem
	if ctx.Mutable != nil {
		evidence = ctx.Mutable.EmittedEvidence()
	}
	return stageauthority.SelectRequiredReadModeWorkflow(rm, evidence, authority).Precedence
}

func argumentFlowCandidateVerifiedStageParticipant(
	rm types.RequestModel,
	endpoint string,
	precedence []stageauthority.PrecedenceRelation,
) string {
	if rm.DiagramHint == nil || len(precedence) == 0 {
		return ""
	}
	matched := ""
	for _, participant := range rm.DiagramHint.Participants {
		if stageauthority.ParticipantMatchesStageEndpoint(rm, participant, endpoint, precedence) {
			identity := strings.ToLower(strings.TrimSpace(participant.Identity))
			if identity == "" || (matched != "" && matched != identity) {
				return ""
			}
			matched = identity
		}
	}
	return matched
}

func argumentFlowCandidateTouchesIncidentParticipant(rm types.RequestModel, candidate types.EvidenceItem) bool {
	if rm.DiagramHint == nil || len(candidate.DeclaredIdentityBindings) == 0 {
		return false
	}
	for _, participant := range rm.DiagramHint.Participants {
		if participant.Role != types.DiagramParticipantIncidentRequired {
			continue
		}
		surfaces := []string{strings.TrimSpace(participant.Identity)}
		surfaces = append(surfaces, types.DiagramParticipantIdentitySurfaces(rm, participant)...)
		for _, surface := range surfaces {
			if types.AnswerCodeIdentityIncidentViaDeclaredBinding(
				surface, candidate.Subject, candidate, nil,
			) {
				return true
			}
		}
	}
	return false
}

func filterSatisfiedArgumentFlowRepairs(
	in []emitEvidenceArgumentFlowRepair,
	evidence []types.EvidenceItem,
) []emitEvidenceArgumentFlowRepair {
	if len(in) == 0 {
		return nil
	}
	out := make([]emitEvidenceArgumentFlowRepair, 0, len(in))
	for _, row := range in {
		if emitEvidenceRelationRepairObligationsSatisfied(
			argumentFlowRepairObligations([]emitEvidenceArgumentFlowRepair{row}), evidence,
		) {
			continue
		}
		out = append(out, row)
	}
	return out
}

// filterSatisfiedRegistrationBindingRepairs keeps a citable container row
// separate from its exact binding expression while avoiding redundant debt
// when the model already emitted that expression in the same or an earlier
// batch. Satisfaction is the durable typed source/line/endpoint tuple, never
// request, reasoning, summary, or final-answer prose.
func filterSatisfiedRegistrationBindingRepairs(
	in []emitEvidenceRegistrationBindingRepair,
	evidence []types.EvidenceItem,
) []emitEvidenceRegistrationBindingRepair {
	if len(in) == 0 {
		return nil
	}
	out := make([]emitEvidenceRegistrationBindingRepair, 0, len(in))
	for _, row := range in {
		if emitEvidenceRelationRepairObligationsSatisfied(
			registrationBindingRepairObligations([]emitEvidenceRegistrationBindingRepair{row}), evidence,
		) {
			continue
		}
		out = append(out, row)
	}
	return out
}

func buildEmitEvidenceArgumentFlowRepair(in []emitEvidenceArgumentFlowRepair) *types.ToolRepair {
	if len(in) == 0 {
		return nil
	}
	fields := make([]string, 0, len(in))
	rows := make([]string, 0, len(in))
	assignmentCompanionCount := 0
	for _, row := range in {
		fields = append(fields, "items")
		loc := row.source
		if row.line > 0 {
			loc = fmt.Sprintf("%s:%d", row.source, row.line)
		}
		origin := fmt.Sprintf("the direct call from items[%d] @ %s is accepted", row.itemIndex, loc)
		if row.assignmentCallCompanion {
			assignmentCompanionCount++
			origin = fmt.Sprintf("the assignment/initializer from items[%d] @ %s contains one unique parser-owned call", row.itemIndex, loc)
		}
		rows = append(rows, fmt.Sprintf(
			"%s; emit one additional item with scope=%q, evidence_kind=%q, source=%q, line_start=%d, anchor_kind=%q, anchor_symbol=%q, subject=%q, predicate=%q, and object=%q",
			origin, string(types.ScopeLine), string(types.EvidenceRelationship), row.source, row.line,
			string(types.AnchorArgument), row.argument, row.argument, "passes argument", row.receiver,
		))
	}
	scope := "call_argument_flow_pair"
	if assignmentCompanionCount == len(in) {
		scope = "assignment_call_argument_flow_pair"
	} else if assignmentCompanionCount > 0 {
		scope += "+assignment_call_argument_flow_pair"
	}
	return &types.ToolRepair{
		Code:   types.ToolRepairCodeEvidenceItemValidation,
		Hint:   "An exact operation site contains a complete data argument whose parser-owned static type matches an incident-required carrier participant. Preserve the separate argument -> receiving-API handoff by emitting only the additional row(s) below. The accepted source observation stays unchanged, and the system has not created evidence or drawn an edge: " + strings.Join(rows, "; "),
		Fields: uniqueEmitEvidenceRepairFields(fields),
		Metadata: map[string]string{
			"repair_status":       types.ToolRepairStatusActionRequired,
			"repair_scope":        scope,
			"repair_stage":        "explorer",
			"completion_blocking": "true",
			emitEvidenceRelationRepairObligationsMetadataKey: encodeEmitEvidenceRelationRepairObligations(
				argumentFlowRepairObligations(in)),
		},
	}
}

// emitEvidenceExactCallRelationRepairRequestShape selects source questions for
// which a model-submitted call-shaped observation is itself part of the typed
// relation surface.  A required diagram is not the only consumer: registration,
// configuration, and non-diagram call/flow explanations also need the exact
// parser-owned caller -> callee tuple or they can finish with text references
// while losing the relationship the user asked about.
//
// This is deliberately schema-only.  It does not inspect the request, evidence
// summary, or answer prose, and it does not create an edge.  The repair is
// offered only after the model has already submitted a line-scoped AnchorCall
// and exactCallEvidenceDirection has found one unique source tuple.  Condition,
// implementation, definition, and return questions keep incidental calls
// non-blocking.  Runtime Trace remains isolated by the caller above.
func emitEvidenceExactCallRelationRepairRequestShape(rm types.RequestModel) bool {
	switch rm.PredicateAxis {
	case types.AxisCall, types.AxisRegister, types.AxisConfigure, types.AxisFlow:
		return true
	}
	switch types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) {
	case types.ReqCallChain, types.ReqRegistration, types.ReqConfigMapping:
		return true
	default:
		return false
	}
}

func buildEmitEvidenceCallEndpointRepair(in []emitEvidenceCallEndpointRepair) *types.ToolRepair {
	if len(in) == 0 {
		return nil
	}
	fields := make([]string, 0, len(in)*5)
	rows := make([]string, 0, len(in))
	callbackPairCount := 0
	for _, row := range in {
		loc := row.source
		if row.line > 0 {
			loc = fmt.Sprintf("%s:%d", row.source, row.line)
		}
		if row.callbackReceiverPair {
			callbackPairCount++
			fields = append(fields, "items")
			inputTarget := emitCallRepairInputTarget(row.callee)
			rows = append(rows, fmt.Sprintf(
				"the callback handoff from items[%d] @ %s was accepted; emit one additional item with evidence_kind=%q, anchor_kind=%q, subject=%q (exact enclosing caller), predicate=%q, object=%q (exact parser call token; parser validation restores the unique qualified callee), and anchor_symbol=%q",
				row.itemIndex, loc, string(types.EvidenceRelationship), string(types.AnchorCall),
				row.caller, "calls", inputTarget, inputTarget,
			))
			continue
		}
		fields = append(fields,
			fmt.Sprintf("items[%d].evidence_kind", row.itemIndex),
			fmt.Sprintf("items[%d].subject", row.itemIndex),
			fmt.Sprintf("items[%d].predicate", row.itemIndex),
			fmt.Sprintf("items[%d].object", row.itemIndex),
			fmt.Sprintf("items[%d].anchor_symbol", row.itemIndex),
		)
		inputTarget := emitCallRepairInputTarget(row.callee)
		rows = append(rows, fmt.Sprintf(
			"items[%d] @ %s requires evidence_kind=%q, subject=%q (exact caller), predicate=%q, object=%q (exact parser call token; parser validation restores the unique qualified callee %q), and anchor_symbol=%q",
			row.itemIndex, loc, string(types.EvidenceRelationship), row.caller, "calls", inputTarget, row.callee, inputTarget,
		))
	}
	hint := "The typed source relationship still needs an exact directed call row. The submitted call-shaped observation remained citable text, but its caller/callee fields did not preserve the unique parser-owned invocation, so call-edge authority was removed. Re-emit only the named item(s), copying the same source/line/snippet and the exact fields below; do not rebuild accepted siblings or rename endpoints to stage/component roles: "
	scope := "call_endpoint_identity"
	if callbackPairCount > 0 {
		hint = "An exact callback source line carries two distinct typed relationships. Its receiving-API -> callable callback handoff is already accepted; the requested call path also needs the independently proven enclosing-caller -> receiving-API invocation. Emit only the additional direct-call item(s) described below and keep the accepted callback row unchanged. The system has not created this sibling edge: "
		scope = "callback_receiver_call_pair"
		if callbackPairCount != len(in) {
			hint += "This batch also contains an ordinary downgraded call row that must be re-emitted with its exact parser tuple. "
			scope = "call_endpoint_identity+callback_receiver_call_pair"
		}
	}
	return &types.ToolRepair{
		Code:   types.ToolRepairCodeEvidenceItemValidation,
		Hint:   hint + strings.Join(rows, "; "),
		Fields: uniqueEmitEvidenceRepairFields(fields),
		Metadata: map[string]string{
			"repair_status":       types.ToolRepairStatusActionRequired,
			"repair_scope":        scope,
			"repair_stage":        "explorer",
			"completion_blocking": "true",
			emitEvidenceRelationRepairObligationsMetadataKey: encodeEmitEvidenceRelationRepairObligations(
				callEndpointRepairObligations(in)),
		},
	}
}

// emitCallRepairInputTarget returns the source-level call token that the
// line grounder accepts as an unambiguous anchor. The durable repair
// obligation deliberately keeps the fully qualified parser-owned callee; an
// exact re-emit is normalized back to that canonical identity before it can
// satisfy completion. Asking the model to copy the qualified receiver into
// object/anchor_symbol is both unnecessary and, for nested selectors such as
// ctx.Mutable.EmittedAnswerSymbols, can make an otherwise exact call look like
// a semantic endpoint and be downgraded to text_reference.
func emitCallRepairInputTarget(callee string) string {
	callee = strings.TrimSpace(callee)
	if tail := emitLastDotSegment(callee); tail != "" {
		return tail
	}
	return callee
}

func buildEmitEvidenceRegistrationBindingRepair(in []emitEvidenceRegistrationBindingRepair) *types.ToolRepair {
	if len(in) == 0 {
		return nil
	}
	fields := make([]string, 0, len(in)*8)
	rows := make([]string, 0, len(in))
	for _, row := range in {
		fields = append(fields,
			fmt.Sprintf("items[%d].evidence_kind", row.itemIndex),
			fmt.Sprintf("items[%d].source", row.itemIndex),
			fmt.Sprintf("items[%d].line_start", row.itemIndex),
			fmt.Sprintf("items[%d].anchor_kind", row.itemIndex),
			fmt.Sprintf("items[%d].anchor_symbol", row.itemIndex),
			fmt.Sprintf("items[%d].subject", row.itemIndex),
			fmt.Sprintf("items[%d].predicate", row.itemIndex),
			fmt.Sprintf("items[%d].object", row.itemIndex),
		)
		rows = append(rows, fmt.Sprintf(
			"items[%d] requires evidence_kind=%q, source=%q, line_start=%d, anchor_kind=%q, anchor_symbol=%q, subject=%q (binding receiver/slot), predicate=%q, and object=%q (complete bound argument expression)",
			row.itemIndex, string(types.EvidenceRegistration), row.source, row.line,
			string(types.AnchorCall), row.anchor, row.registry, "registers", row.boundObject,
		))
	}
	return &types.ToolRepair{
		Code:   types.ToolRepairCodeEvidenceItemValidation,
		Hint:   "A load-bearing registration row cited a nearby definition or wrapper, but an already-read source line contains one unique receiver-call argument that binds the same endpoint. Re-emit only the named registration item with the exact syntax-owned tuple below. The system has not created or accepted this edge; only the corrected model emit may publish it: " + strings.Join(rows, "; "),
		Fields: uniqueEmitEvidenceRepairFields(fields),
		Metadata: map[string]string{
			"repair_status":       types.ToolRepairStatusActionRequired,
			"repair_scope":        "registration_binding_expression",
			"repair_stage":        "explorer",
			"completion_blocking": "true",
			emitEvidenceRelationRepairObligationsMetadataKey: encodeEmitEvidenceRelationRepairObligations(
				registrationBindingRepairObligations(in)),
		},
	}
}

func mergeEmitEvidenceRelationEndpointRepairs(first, second *types.ToolRepair) *types.ToolRepair {
	if first == nil {
		return second
	}
	if second == nil {
		return first
	}
	first.Fields = uniqueEmitEvidenceRepairFields(append(first.Fields, second.Fields...))
	first.Hint += " Additional exact relation repair: " + second.Hint
	if first.Metadata == nil {
		first.Metadata = make(map[string]string)
	}
	first.Metadata["repair_status"] = types.ToolRepairStatusActionRequired
	firstScope := strings.TrimSpace(first.Metadata["repair_scope"])
	secondScope := strings.TrimSpace(second.Metadata["repair_scope"])
	switch {
	case firstScope == "":
		first.Metadata["repair_scope"] = secondScope
	case secondScope != "" && !strings.Contains("+"+firstScope+"+", "+"+secondScope+"+"):
		first.Metadata["repair_scope"] = firstScope + "+" + secondScope
	}
	first.Metadata["completion_blocking"] = "true"
	mergeEmitEvidenceRelationRepairObligationMetadata(first, second)
	return first
}

func mergeEmitEvidenceValueTransferRepair(classification, endpoint *types.ToolRepair) *types.ToolRepair {
	if classification == nil {
		return endpoint
	}
	if endpoint == nil {
		return classification
	}
	classification.Fields = uniqueEmitEvidenceRepairFields(append(classification.Fields, endpoint.Fields...))
	classification.Hint += " Additional exact endpoint repair: " + endpoint.Hint
	if classification.Metadata == nil {
		classification.Metadata = make(map[string]string)
	}
	endpointScope := strings.TrimSpace(endpoint.Metadata["repair_scope"])
	if endpointScope == "" {
		endpointScope = "relation_endpoint_identity"
	}
	classification.Metadata["repair_status"] = types.ToolRepairStatusActionRequired
	classification.Metadata["repair_scope"] = "value_transfer_classification+" + endpointScope
	classification.Metadata["completion_blocking"] = "true"
	mergeEmitEvidenceRelationRepairObligationMetadata(classification, endpoint)
	return classification
}

func buildEmitEvidenceItemValidationRepair(rejections, fields []string, completionBlocking bool) *types.ToolRepair {
	fields = uniqueEmitEvidenceRepairFields(fields)
	if len(rejections) == 0 || len(fields) == 0 {
		return nil
	}
	metadata := map[string]string{
		"repair_status":  types.ToolRepairStatusActionRequired,
		"repair_scope":   "item_validation",
		"repair_stage":   "explorer",
		"rejected_count": strconv.Itoa(len(rejections)),
	}
	if completionBlocking {
		metadata["completion_blocking"] = "true"
	}
	return &types.ToolRepair{
		Code: types.ToolRepairCodeEvidenceItemValidation,
		Hint: fmt.Sprintf(
			"Correct only the rejected emit_evidence item field(s) %s and re-emit those item(s). Valid siblings were already accepted; do not rebuild the whole evidence batch. Validation reasons: %s",
			strings.Join(fields, ", "), strings.Join(rejections, "; ")),
		Fields:   fields,
		Metadata: metadata,
	}
}

// mergeEmitEvidenceValidationRepairs preserves both producer-owned repair
// recipes when one emit call contains a skipped schema-invalid item and an
// accepted assignment whose directed endpoint authority was removed. Both
// repairs share one code/stage, so one structured repair can carry their field
// union and exact instructions without making the model infer which debt won.
func mergeEmitEvidenceValidationRepairs(validation, endpoint *types.ToolRepair) *types.ToolRepair {
	if validation == nil {
		return endpoint
	}
	if endpoint == nil {
		return validation
	}
	validation.Fields = uniqueEmitEvidenceRepairFields(append(validation.Fields, endpoint.Fields...))
	validation.Hint += " Additional exact endpoint repair: " + endpoint.Hint
	if validation.Metadata == nil {
		validation.Metadata = make(map[string]string)
	}
	validation.Metadata["repair_status"] = types.ToolRepairStatusActionRequired
	endpointScope := strings.TrimSpace(endpoint.Metadata["repair_scope"])
	if endpointScope == "" {
		endpointScope = "relation_endpoint_identity"
	}
	validation.Metadata["repair_scope"] = "item_validation+" + endpointScope
	validation.Metadata["completion_blocking"] = "true"
	mergeEmitEvidenceRelationRepairObligationMetadata(validation, endpoint)
	return validation
}

// emitEvidenceValidationRepairFields converts producer-owned validation
// branches into exact JSON paths. Matching the internal validation error is
// safe here: the text is generated by buildEmitEvidenceItem, not supplied by
// the request or model prose. The original invalid value is never guessed or
// repaired by the system.
func emitEvidenceValidationRepairFields(in emitEvidenceItem, index int, err error) []string {
	prefix := fmt.Sprintf("items[%d].", index)
	if err == nil {
		return []string{strings.TrimSuffix(prefix, ".")}
	}
	msg := err.Error()
	var fields []string
	add := func(field string) {
		fields = append(fields, prefix+field)
	}
	switch {
	case strings.Contains(msg, "scope is required"):
		add("scope")
	case strings.Contains(msg, "evidence_kind"):
		add("evidence_kind")
	case strings.Contains(msg, "source is required") || strings.Contains(msg, "does not look like a repo-relative file path") || strings.Contains(msg, "tool-output blob"):
		add("source")
	case strings.Contains(msg, "scope=line requires line_start"):
		add("line_start")
	case strings.Contains(msg, "line_end") && strings.Contains(msg, "line_start"):
		add("line_start")
		add("line_end")
	case strings.Contains(msg, "section_path"):
		add("section_path")
	case strings.Contains(msg, "scope=file must have line_start"):
		add("line_start")
	case strings.Contains(msg, "file_role_label"):
		add("file_role_label")
	case strings.Contains(msg, "crossfile_query with at least"):
		add("crossfile_query.files")
	case strings.Contains(msg, "crossfile_query.files capped"):
		add("crossfile_query.files")
	case strings.Contains(msg, "crossfile_query.pattern"):
		add("crossfile_query.pattern")
	case strings.Contains(msg, "crossfile_assertion"):
		add("crossfile_assertion.kind")
	case strings.Contains(msg, "negative_query.section"):
		add("negative_query.section")
	case strings.Contains(msg, "negative_query"):
		add("negative_query")
	case strings.Contains(msg, "negative_scope"):
		add("negative_scope")
	case strings.Contains(msg, "relationship items require object"):
		add("object")
	case strings.Contains(msg, "registration items require both subject and object"):
		add("subject")
		add("object")
	case strings.Contains(msg, "conditional items require scope=line and anchor_kind=condition"):
		add("scope")
		add("anchor_kind")
	case strings.Contains(msg, "conditional items require the exact non-empty condition"):
		add("condition")
	case strings.Contains(msg, "anchor_kind"):
		add("anchor_kind")
	case strings.Contains(msg, "anchor_symbol"):
		add("anchor_symbol")
	case strings.Contains(msg, "context_role_hint"):
		add("context_role_hint")
	case strings.Contains(msg, "diagram_role_hint"):
		add("diagram_role_hint")
	case strings.Contains(msg, "salience="):
		add("salience")
	case strings.Contains(msg, "load_bearing_summary=true requires"):
		add("summary")
	default:
		// Keep the repair local even when a future validator adds a new
		// branch before this mapper is extended. The item path is precise and
		// prevents a misleading "no actionable target" result.
		fields = append(fields, strings.TrimSuffix(prefix, "."))
	}
	return uniqueEmitEvidenceRepairFields(fields)
}

func uniqueEmitEvidenceRepairFields(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, field := range in {
		field = strings.TrimSpace(field)
		if field == "" || seen[field] {
			continue
		}
		seen[field] = true
		out = append(out, field)
	}
	return out
}

func emitEvidenceValidationFailureBlocksCompletion(ctx *types.BusContext, in emitEvidenceItem) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	kind := strings.ToLower(strings.TrimSpace(firstNonEmptyString([]string{in.EvidenceKind, in.LegacyKind})))
	// A cross-component call-chain investigation that has explicitly submitted
	// a registration row is declaring a typed boundary candidate. If that row
	// is schema-invalid, allowing completion immediately strands the two proved
	// invocation segments and pushes the Finalizer toward either an invented
	// call edge or a misleadingly disconnected "complete" chain. Keep only
	// this exact typed debt blocking until the model repairs or supersedes it.
	// This gate reads no request/evidence prose and does not infer a binding:
	// the model still has to cite the actual source expression and provide both
	// endpoints. Runtime Trace uses its separate causal contract.
	if types.EvidenceKind(kind) == types.EvidenceRegistration &&
		types.ResolveQuestionFamily(rm) == types.QFCallChain &&
		rm.Predicates.IsCrossComponent &&
		rm.Intent != types.IntentRootCause {
		return true
	}
	if rm.DiagramHint == nil || !rm.DiagramHint.Required ||
		!types.PredicateAxisRequiresDiagramEdgeOwnership(rm.PredicateAxis) {
		return false
	}
	switch types.EvidenceKind(kind) {
	case types.EvidenceRelationship, types.EvidenceRegistration, types.EvidenceConditional:
	default:
		return false
	}
	anchor, ok := findAnchorKind(strings.ToLower(strings.TrimSpace(in.AnchorKind)))
	if !ok {
		// A relation-shaped row whose anchor kind itself is malformed is still
		// load-bearing for a required typed relation diagram.
		return true
	}
	return types.PredicateAxisHasMatchingAnchor(rm.PredicateAxis, types.EvidenceItem{AnchorKind: anchor})
}

type emitEvidenceGroundingTally struct {
	grounded   int
	recovered  int
	ungrounded int
}

func evidenceGroundingTally(items []types.EvidenceItem) emitEvidenceGroundingTally {
	var tally emitEvidenceGroundingTally
	for _, item := range items {
		switch item.GroundingStatus {
		case types.GroundingGrounded:
			tally.grounded++
		case types.GroundingRecovered:
			tally.recovered++
		case types.GroundingUngrounded:
			tally.ungrounded++
		}
	}
	return tally
}

func emitEvidenceSourceCount(items []types.EvidenceItem) int {
	sources := make(map[string]struct{})
	for _, item := range items {
		source := canonicalEmitPath(item.Source)
		if source == "" {
			source = strings.TrimSpace(item.Source)
		}
		if source == "" {
			continue
		}
		sources[source] = struct{}{}
	}
	return len(sources)
}

func cumulativeEmitEvidenceRepairAuditTally(items []types.EvidenceItem) (audit emitEvidenceGroundingTally, coveredOrNonActionable int) {
	if len(items) == 0 {
		return audit, 0
	}
	groundedByFile := make(map[string][]groundedRepairCarrier)
	for _, item := range items {
		if item.Source == "" || item.LineStart <= 0 || item.GroundingStatus != types.GroundingGrounded {
			continue
		}
		file := canonicalEmitPath(item.Source)
		if file == "" {
			file = item.Source
		}
		tails := make(map[string]bool)
		for _, tail := range types.EvidenceSurfaceSymbolTails(item) {
			tails[tail] = true
		}
		groundedByFile[file] = append(groundedByFile[file], groundedRepairCarrier{
			line:  item.LineStart,
			tails: tails,
		})
	}
	for _, item := range items {
		switch item.GroundingStatus {
		case types.GroundingRecovered, types.GroundingUngrounded:
		default:
			continue
		}
		file := canonicalEmitPath(item.Source)
		if file == "" {
			file = item.Source
		}
		if item.Source == "" || item.LineStart <= 0 ||
			evidenceRepairShouldDrop(item) ||
			evidenceRepairCoveredByGroundedSibling(item, file, item.LineStart, groundedByFile[file]) {
			coveredOrNonActionable++
			continue
		}
		switch item.GroundingStatus {
		case types.GroundingRecovered:
			audit.recovered++
		case types.GroundingUngrounded:
			audit.ungrounded++
		}
	}
	return audit, coveredOrNonActionable
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
	targets := emitEvidenceRepairTargets(ctx, items, reports)
	hasNonGrounded := false
	for _, item := range items {
		if item.GroundingStatus == types.GroundingRecovered || item.GroundingStatus == types.GroundingUngrounded {
			hasNonGrounded = true
			break
		}
	}
	if len(targets) == 0 && !hasNonGrounded {
		return nil
	}
	repair := &types.ToolRepair{
		Code: types.ToolRepairCodeEvidenceLineTextRepair,
		Metadata: map[string]string{
			"repair_scope": "line_text_grounding",
			"repair_stage": "explorer",
		},
	}
	if len(targets) == 0 {
		repair.Metadata["repair_status"] = types.ToolRepairStatusSatisfiedOrNonActionable
		return repair
	}
	repair.Hint = renderEmitEvidenceRepairToolHint(targets)
	repair.Targets = targets
	repair.Metadata["repair_status"] = types.ToolRepairStatusActionRequired
	return repair
}

type groundedRepairCarrier struct {
	line  int
	tails map[string]bool
}

func emitEvidenceRepairTargets(ctx *types.BusContext, items []types.EvidenceItem, reports []ground.Report) []types.ToolRepairTarget {
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
		file, ok := canonicalEmitRepairTargetPath(ctx, it.Source)
		if !ok {
			continue
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
		file, ok := canonicalEmitRepairTargetPath(ctx, it.Source)
		if !ok {
			continue
		}
		lines := emitEvidenceRepairCandidateLines(it, i, reports)
		covered := false
		for _, line := range lines {
			if evidenceRepairCoveredByGroundedSibling(it, file, line, groundedByFile[file]) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		b := byFile[file]
		if b == nil {
			b = &bucket{seen: make(map[int]bool)}
			byFile[file] = b
		}
		for _, line := range lines {
			if line <= 0 {
				continue
			}
			if !b.seen[line] {
				b.seen[line] = true
				b.order = append(b.order, line)
			}
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

func canonicalEmitRepairTargetPath(ctx *types.BusContext, source string) (string, bool) {
	file := canonicalEmitPath(source)
	if file == "" || !emitLooksLikePath(file) {
		return "", false
	}
	if path.IsAbs(file) {
		if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
			return "", false
		}
		repoRoot, err := filepath.Abs(ctx.RepoRoot)
		if err != nil {
			return "", false
		}
		absSource, err := filepath.Abs(filepath.FromSlash(file))
		if err != nil {
			return "", false
		}
		rel, err := filepath.Rel(repoRoot, absSource)
		if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
			return "", false
		}
		file = canonicalEmitPath(rel)
	}
	if file == "." || file == ".." || strings.HasPrefix(file, "../") || strings.HasPrefix(file, "/") {
		return "", false
	}
	return file, true
}

func emitEvidenceRepairCandidateLines(item types.EvidenceItem, idx int, reports []ground.Report) []int {
	seen := make(map[int]bool, 3)
	out := make([]int, 0, 3)
	add := func(line int) {
		if line <= 0 || seen[line] {
			return
		}
		seen[line] = true
		out = append(out, line)
	}
	if idx < len(reports) {
		r := reports[idx]
		if r.OriginalLine > 0 && r.AdjustedLine > 0 && r.OriginalLine != r.AdjustedLine {
			add(r.OriginalLine)
			add(r.AdjustedLine)
			return out
		}
		add(r.AdjustedLine)
	}
	add(item.LineStart)
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

func renderToolRepairTargetsInline(targets []types.ToolRepairTarget, maxFiles, maxLines int) string {
	if len(targets) == 0 {
		return "none"
	}
	if maxFiles <= 0 || maxFiles > len(targets) {
		maxFiles = len(targets)
	}
	parts := make([]string, 0, maxFiles+1)
	for _, target := range targets[:maxFiles] {
		file := strings.TrimSpace(target.File)
		if file == "" {
			file = "<unknown>"
		}
		lines := renderToolRepairLineList(target.Lines, maxLines)
		if lines == "" {
			parts = append(parts, file)
			continue
		}
		lineWord := "lines"
		if len(target.Lines) == 1 {
			lineWord = "line"
		}
		parts = append(parts, fmt.Sprintf("%s near %s %s", file, lineWord, lines))
	}
	if len(targets) > maxFiles {
		parts = append(parts, fmt.Sprintf("%d more file(s)", len(targets)-maxFiles))
	}
	return strings.Join(parts, "; ")
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

func failEmitWithRepair(name string, now time.Time, repair *types.ToolRepair, format string, args ...interface{}) (types.ToolResult, error) {
	msg := fmt.Sprintf(format, args...)
	return types.ToolResult{
		ToolName:  name,
		Success:   false,
		Summary:   msg,
		Repair:    attachToolJSONSurfaceMetadata(name, repair),
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
	unverified := dedupeUnverifiedFindings(ctx.Mutable.EvidenceClosure().UnverifiedFindings())
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
	structuredMention := exactResolutionEvidenceMentionsAnyTarget(contract, *ev)
	groundedWindowMention := evidenceGroundedWindowMentionsAnyTarget(*ev, gc, contract)
	if ev.ContextRole == types.EvidenceContextRoleIllustrativeOnly && structuredMention {
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
		types.IsNegativeEvidencePredicate(ev.Predicate) {
		surface := strings.Join([]string{
			ev.Subject, ev.AnchorSymbol, ev.Object, ev.Source,
		}, "\n")
		sameFamily := types.ExactResolutionSameFamilyMatchScore(contract, surface) > 0 ||
			types.ExactResolutionEvidenceCanSatisfyRelatedContext(contract, *ev, nil)
		targetMention := structuredMention ||
			exactResolutionEvidenceDirectlyAnchorsAnyTarget(contract, *ev) ||
			groundedWindowMention
		if targetMention || sameFamily {
			if targetMention {
				ev.ContextRole = types.EvidenceContextRoleAbsenceSupport
			} else if ev.ContextRole == types.EvidenceContextRoleUnknown || ev.ContextRole == types.EvidenceContextRoleDefining {
				ev.ContextRole = types.EvidenceContextRoleRelatedContext
			}
			note := fmt.Sprintf(
				"this item is a negative probe about the unresolved exact %s and cannot count as defining proof. Treat it as context only; do NOT repair this item.",
				exactResolutionTargetLabel(contract),
			)
			if targetMention {
				note = fmt.Sprintf(
					"this item is a negative probe for the unresolved exact %s and supports the absence conclusion, but it does not define the target. Treat it as absence support only; do NOT repair this item.",
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
		(structuredMention || groundedWindowMention) &&
		!exactResolutionEvidenceDirectlyAnchorsAnyTarget(contract, *ev) {
		note := fmt.Sprintf(
			"this item names the requested exact %s only in explanatory context, not as a defining anchor. Treat it as nearby context only; do NOT repair this item.",
			exactResolutionTargetLabel(contract),
		)
		if ev.ContextRole == types.EvidenceContextRoleUnknown || ev.ContextRole == types.EvidenceContextRoleDefining {
			ev.ContextRole = types.EvidenceContextRoleRelatedContext
			changed = true
		}
		if groundedWindowMention {
			note = fmt.Sprintf(
				"this item names the requested exact %s in the grounded code/config window but not as its defining anchor. Keep it as positive related context; it disproves a global not-found conclusion but does not by itself prove the target's definition.",
				exactResolutionTargetLabel(contract),
			)
		}
		if appendGroundingNoteOnce(ev, note) {
			changed = true
		}
	}
	if len(pendingTargets) == 0 {
		return changed
	}
	if ev.ContextRole == types.EvidenceContextRoleIllustrativeOnly || structuredMention || groundedWindowMention {
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
		ev.Subject, ev.Predicate, ev.Object, ev.AnchorSymbol, ev.Condition, ev.Snippet)
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
		canon := ground.CanonicalBusPath(ctx, file)
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
		if file != "" && (file == source || strings.HasSuffix(file, "/"+source) || strings.HasSuffix(source, "/"+file)) {
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
// autoPairRoleDescriptionEvidence (Plan 2 v2, 2026-05-05) walks the
// just-built evidence batch and, for every grounded definition-anchor
// item whose source's leading doc comment is in the read_file gutter,
// produces a parallel evidence_kind=mechanism item carrying the
// component's role description (the WHAT axis to the original's
// WHERE axis). Returns the list of newly synthesised items; callers
// append to the batch and bump the AutoPairedRoleDescriptions counter.
//
// Trigger conditions (ALL must hold):
//   - it.AnchorKind == AnchorDefinition (definition-class anchor)
//   - it.GroundingStatus == Grounded or Recovered (evidence is sound)
//   - it.Scope == ScopeLine and it.LineStart > 0
//   - it.Source is non-empty and present in gc.LineIndex
//   - the same (source, line_start) does not already have a manual
//     EvidenceMechanism entry in this same batch (deduplication)
//
// The extracted comment text feeds Summary; Subject is copied from
// the originating item; Predicate is set to "documents". The synthesised
// item has Producer="auto_pair_role_description" so downstream
// consumers can attribute / filter if needed.
func autoPairRoleDescriptionEvidence(built []types.EvidenceItem, gc *ground.Context) []types.EvidenceItem {
	if len(built) == 0 || gc == nil || len(gc.LineIndex) == 0 {
		return nil
	}
	// Index existing mechanism entries by (source, line_start) so we
	// don't double-pair if the LLM already supplied a WHAT axis.
	type srcLine struct {
		source string
		line   int
	}
	manualMech := make(map[srcLine]bool, len(built))
	for _, it := range built {
		if it.Kind != types.EvidenceMechanism {
			continue
		}
		manualMech[srcLine{it.Source, it.LineStart}] = true
	}
	out := make([]types.EvidenceItem, 0, 4)
	seen := make(map[srcLine]bool, 4)
	for i := range built {
		it := built[i]
		if it.AnchorKind != types.AnchorDefinition {
			continue
		}
		if it.Scope != "" && it.Scope != types.ScopeLine {
			continue
		}
		if it.LineStart <= 0 || it.Source == "" {
			continue
		}
		if it.GroundingStatus != types.GroundingGrounded && it.GroundingStatus != types.GroundingRecovered {
			continue
		}
		key := srcLine{it.Source, it.LineStart}
		if manualMech[key] || seen[key] {
			continue
		}
		text, commentLine := extractDocCommentForGroundedItem(gc, it.Source, it.LineStart)
		if text == "" {
			continue
		}
		seen[key] = true
		mech := types.EvidenceItem{
			Kind:            types.EvidenceMechanism,
			Subject:         it.Subject,
			Predicate:       "documents",
			Object:          it.AnchorSymbol,
			Summary:         text,
			Source:          it.Source,
			LineStart:       commentLine,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    it.AnchorSymbol,
			OwnerSymbol:     it.OwnerSymbol,
			Scope:           types.ScopeLine,
			Confidence:      it.Confidence,
			DerivedFrom:     []string{it.ID},
			Producer:        types.EvidenceProducerAutoPairRoleDescription,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
			GroundingNote:   "auto-paired from leading doc comment",
		}
		mech.ID = types.StableEvidenceID(mech)
		out = append(out, mech)
	}
	return out
}

// extractDocCommentForGroundedItem reconstructs a content view of the
// source from the read_file gutter and dispatches to the
// language-aware comment extractor. Returns empty when the line(s)
// above lineStart were not in the read_file history (no false-pair
// risk from unread regions).
func extractDocCommentForGroundedItem(gc *ground.Context, source string, lineStart int) (string, int) {
	if gc == nil || gc.LineIndex == nil {
		return "", 0
	}
	idx := gc.LineIndex[source]
	if len(idx) == 0 {
		return "", 0
	}
	// Require the line immediately above to be in the gutter — this
	// is where any leading comment block would start its tail. If the
	// region is unread, decline rather than risk a missed comment.
	if _, ok := idx[lineStart-1]; !ok {
		// Allow the case where the def has no comment above but does
		// have a Python docstring on the line(s) below; we still need
		// SOME context. For Python, require lineStart+1 instead.
		if _, okBelow := idx[lineStart+1]; !okBelow {
			return "", 0
		}
	}
	// Build a 1-based contiguous string from the LineIndex map. Find
	// the max line number we know about and pad missing rows with "".
	maxLine := lineStart + 6 // python docstring may sit a few lines below def
	for k := range idx {
		if k > maxLine {
			maxLine = k
		}
	}
	parts := make([]string, maxLine)
	for i := 1; i <= maxLine; i++ {
		parts[i-1] = idx[i]
	}
	content := []byte(strings.Join(parts, "\n"))
	return types.ExtractLeadingDocComment(content, lineStart, source)
}

func normalizeEvidenceSurfaceTerms(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, raw := range in {
		term := strings.TrimSpace(raw)
		if term == "" {
			continue
		}
		if len(term) > 120 {
			term = term[:120]
		}
		key := strings.ToLower(term)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, term)
		if len(out) >= 12 {
			break
		}
	}
	return out
}

func validateEvidenceSurfaceTerms(index int, item types.EvidenceItem, gc *ground.Context) error {
	if len(item.SurfaceTerms) == 0 {
		return nil
	}
	if gc == nil || strings.TrimSpace(item.Source) == "" {
		return fmt.Errorf("items[%d]: surface_terms require already-observed source lines for source=%q", index, item.Source)
	}
	lineIndex := gc.ObservedLineIndex
	if len(lineIndex) == 0 {
		lineIndex = gc.LineIndex
	}
	if len(lineIndex) == 0 {
		return fmt.Errorf("items[%d]: surface_terms require already-observed source lines for source=%q", index, item.Source)
	}
	source := ground.CanonicalContextPath(gc, item.Source)
	fileLines := lineIndex[source]
	if len(fileLines) == 0 {
		return fmt.Errorf("items[%d]: surface_terms require source %q to have appeared in read_file or grep output", index, item.Source)
	}
	window := evidenceSurfaceTermWindow(item, fileLines)
	for _, term := range item.SurfaceTerms {
		if !strings.Contains(window, term) {
			return fmt.Errorf("items[%d]: surface_terms term %q is not grounded in the already-read source window for %s:%d", index, term, item.Source, item.LineStart)
		}
	}
	return nil
}

func dropInvalidEvidenceSurfaceTerms(index int, item *types.EvidenceItem, gc *ground.Context) []string {
	if item == nil || len(item.SurfaceTerms) == 0 {
		return nil
	}
	dropAll := func(reason string) []string {
		dropped := append([]string(nil), item.SurfaceTerms...)
		item.SurfaceTerms = nil
		out := make([]string, 0, len(dropped))
		for _, term := range dropped {
			out = append(out, fmt.Sprintf("items[%d]: %q (%s)", index, term, reason))
		}
		return out
	}
	if gc == nil || strings.TrimSpace(item.Source) == "" {
		return dropAll(fmt.Sprintf("source %q has no observed source window", item.Source))
	}
	lineIndex := gc.ObservedLineIndex
	if len(lineIndex) == 0 {
		lineIndex = gc.LineIndex
	}
	if len(lineIndex) == 0 {
		return dropAll(fmt.Sprintf("source %q has no observed source window", item.Source))
	}
	source := ground.CanonicalContextPath(gc, item.Source)
	fileLines := lineIndex[source]
	if len(fileLines) == 0 {
		return dropAll(fmt.Sprintf("source %q did not appear in read_file or grep output", item.Source))
	}
	window := evidenceSurfaceTermWindow(*item, fileLines)
	if window == "" {
		return dropAll(fmt.Sprintf("source %q has no observed lines near %d", item.Source, item.LineStart))
	}
	kept := make([]string, 0, len(item.SurfaceTerms))
	var dropped []string
	for _, term := range item.SurfaceTerms {
		if strings.Contains(window, term) {
			kept = append(kept, term)
			continue
		}
		dropped = append(dropped, fmt.Sprintf("items[%d]: %q not found in already-read window for %s:%d",
			index, term, item.Source, item.LineStart))
	}
	if len(dropped) == 0 {
		return nil
	}
	item.SurfaceTerms = kept
	return dropped
}

func validateRequestedDecoratorRegistrationAlignment(index int, item types.EvidenceItem, gc *ground.Context, ctx *types.BusContext) error {
	if item.Kind != types.EvidenceRegistration || item.AnchorKind != types.AnchorDefinition {
		return nil
	}
	requested := requestedDecoratorSurfaceTerms(ctx)
	if len(requested) == 0 {
		return nil
	}
	claimed := decoratorSurfaceTermFromLabel(item.Object)
	if claimed == "" || !requested[claimed] {
		return nil
	}
	actual := attachedDecoratorSurfaceTermSet(item, gc)
	if len(actual) == 0 || actual[claimed] {
		return nil
	}
	actualList := make([]string, 0, len(actual))
	for term := range actual {
		actualList = append(actualList, term)
	}
	sort.Strings(actualList)
	return fmt.Errorf(
		"items[%d]: registration object %q claims requested decorator %q, but attached source decorators at %s:%d are %s; re-emit this evidence with the actual decorator object or mark it as related context instead of a principal %s member",
		index, item.Object, claimed, item.Source, item.LineStart, strings.Join(actualList, ", "), claimed)
}

func requestedDecoratorSurfaceTerms(ctx *types.BusContext) map[string]bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	rm := ctx.AnalysisIR.RequestModel
	out := make(map[string]bool)
	add := func(raw string) {
		// Hard decorator-alignment gates require exact decorator syntax from a
		// typed request carrier. Generic analyzer entities such as "Builder" are
		// useful search hints, but they are too noisy to authorize rejection as
		// if the user had precisely requested "@Builder".
		for _, term := range decoratorSurfaceTermRe.FindAllString(raw, -1) {
			out[term] = true
		}
	}
	for _, raw := range rm.AnalyzerHints.Entities {
		add(raw)
	}
	for _, raw := range rm.AnalyzerHints.PrimaryEntities {
		add(raw)
	}
	for _, st := range rm.SubTopics {
		for _, raw := range st.Entities {
			add(raw)
		}
	}
	if rm.SourceInventoryProfile != nil {
		for _, raw := range rm.SourceInventoryProfile.SourceQuotes {
			add(raw)
		}
	}
	if rm.AnswerVisibilityProfile != nil {
		for _, raw := range rm.AnswerVisibilityProfile.SourceQuotes {
			add(raw)
		}
	}
	if rm.CurrentSourceExplanationProfile != nil {
		for _, raw := range rm.CurrentSourceExplanationProfile.SourceQuotes {
			add(raw)
		}
		for _, raw := range rm.CurrentSourceExplanationProfile.TargetTerms {
			add(raw)
		}
	}
	if rm.ExternalObservationPolicy != nil {
		for _, raw := range rm.ExternalObservationPolicy.SourceQuotes {
			add(raw)
		}
	}
	if rm.RequestedAnswerDimensions != nil {
		for _, dim := range rm.RequestedAnswerDimensions.Dimensions {
			add(dim.SourceQuote)
			add(dim.Label)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func attachedDecoratorSurfaceTermSet(item types.EvidenceItem, gc *ground.Context) map[string]bool {
	out := make(map[string]bool)
	add := func(raw string) {
		if term := decoratorSurfaceTermFromLabel(raw); term != "" {
			out[term] = true
		}
	}
	for _, term := range repomapDecoratorSurfaceTermsForEvidence(item, gc) {
		add(term)
	}
	for _, term := range decoratorSurfaceCandidatesAroundItem(item, gc) {
		add(term)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func repomapDecoratorSurfaceTermsForEvidence(item types.EvidenceItem, gc *ground.Context) []string {
	if gc == nil || gc.Graph == nil || item.Source == "" || item.LineStart <= 0 {
		return nil
	}
	source := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
	if source == "" {
		return nil
	}
	var out []string
	seen := make(map[string]bool)
	add := func(raw string) {
		if term := decoratorSurfaceTermFromLabel(raw); term != "" && !seen[term] {
			seen[term] = true
			out = append(out, term)
		}
	}
	for _, fi := range gc.Graph.Files {
		if fi == nil || strings.TrimSpace(strings.ReplaceAll(fi.RelPath, `\`, `/`)) != source {
			continue
		}
		for _, sym := range fi.Symbols {
			if sym.Line != item.LineStart {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(sym.Kind)) {
			case "builder", "styles", "extend", "component", "annotation", "decorator":
				add(decoratorSurfaceTermFromSymbolKind(sym.Kind))
			}
			for _, match := range decoratorSurfaceTermRe.FindAllString(sym.Doc, -1) {
				add(match)
			}
		}
		break
	}
	return out
}

func decoratorSurfaceTermFromSymbolKind(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "builder":
		return "@Builder"
	case "styles":
		return "@Styles"
	case "extend":
		return "@Extend"
	case "component":
		return "@Component"
	case "annotation", "decorator":
		return ""
	default:
		return raw
	}
}

func decoratorSurfaceTermFromLabel(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	match := decoratorSurfaceTermRe.FindString(raw)
	if match != "" {
		return match
	}
	raw = strings.Trim(raw, "`'\"")
	if raw == "" || strings.ContainsAny(raw, " \t\r\n/\\.:,;()[]{}<>") {
		return ""
	}
	for _, r := range raw {
		if !(r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return ""
		}
	}
	return "@" + raw
}

func evidenceSurfaceTermWindow(item types.EvidenceItem, fileLines map[int]string) string {
	if len(fileLines) == 0 {
		return ""
	}
	start, end := item.LineStart, item.LineEnd
	if start <= 0 {
		start = 1
	}
	if end < start {
		end = start
	}
	start -= 8
	if start < 1 {
		start = 1
	}
	end += 8
	var b strings.Builder
	for i := start; i <= end; i++ {
		if line, ok := fileLines[i]; ok {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

type surfaceTermReviewSuggestion struct {
	source string
	line   int
	anchor string
	terms  []string
}

func buildEmitEvidenceSurfaceTermReview(items []types.EvidenceItem, gc *ground.Context) *types.ToolRepair {
	if len(items) == 0 || gc == nil || len(gc.LineIndex) == 0 {
		return nil
	}
	suggestions := make([]surfaceTermReviewSuggestion, 0, 4)
	for _, item := range items {
		if len(suggestions) >= 4 {
			break
		}
		if item.Producer != EmitEvidenceProducer {
			continue
		}
		if item.GroundingStatus != types.GroundingGrounded && item.GroundingStatus != types.GroundingRecovered {
			continue
		}
		terms := missingSurfaceTermReviewCandidates(item, gc)
		if len(terms) == 0 {
			continue
		}
		if len(terms) > 4 {
			terms = terms[:4]
		}
		suggestions = append(suggestions, surfaceTermReviewSuggestion{
			source: item.Source,
			line:   item.LineStart,
			anchor: item.AnchorSymbol,
			terms:  terms,
		})
	}
	if len(suggestions) == 0 {
		return nil
	}
	return &types.ToolRepair{
		Code: EmitEvidenceSurfaceTermReviewCode,
		Hint: renderEmitEvidenceSurfaceTermReviewHint(suggestions),
		Metadata: map[string]string{
			"repair_scope":  "surface_terms",
			"repair_stage":  "explorer",
			"repair_status": types.ToolRepairStatusActionRecommended,
		},
	}
}

func renderEmitEvidenceSurfaceTermReviewHint(suggestions []surfaceTermReviewSuggestion) string {
	var b strings.Builder
	b.WriteString("Progress check: some accepted evidence is anchored under already-read source/header labels that were not model-authored into `surface_terms`.\n")
	b.WriteString("If any of these labels are part of the user-visible answer, re-emit the affected evidence now with the listed `surface_terms`; do not rely on answer synthesis to infer labels from comments or paths.\n")
	for _, s := range suggestions {
		anchor := strings.TrimSpace(s.anchor)
		if anchor == "" {
			anchor = "-"
		}
		fmt.Fprintf(&b, "  - `%s:%d` (%s): add surface_terms %s\n",
			s.source, s.line, anchor, renderQuotedSurfaceTerms(s.terms))
	}
	return b.String()
}

func renderQuotedSurfaceTerms(terms []string) string {
	parts := make([]string, 0, len(terms))
	for _, term := range terms {
		if term = strings.TrimSpace(term); term != "" {
			parts = append(parts, strconv.Quote(term))
		}
	}
	if len(parts) == 0 {
		return "[]"
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func missingSurfaceTermReviewCandidates(item types.EvidenceItem, gc *ground.Context) []string {
	if item.Source == "" || item.LineStart <= 0 || gc == nil {
		return nil
	}
	if len(gc.LineIndex[item.Source]) == 0 {
		return nil
	}
	comment, _ := extractDocCommentForGroundedItem(gc, item.Source, item.LineStart)
	candidates := sourceLabelCandidatesFromText(comment, item.Source)
	candidates = append(candidates, decoratorSurfaceCandidatesAroundItem(item, gc)...)
	if len(candidates) == 0 {
		return nil
	}
	existing := surfaceTermReviewExistingText(item)
	out := make([]string, 0, len(candidates))
	seen := make(map[string]bool, len(candidates))
	for _, term := range candidates {
		key := strings.ToLower(term)
		if seen[key] ||
			surfaceTermReviewContains(existing, term) ||
			surfaceTermReviewDuplicatesEvidenceSource(term, item) ||
			!types.SurfaceTermShouldBeRequiredForEvidence(term, item) {
			continue
		}
		seen[key] = true
		out = append(out, term)
	}
	return out
}

func decoratorSurfaceCandidatesAroundItem(item types.EvidenceItem, gc *ground.Context) []string {
	if gc == nil || item.Source == "" || item.LineStart <= 0 {
		return nil
	}
	fileLines := gc.LineIndex[item.Source]
	if len(fileLines) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	addFromLine := func(text string) {
		text = strings.TrimSpace(text)
		if text == "" || !strings.HasPrefix(text, "@") {
			return
		}
		for _, match := range decoratorSurfaceTermRe.FindAllString(text, -1) {
			if !seen[match] {
				seen[match] = true
				out = append(out, match)
			}
		}
	}
	if text, ok := fileLines[item.LineStart]; ok {
		addFromLine(text)
	}
	for line := item.LineStart - 1; line >= 1; line-- {
		text, ok := fileLines[line]
		if !ok {
			break
		}
		text = strings.TrimSpace(text)
		if text == "" || !strings.HasPrefix(text, "@") {
			break
		}
		addFromLine(text)
	}
	return out
}

var decoratorSurfaceTermRe = regexp.MustCompile(`@[A-Za-z_][A-Za-z0-9_]*`)

func surfaceTermReviewExistingText(item types.EvidenceItem) string {
	parts := []string{
		item.Subject,
		item.Predicate,
		item.Object,
		item.AnchorSymbol,
		item.Snippet,
	}
	parts = append(parts, item.SurfaceTerms...)
	return "\n" + strings.ToLower(strings.Join(parts, "\n")) + "\n"
}

func surfaceTermReviewContains(existing, term string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return true
	}
	return strings.Contains(existing, term)
}

func surfaceTermReviewDuplicatesEvidenceSource(term string, item types.EvidenceItem) bool {
	termPath := normalizeSurfaceTermReviewPath(term)
	sourcePath := normalizeSurfaceTermReviewPath(item.Source)
	if termPath == "" || sourcePath == "" {
		return false
	}
	if strings.EqualFold(termPath, sourcePath) {
		return true
	}
	if !strings.Contains(termPath, "/") {
		return strings.EqualFold(path.Base(sourcePath), termPath)
	}
	return sourcePathHasSegmentSuffix(sourcePath, termPath)
}

func normalizeSurfaceTermReviewPath(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "`'\"")
	raw = strings.ReplaceAll(raw, `\`, `/`)
	raw = strings.Trim(raw, "/")
	if raw == "" {
		return ""
	}
	return path.Clean(raw)
}

func sourcePathHasSegmentSuffix(sourcePath, suffix string) bool {
	if suffix == "" || sourcePath == "" {
		return false
	}
	sourcePath = strings.Trim(sourcePath, "/")
	suffix = strings.Trim(suffix, "/")
	if strings.EqualFold(sourcePath, suffix) {
		return true
	}
	return len(sourcePath) > len(suffix) &&
		strings.HasSuffix(strings.ToLower(sourcePath), strings.ToLower("/"+suffix))
}

func sourceLabelCandidatesFromText(text, source string) []string {
	sourceExt := strings.ToLower(filepath.Ext(source))
	if sourceExt == "" {
		return nil
	}
	tokens := splitSurfaceReviewTokens(text)
	out := make([]string, 0, len(tokens))
	seen := make(map[string]bool, len(tokens))
	for _, tok := range tokens {
		term := strings.TrimSpace(tok)
		if !looksLikeSourceLabelForSameExt(term, sourceExt) {
			continue
		}
		key := strings.ToLower(term)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, term)
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func splitSurfaceReviewTokens(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
		switch r {
		case '_', '.', '-', '/', ':', '@':
			return false
		default:
			return true
		}
	})
}

func looksLikeSourceLabelForSameExt(term, sourceExt string) bool {
	term = strings.TrimSpace(term)
	if term == "" || !types.IsCodeIdentitySurface(term) {
		return false
	}
	ext := strings.ToLower(filepath.Ext(term))
	if ext == "" || ext != sourceExt {
		return false
	}
	// A same-extension file-like label is precise enough for an
	// advisory repair. It avoids domain names such as example.com in a
	// .go file while still covering cross-language original-source
	// labels like Index.ets, Widget.cpp, module.cj, and foo.ts.
	base := strings.TrimSuffix(filepath.Base(term), filepath.Ext(term))
	return base != "" && strings.ContainsAny(term, "./")
}

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
