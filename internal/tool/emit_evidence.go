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
	"time"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/authority"
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
	LoadBearingSummary bool `json:"load_bearing_summary,omitempty"`

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
const EmitEvidenceProducer = "explorer.emit_evidence"

// EmitEvidenceSurfaceTermReviewCode marks a successful emit_evidence
// result whose accepted facts are grounded, but whose nearby
// already-read source/header labels look like user-visible aliases the
// model did not yet author into surface_terms. The repair is advisory:
// it asks the model to re-emit structured data when those labels are
// load-bearing; it never auto-fills answer text.
const EmitEvidenceSurfaceTermReviewCode = "evidence_surface_terms_review"

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
		"  - anchor_kind   = the source surface at source:line_start (definition / call / condition / return / assignment / initializer / import / text_reference)\n" +
		"Never put `direct` / `conditional` / `registration` / `mechanism` / `relationship` into anchor_kind. " +
		"Never put `definition` / `call` / `condition` / `return` / `assignment` / `initializer` / `import` / `text_reference` into evidence_kind.\n\n" +
		"anchor_kind tells the grounder what KIND of code location you are pointing at:\n" +
		"  - definition: the line is a function/type/const/var declaration\n" +
		"  - call:       the line contains a function/method call (anchor_symbol = callee name)\n" +
		"  - condition:  the line starts an if / when / unless / switch / case / guard\n" +
		"  - return:     the line is a return or yield\n" +
		"  - assignment: the line assigns (:= or =)\n" +
		"  - initializer: the line initializes a field/property/member inside a struct/object/named-argument/designated/config literal\n" +
		"  - import:     the line is an import / use / require (anchor_symbol = package path/alias)\n\n" +
		"  - text_reference: the line's visible source/config/doc/comment text is itself the evidence; use this for documentation references, examples, generated headers, config prose, or comment-only mentions. It does NOT prove a definition/call/assignment.\n\n" +
		"anchor_symbol is the concrete identifier the grounder should see at line_start. For a " +
		"method call 'x.Execute()' at line 42 the anchor_symbol is 'Execute' and anchor_kind is 'call'. " +
		"For a struct type declaration 'type Orchestrator struct' the anchor_symbol is 'Orchestrator' " +
		"and anchor_kind is 'definition'. On that same line the evidence_kind may still be 'direct' " +
		"because the semantic claim is simply a direct fact about a definition. Likewise a config assignment " +
		"line can be `evidence_kind=\"direct\"` with `anchor_kind=\"assignment\"`. A line such as " +
		"`CitationReq: types.CitationReq{Required: false}` or `.required = false` is an initializer, " +
		"not a symbol definition.\n\n" +
		"For call-like evidence (`predicate` = calls / invokes / dispatches / delegates to) with " +
		"`anchor_kind=\"call\"`, the semantic direction is ALWAYS caller -> callee: `subject` must be " +
		"the containing function/method and `object` must be the callee on that line. Example: if " +
		"`outer()` contains `return inner(...)`, emit `subject=\"outer\"`, `predicate=\"calls\"`, " +
		"`object=\"inner\"`.\n\n" +
		"Optional hint fields: `context_role_hint` may be `defining`, `absence_support`, `related_context`, or `illustrative_only` " +
		"to recommend how the item should be used for exact-target answers. `diagram_role_hint` may be `default`, " +
		"`config`, `runtime`, or `override` for config-precedence traces (`config` = grounded repo/user config-file layer such as YAML/JSON/TOML/INI/etc.). These are recommendations only: the tool " +
		"validates them structurally and may downgrade or ignore inconsistent hints.\n\n" +
		"surface_terms is optional model-authored structured data for exact user-visible labels / aliases copied verbatim from already-read source, log, or trace lines (for example route names, package/module labels, config keys, macro names, trace span names, original file labels, and labels in leading documentation/header comments attached to the cited anchor). The tool rejects any surface term that is not grounded in the read window; final answer validation may require preserving accepted terms.\n\n" +
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
				"description": "Batch of evidence items extracted from one or more files. Send the full batch in one call — do not invoke the tool per item.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"scope": map[string]any{
							"type":        "string",
							"enum":        emitEvidenceScopeNames(),
							"description": "REQUIRED. Anchor shape — the system routes the item through a scope-specific grounder. Pick the scope that matches what your evidence proves. NOTE: 'line' is NOT a default; pick the most specific scope. If the fact is layer-shaped / contract-shaped / absence-shaped (NOT a per-line code claim), prefer 'file' / 'crossfile' / 'negative' — using 'line' to anchor a sibling line as a proxy for layer identity weakens the answer surface (the line-only renderer hides the layer/contract/absence semantics in prose). Definitions: 'line' = single (file, line) — direct/conditional/mechanism/relationship/registration over a specific code location; 'line_range' = multi-line block (struct definition, function body, comment block); 'section' = named YAML/Go/JSON/TOML schema section (use section_path); 'file' = file's identity AS a layer (use file_role_label) — the file IS the config layer / CLI registration layer / etc., independent of any specific line in it; 'crossfile' = cross-file contract verified by query (use crossfile_query + crossfile_assertion) — the system re-runs your query and rejects the item if the assertion fails; 'negative' = confirmed absence (use negative_query + negative_scope, requires evidence_kind='absent'). The system rejects emit if the per-scope required fields are missing.",
						},
						"evidence_kind": map[string]any{
							"type":        "string",
							"enum":        emitEvidenceAllowedKindNames(),
							"description": "REQUIRED. Semantic fact shape, NOT the syntax at line_start. direct = literal fact at file:line. conditional = behaviour gated by an IF clause. registration = something registered/bound with EXACT values. mechanism = how a process works step by step. relationship = link between two symbols (use subject + object). absent = confirmed absence (REQUIRES scope='negative'). Values like definition/call/assignment belong in anchor_kind, not here.",
						},
						"subject": map[string]any{
							"type":        "string",
							"description": "Primary semantic symbol the item is about (function name, type, key). For call-like predicates with anchor_kind='call', subject MUST be the caller / containing function at that line.",
						},
						"predicate": map[string]any{
							"type":        "string",
							"description": "Lowercase verb tying subject to object. PREFER these canonical verbs so the deterministic relation-diagram renderer picks the edge up — anything outside this list is rendered as unstructured prose: calls, invokes, dispatches, delegates to, binds, binds ONLY, registers, wires, provides, returns, yields, constructs, instantiates, defines, implements, extends, embeds, maps, config, decorates. Optional; defaults to the lower-cased evidence_kind.",
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
							"description": "Exact gutter line number from read_file — NEVER estimated. The grounder uses this to verify the claim; wrong numbers are flagged as ungrounded or auto-recovered.",
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
						"load_bearing_summary": map[string]any{
							"type":        "boolean",
							"description": "OPTIONAL. Default false. Set true ONLY when the `summary` text holds a scalar (commit hash, version string, count, single concrete identifier, value derived from a tool / shell / git command output) that the user-facing answer must reproduce verbatim AND the typed fields (subject / predicate / object / anchor_symbol / snippet) cannot themselves carry that scalar. False is correct for the common case where summary is a paraphrase / rationale that the typed fields already encode. The tool rejects this flag when summary is empty.",
						},
						"surface_terms": map[string]any{
							"type":        "array",
							"items":       map[string]any{"type": "string"},
							"description": "OPTIONAL. Model-authored exact strings from the already-read source/log/trace lines that the final answer should preserve as visible aliases or labels, but that are not already captured by subject/object/anchor_symbol. Use for original file labels, route names, package/module names, config keys, macro names, trace span names, runtime object labels, or labels in leading documentation/header comments attached to the cited anchor. Every term must appear verbatim in the cited source window; the tool rejects ungrounded terms.",
						},
						"anchor_kind": map[string]any{
							"type":        "string",
							"enum":        emitAnchorKindNames(),
							"description": "REQUIRED. Source surface at line_start, NOT the semantic evidence shape. definition = symbol declaration, call = function/method call site, condition = if/when/switch/case/guard line, return = return or yield, assignment = := or = assignment/write, initializer = field/property/member inside a struct/object/named-argument/designated/config literal, import = import/use/require statement, text_reference = visible source/config/doc/comment text is itself the evidence and must not be treated as definition/call/assignment proof. Values like direct/conditional/registration belong in evidence_kind, not here. The grounder dispatches on this so wrong anchor kinds produce confusing ungrounded verdicts.",
						},
						"anchor_symbol": map[string]any{
							"type":        "string",
							"description": "REQUIRED. The identifier the grounder should find on line_start. For a call like 'x.Execute()' the anchor_symbol is 'Execute'. For a type decl 'type Orchestrator struct' the anchor_symbol is 'Orchestrator'. For an import the anchor_symbol is the package path or local alias.",
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
							"description": "REQUIRED when scope='crossfile'. Structured query the LLM is asserting about. Files is hard-capped at 5; pattern is a Go regex.",
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
							"description": "REQUIRED when scope='negative'. The query whose ABSENCE of matches is the claim. Pair with negative_scope to control where the query searches.",
							"properties": map[string]any{
								"file":    map[string]any{"type": "string"},
								"pattern": map[string]any{"type": "string"},
								"section": map[string]any{"type": "string", "description": "Required when negative_scope='section'."},
							},
						},
						"negative_scope": map[string]any{
							"type":        "string",
							"enum":        emitEvidenceNegativeScopeNames(),
							"description": "REQUIRED when scope='negative'. Qualifies WHERE the absence holds: file = whole file; range = within a line range; section = within a named schema section; struct_fields = against a struct's field set.",
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
          "predicate":     {"type": "string", "description": "Lowercase verb tying subject to object. PREFER these canonical verbs so the deterministic relation-diagram renderer picks the edge up — anything outside this list is rendered as unstructured prose: calls, invokes, dispatches, delegates to, binds, binds ONLY, registers, wires, provides, returns, yields, constructs, instantiates, defines, implements, extends, embeds, maps, config, decorates. Optional; defaults to the lower-cased kind."},
          "object":        {"type": "string", "description": "Secondary symbol or value. Required for relationship; optional otherwise. For call-like predicates with anchor_kind='call', object MUST be the callee symbol on that line."},
          "source":        {"type": "string", "description": "Repository-relative file path the fact comes from. Required."},
          "line_start":    {"type": "integer", "description": "Exact gutter line number from read_file — NEVER estimated. The grounder uses this to verify the claim; wrong numbers are flagged as ungrounded or auto-recovered."},
          "line_end":      {"type": "integer", "description": "Last line of the cited range. Defaults to line_start when omitted."},
          "condition":     {"type": "string", "description": "For conditional items: the exact IF clause that triggers the behaviour."},
          "summary":       {"type": "string", "description": "Free-text rationale describing the fact. Keep concise; do not paraphrase numbers or string literals."},
          "context_role_hint": {"type": "string", "enum": %s, "description": "OPTIONAL recommendation for exact-target questions. defining = direct defining proof, absence_support = grounded evidence that helps justify why the exact target is absent but does NOT define it, related_context = grounded nearby context but not the exact target itself, illustrative_only = comment/doc/test/example mention that should NOT be treated as defining proof. The tool validates and may downgrade the hint."},
          "diagram_role_hint": {"type": "string", "enum": %s, "description": "OPTIONAL recommendation for config-precedence traces. default = code defaults, config = repo/user config-file layer (YAML/JSON/TOML/INI/etc.), runtime = code/runtime binding layer, override = CLI/high-precedence override layer. The tool validates and may ignore inconsistent hints."},
          "surface_terms": {"type": "array", "items": {"type": "string"}, "description": "OPTIONAL exact source/log/trace strings that should remain visible in the final answer as aliases or labels, including labels from leading documentation/header comments attached to the cited anchor. Every term must appear verbatim in the already-read source window."},
          "anchor_kind":   {"type": "string", "enum": %s, "description": "REQUIRED. What line_start points at: 'definition' = symbol declaration, 'call' = function/method call site, 'condition' = if/when/switch/case/guard line, 'return' = return or yield, 'assignment' = := or = assignment/write, 'initializer' = field/property/member inside a struct/object/named-argument/designated/config literal, 'import' = import/use/require statement, 'text_reference' = visible source/config/doc/comment text itself. text_reference is for docs/examples/generated headers/config prose/comment-only mentions and cannot prove a definition/call/assignment."},
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
			Summary:   "emit_evidence requires a writable context; the caller did not provide one (sub-agents are not supported)",
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
		err = RemapStrictDecodeError(err, nil)
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
	// P4-cross-sub-repo (Sc 6): wire ground's package-level
	// CrossRepoOracle from BusContext.MultiGraph (type-asserted via
	// inline interface so internal/tool/ground stays free of the
	// repomap/multigraph import cycle that tool/ground →
	// repomap/multigraph → repomap/topology → tool would form).
	if oracleSource, ok := ctx.MultiGraph.(interface {
		Oracle() types.SymbolOracle
	}); ok && oracleSource != nil {
		ground.SetCrossRepoOracle(oracleSource.Oracle())
	} else {
		ground.SetCrossRepoOracle(nil)
	}
	gc := ground.BuildContext(ctx)
	diagramRequiredFiles := exactResolutionDiagramRequiredFiles(ctx, exactResolutionContract)
	reports := make([]ground.Report, len(built))
	surfaceTermRejects := make([]string, 0)
	surfaceAlignmentRejects := make([]string, 0)
	for i := range built {
		// Per-scope dispatch: ScopeLine routes to the existing tier
		// cascade; schema-level scopes (File / Crossfile / Negative)
		// route to their own grounders.
		r := ground.GroundItemScoped(&built[i], gc)
		normalizeCallEvidenceDirection(&built[i], gc)
		if stampEvidenceOwnerSymbol(&built[i], gc) {
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
		if err := validateEvidenceSurfaceTerms(i, built[i], gc); err != nil {
			surfaceTermRejects = append(surfaceTermRejects, err.Error())
		}
		if err := validateRequestedDecoratorRegistrationAlignment(i, built[i], gc, ctx); err != nil {
			surfaceAlignmentRejects = append(surfaceAlignmentRejects, err.Error())
		}
	}
	if len(surfaceTermRejects) > 0 {
		return failEmit(t.Name(), now,
			"surface_terms must be exact strings from already-read source lines:\n%s",
			strings.Join(surfaceTermRejects, "\n"))
	}
	if len(surfaceAlignmentRejects) > 0 {
		return failEmit(t.Name(), now,
			"evidence surface alignment failed:\n%s",
			strings.Join(surfaceAlignmentRejects, "\n"))
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

	ctx.Mutable.AppendEvidence(built)

	surfaceReview := buildEmitEvidenceSurfaceTermReview(built, gc)
	summary := renderEmitSummary(ctx, built, reports, ctx.Mutable.EmittedEvidence())
	if surfaceReview != nil && strings.TrimSpace(surfaceReview.Hint) != "" {
		summary = strings.TrimRight(summary, "\n") + "\n\n" + surfaceReview.Hint + "\n"
	}
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
	if repair == nil || (repair.Metadata != nil && repair.Metadata["repair_status"] != "action_required") {
		if surfaceReview != nil {
			repair = surfaceReview
		}
	}
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
	case types.AnchorDefinition, types.AnchorAssignment, types.AnchorInitializer, types.AnchorImport, types.AnchorReturn:
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
	candidates := emitPreferredCallTargetNames(it)
	caller := enclosingCallableSymbolName(fi, it.LineStart)
	callee := ""
	if exact, ok := sourceLineCallTargetForCandidates(gc, it.Source, it.LineStart, candidates); ok {
		callee = exact
	} else if rel, ok := findCallRelationAtLineForCandidates(fi, it.LineStart, candidates); ok {
		callee = callRelationTargetName(gc.Graph, fi, rel)
	}
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
	add(it.AnchorSymbol)
	add(it.Object)
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
	if gc == nil || gc.Graph == nil {
		return false
	}
	fi, ok := gc.Graph.FileIndex[it.Source]
	if !ok || fi == nil {
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

func stabilizeConditionAnchorClaim(it *types.EvidenceItem, gc *ground.Context) bool {
	if it == nil || gc == nil || it.Source == "" || it.LineStart <= 0 {
		return false
	}
	claim := normalizeStatementLocalAnchorClaim(it.Condition)
	if claim == "" {
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
	if it == nil || gc == nil || gc.Graph == nil || it.Source == "" || it.LineStart <= 0 {
		return ""
	}
	switch it.AnchorKind {
	case types.AnchorCall, types.AnchorCondition, types.AnchorReturn, types.AnchorAssignment, types.AnchorInitializer:
	default:
		return ""
	}
	fi, ok := gc.Graph.FileIndex[it.Source]
	if !ok || fi == nil {
		return ""
	}
	return enclosingCallableSymbolName(fi, it.LineStart)
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
		if relName == "" {
			relName = strings.TrimSpace(rel.To)
		}
		if candSet[relName] {
			return rel, true
		}
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
		Code: "evidence_line_text_repair",
		Metadata: map[string]string{
			"repair_scope": "line_text_grounding",
			"repair_stage": "explorer",
		},
	}
	if len(targets) == 0 {
		repair.Metadata["repair_status"] = "satisfied_or_non_actionable"
		return repair
	}
	repair.Hint = renderEmitEvidenceRepairToolHint(targets)
	repair.Targets = targets
	repair.Metadata["repair_status"] = "action_required"
	return repair
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
		types.IsNegativeEvidencePredicate(ev.Predicate) {
		surface := strings.Join([]string{
			ev.Subject, ev.AnchorSymbol, ev.Object, ev.Source,
		}, "\n")
		sameFamily := types.ExactResolutionSameFamilyMatchScore(contract, surface) > 0 ||
			types.ExactResolutionEvidenceCanSatisfyRelatedContext(contract, *ev, nil)
		targetMention := exactResolutionEvidenceMentionsAnyTarget(contract, *ev) ||
			exactResolutionEvidenceDirectlyAnchorsAnyTarget(contract, *ev) ||
			evidenceGroundedWindowMentionsAnyTarget(*ev, gc, contract)
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
			Producer:        "auto_pair_role_description",
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
	if gc == nil || len(gc.LineIndex) == 0 || strings.TrimSpace(item.Source) == "" {
		return fmt.Errorf("items[%d]: surface_terms require already-read source lines for source=%q", index, item.Source)
	}
	fileLines := gc.LineIndex[item.Source]
	if len(fileLines) == 0 {
		return fmt.Errorf("items[%d]: surface_terms require source %q to have been read with read_file", index, item.Source)
	}
	window := evidenceSurfaceTermWindow(item, fileLines)
	for _, term := range item.SurfaceTerms {
		if !strings.Contains(window, term) {
			return fmt.Errorf("items[%d]: surface_terms term %q is not grounded in the already-read source window for %s:%d", index, term, item.Source, item.LineStart)
		}
	}
	return nil
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
		term := decoratorSurfaceTermFromLabel(raw)
		if term != "" {
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
			"repair_status": "action_recommended",
		},
	}
}

func renderEmitEvidenceSurfaceTermReviewHint(suggestions []surfaceTermReviewSuggestion) string {
	var b strings.Builder
	b.WriteString("MID-LOOP CHECK: some accepted evidence is anchored under already-read source/header labels that were not model-authored into `surface_terms`.\n")
	b.WriteString("If any of these labels are part of the user-visible answer, re-emit the affected evidence now with the listed `surface_terms`; do not rely on downstream synthesis to infer labels from comments or paths.\n")
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
		if seen[key] || surfaceTermReviewContains(existing, term) || !types.SurfaceTermShouldBeRequiredForEvidence(term, item) {
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
