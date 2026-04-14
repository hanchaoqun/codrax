package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// EmitAnswerSymbol is the structured channel through which the
// extractor agent (P2.1 Turn B) emits AnswerSymbol items directly,
// instead of relying on extractAnswerSymbols' fragile chain-walking
// over markdown evidence.
//
// P2.1 design (memory/project_architecture_remediation_roadmap.md §6,
// Turn B is constrained to a fixed tool set: emit_evidence,
// emit_answer_symbol, emit_hypothesis_verdict). Read-only investigation
// tools (grep, read_file, repo_map) are NOT exposed to Turn B —
// extraction must work from Turn A's transcript snapshot, never from
// fresh file IO.
//
// Two structural defenses ride on this tool:
//
//  1. Line-hallucination gate (Pattern 2 from the fake-green audit).
//     Every item REQUIRES line > 0. Items without a concrete line are
//     rejected at decode time, never silently demoted.
//
//  2. Blob-file path leak gate (UNRESOLVED bug N=1, see
//     memory/project_blob_file_leak_unresolved.md and
//     docs/bug-blob-file-path-leak-into-final-answer.md). The file
//     field MUST NOT live under BusContext.WorkDir. Turn A's tool
//     output blobs are stored there; if the LLM mistakes a blob path
//     for a repo path the symbol's file:line citation points to a
//     self-erasing tempdir entry. The check is structural — string
//     prefix match against the absolute or relative WorkDir, no
//     extension list — so it cannot be bypassed by spelling tricks.
//
// Classified ReadOnly because IsWrite() is the filesystem-write
// boundary; mutating BusContext is not a filesystem write.
// Classified NonEvidenceTool: the answer symbol it carries is part of
// the final answer slate, not a repo fact.
type EmitAnswerSymbol struct {
	ReadOnly
	NonEvidenceTool
}

// Allowed kinds. Mirrors the symbol kinds the explorer's repomap
// graph actually emits (see internal/tool/repomap/*). The list is
// closed because AnswerSymbol.Kind is consumed by the finalizer's
// renderer which has zero handling for unknown kinds.
var emitAnswerSymbolAllowedKinds = map[string]string{
	"function": "function",
	"func":     "function",
	"method":   "method",
	"type":     "type",
	"struct":   "struct",
	"class":    "class",
	"const":    "const",
	"var":      "var",
}

type emitAnswerSymbolParams struct {
	Items []emitAnswerSymbolItem `json:"items"`
}

type emitAnswerSymbolItem struct {
	Name      string `json:"name"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	Kind      string `json:"kind"`
	Chain     string `json:"chain,omitempty"`
	Rationale string `json:"rationale,omitempty"`
}

// EmitAnswerSymbolProducer is the producer string stamped on every
// item the tool ingests. Exported so downstream consumers can identify
// the channel without grepping for a literal.
const EmitAnswerSymbolProducer = "explorer.emit_answer_symbol"

func (t *EmitAnswerSymbol) Name() string { return "emit_answer_symbol" }

func (t *EmitAnswerSymbol) Description() string {
	return "Emit one or more AnswerSymbol items as the structured answer slate for an enumeration / " +
		"list_of_symbols / call_chain question. Call this from the extractor (Turn B) AFTER you have " +
		"finished reading Turn A's investigation transcript, with one item per terminal symbol the " +
		"final answer should list. The batched 'items' array preserves the 'one tool call per " +
		"answer batch' write pattern; do not call this tool once per item. Every item MUST cite " +
		"name, file (repo-relative path), line (gutter line number, never estimated), and kind " +
		"(function/method/type/struct/class/const/var). Items with line == 0 are REJECTED. Items " +
		"whose file path lives inside the per-trace WorkDir (a temporary blob directory) are " +
		"REJECTED — that is a sign the LLM mistook a tool-output blob for a repo file."
}

func (t *EmitAnswerSymbol) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "items": {
      "type": "array",
      "description": "Batch of answer-symbol items extracted from the investigation transcript. Send the full batch in one call — do not invoke the tool per item.",
      "items": {
        "type": "object",
        "properties": {
          "name":      {"type": "string", "description": "Symbol identifier, exactly as it appears in source. Required."},
          "file":      {"type": "string", "description": "Repository-relative file path the symbol is defined in. Required. MUST NOT be a path inside the per-trace WorkDir."},
          "line":      {"type": "integer", "description": "First line of the symbol definition, taken EXACTLY from the read_file gutter. Required and must be > 0 — items with line == 0 are rejected."},
          "kind":      {"type": "string", "enum": ["function", "func", "method", "type", "struct", "class", "const", "var"], "description": "Symbol category. function/func and method are normalized; struct/class/type are kept as-given."},
          "chain":     {"type": "string", "description": "Optional resolution chain text that yielded this symbol (e.g. 'AgentExplorer registers NewExplorerAgent which returns explorerEvaluator.Name() = explorer'). Empty when the symbol is a direct read."},
          "rationale": {"type": "string", "description": "Optional one-sentence rationale for why this terminal was selected. Keep concise."}
        },
        "required": ["name", "file", "line", "kind"]
      }
    }
  },
  "required": ["items"]
}`)
}

func (t *EmitAnswerSymbol) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	now := time.Now()
	if ctx == nil || ctx.Mutable == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_answer_symbol requires BusContext.Mutable; the caller did not provide one (sub-agents are not supported)",
			Timestamp: now,
		}, nil
	}

	dec := json.NewDecoder(bytes.NewReader(params))
	dec.DisallowUnknownFields()
	var p emitAnswerSymbolParams
	if err := dec.Decode(&p); err != nil {
		return failEmit(t.Name(), now, "invalid params: %v", err)
	}
	if len(p.Items) == 0 {
		return failEmit(t.Name(), now, "items is empty; emit at least one answer-symbol object per call")
	}

	workDir := strings.TrimSpace(ctx.WorkDir)
	built := make([]types.AnswerSymbol, 0, len(p.Items))
	for i, in := range p.Items {
		sym, perr := buildEmitAnswerSymbolItem(in, i, workDir)
		if perr != nil {
			return failEmit(t.Name(), now, "%v", perr)
		}
		built = append(built, sym)
	}

	ctx.Mutable.AppendEmittedAnswerSymbols(built)

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   renderEmitAnswerSymbolSummary(built),
		Timestamp: now,
	}, nil
}

// buildEmitAnswerSymbolItem validates a single decoded item and
// converts it into a types.AnswerSymbol. Validation is structural
// (path-shape, prefix, line bounds), never wordlist-based.
func buildEmitAnswerSymbolItem(in emitAnswerSymbolItem, index int, workDir string) (types.AnswerSymbol, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return types.AnswerSymbol{}, fmt.Errorf("items[%d]: name is required", index)
	}
	file := strings.TrimSpace(in.File)
	if file == "" {
		return types.AnswerSymbol{}, fmt.Errorf("items[%d]: file is required", index)
	}
	if !emitLooksLikePath(file) {
		return types.AnswerSymbol{}, fmt.Errorf("items[%d]: file %q does not look like a repo-relative file path", index, in.File)
	}
	if isInsideWorkDir(file, workDir) {
		return types.AnswerSymbol{}, fmt.Errorf("items[%d]: file %q lives inside the per-trace WorkDir (%s) — that is a tool-output blob, not a repo file. Re-cite the original repo path that the blob was extracted from.", index, in.File, workDir)
	}
	if in.Line <= 0 {
		return types.AnswerSymbol{}, fmt.Errorf("items[%d]: line must be > 0 (got %d). Pattern 2 line-hallucination guard — every answer symbol needs a concrete gutter line.", index, in.Line)
	}
	kindKey := strings.ToLower(strings.TrimSpace(in.Kind))
	kind, ok := emitAnswerSymbolAllowedKinds[kindKey]
	if !ok {
		return types.AnswerSymbol{}, fmt.Errorf("items[%d]: unknown kind %q (allowed: function, func, method, type, struct, class, const, var)", index, in.Kind)
	}

	return types.AnswerSymbol{
		Name:      name,
		File:      file,
		Line:      in.Line,
		Kind:      kind,
		Chain:     strings.TrimSpace(in.Chain),
		Rationale: strings.TrimSpace(in.Rationale),
	}, nil
}

// isInsideWorkDir reports whether path is inside (or equal to) the
// per-trace WorkDir. Empty workDir means "no blob staging area
// configured" and the check is skipped — unit tests with a
// zero-value BusContext, and CI runs without temp scratch space, must
// not falsely reject every cite.
//
// The check uses a normalized prefix match: both sides have leading/
// trailing slashes trimmed and "/" appended to the workDir comparison
// key so that "/tmp/codrax-trace-foo" does not falsely match
// "/tmp/codrax-trace-foo-sibling/file.go". This is the same shape
// used by the docs/bug-blob-file-path-leak-into-final-answer.md
// short-term mitigation: structural prefix only, no extension list.
func isInsideWorkDir(filePath, workDir string) bool {
	if workDir == "" || filePath == "" {
		return false
	}
	clean := strings.TrimRight(workDir, "/")
	if clean == "" {
		return false
	}
	if filePath == clean {
		return true
	}
	return strings.HasPrefix(filePath, clean+"/")
}

func renderEmitAnswerSymbolSummary(items []types.AnswerSymbol) string {
	var b strings.Builder
	fmt.Fprintf(&b, "emit_answer_symbol accepted %d item(s)\n", len(items))
	for _, it := range items {
		fmt.Fprintf(&b, "  %s (%s:%d) [%s]\n", it.Name, it.File, it.Line, it.Kind)
	}
	return b.String()
}
