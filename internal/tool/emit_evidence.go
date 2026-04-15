package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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

func buildEmitEvidenceAllowedKinds() map[string]types.EvidenceKind {
	out := make(map[string]types.EvidenceKind, len(types.LLMEmittableEvidenceKinds()))
	for _, k := range types.LLMEmittableEvidenceKinds() {
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

type emitEvidenceParams struct {
	Items []emitEvidenceItem `json:"items"`
}

type emitEvidenceItem struct {
	Kind      string `json:"kind"`
	Subject   string `json:"subject,omitempty"`
	Predicate string `json:"predicate,omitempty"`
	Object    string `json:"object,omitempty"`
	Source    string `json:"source"`
	LineStart int    `json:"line_start,omitempty"`
	LineEnd   int    `json:"line_end,omitempty"`
	Condition string `json:"condition,omitempty"`
	Summary   string `json:"summary,omitempty"`
}

// EmitEvidenceProducer is the Producer string stamped on every item
// the tool ingests. Exported so tests and downstream consumers
// (filtering-pipeline doc, future grounder integration) can identify
// the channel without grepping for a literal.
const EmitEvidenceProducer = "explorer.emit_evidence"

func (t *EmitEvidence) Name() string { return "emit_evidence" }

func (t *EmitEvidence) Description() string {
	return "Emit one or more structured evidence items as the result of reading a source file. " +
		"Call this AFTER you have read a file in Phase 2 of the explore stage, with one item per " +
		"fact you want the synthesis layer to see. The batched 'items' array preserves the " +
		"existing 'one tool call per file' write pattern; do not call this tool once per item. " +
		"Every item MUST cite source (file path) and SHOULD cite line_start (gutter line number, " +
		"never estimated). kind is one of: " + strings.Join(emitEvidenceAllowedKindNames(), ", ") +
		". Unknown kinds and unknown fields are REJECTED — the tool will not " +
		"silently coerce. If you are unsure which kind to use, prefer 'direct' over guessing."
}

func (t *EmitEvidence) Parameters() json.RawMessage {
	// Build the enum list as JSON so it stays in lockstep with the
	// canonical types.LLMEmittableEvidenceKinds list — hand-editing the
	// schema literal is how the 6-vs-11 drift bug was born.
	enumJSON, _ := json.Marshal(emitEvidenceAllowedKindNames())
	schema := fmt.Sprintf(`{
  "type": "object",
  "properties": {
    "items": {
      "type": "array",
      "description": "Batch of evidence items extracted from one or more files. Send the full batch in one call — do not invoke the tool per item.",
      "items": {
        "type": "object",
        "properties": {
          "kind":       {"type": "string", "enum": %s, "description": "Evidence shape. direct = literal fact at file:line. conditional = behaviour gated by an IF clause. registration = something registered/bound with EXACT values. mechanism = how a process works step by step. relationship = link between two symbols (use subject + object). absent = expected pattern was looked for and NOT found."},
          "subject":    {"type": "string", "description": "Primary symbol the item is about (function name, type, key). Optional but strongly recommended."},
          "predicate":  {"type": "string", "description": "Verb tying subject to object (e.g. 'binds', 'returns', 'calls'). Optional; defaults to the lower-cased kind."},
          "object":     {"type": "string", "description": "Secondary symbol or value. Required for relationship; optional otherwise."},
          "source":     {"type": "string", "description": "Repository-relative file path the fact comes from. Required."},
          "line_start": {"type": "integer", "description": "First line of the cited code, taken EXACTLY from the read_file gutter. Use 0 (or omit) only if no specific line applies."},
          "line_end":   {"type": "integer", "description": "Last line of the cited range. Defaults to line_start when omitted."},
          "condition":  {"type": "string", "description": "For conditional items: the exact IF clause that triggers the behaviour. Optional otherwise."},
          "summary":    {"type": "string", "description": "Free-text rationale describing the fact. Keep concise; do not paraphrase numbers or string literals."}
        },
        "required": ["kind", "source"]
      }
    }
  },
  "required": ["items"]
}`, string(enumJSON))
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
	built := make([]types.EvidenceItem, 0, len(p.Items))
	for i, in := range p.Items {
		ev, perr := buildEmitEvidenceItem(in, i, workDir)
		if perr != nil {
			return failEmit(t.Name(), now, "%v", perr)
		}
		built = append(built, ev)
	}

	ctx.Mutable.AppendEvidence(built)

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   renderEmitSummary(built),
		Timestamp: now,
	}, nil
}

// buildEmitEvidenceItem validates a single decoded item and converts
// it into a types.EvidenceItem with stable ID and producer stamped.
// All validation is structural, never wordlist-based.
func buildEmitEvidenceItem(in emitEvidenceItem, index int, workDir string) (types.EvidenceItem, error) {
	kindKey := strings.ToLower(strings.TrimSpace(in.Kind))
	kind, ok := emitEvidenceAllowedKinds[kindKey]
	if !ok {
		return types.EvidenceItem{}, fmt.Errorf("items[%d]: unknown kind %q (allowed: %s)", index, in.Kind, strings.Join(emitEvidenceAllowedKindNames(), ", "))
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
	if in.LineStart < 0 || in.LineEnd < 0 {
		return types.EvidenceItem{}, fmt.Errorf("items[%d]: line_start/line_end must be >= 0", index)
	}
	if in.LineEnd > 0 && in.LineEnd < in.LineStart {
		return types.EvidenceItem{}, fmt.Errorf("items[%d]: line_end (%d) is before line_start (%d)", index, in.LineEnd, in.LineStart)
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
	lineStart := in.LineStart
	lineEnd := in.LineEnd
	if lineEnd == 0 {
		lineEnd = lineStart
	}

	item := types.EvidenceItem{
		Kind:       kind,
		Subject:    subject,
		Predicate:  predicate,
		Object:     object,
		Summary:    summary,
		Condition:  condition,
		Source:     source,
		LineStart:  lineStart,
		LineEnd:    lineEnd,
		Confidence: 0.78, // matches parseEvidenceLine's confidence floor
		Producer:   EmitEvidenceProducer,
	}
	item.ID = types.StableEvidenceID(item.Kind, item.Subject, item.Predicate, item.Object, item.Condition, item.Source, item.LineStart, item.LineEnd)
	return item, nil
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

func renderEmitSummary(items []types.EvidenceItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "emit_evidence accepted %d item(s)\n", len(items))
	bySource := make(map[string]int)
	for _, it := range items {
		bySource[it.Source]++
	}
	for src, n := range bySource {
		fmt.Fprintf(&b, "  %s: %d\n", src, n)
	}
	return b.String()
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
