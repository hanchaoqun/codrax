package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/authority"
	"github.com/hanchaoqun/codrax/internal/tool/ground"
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
//     memory/project_blob_file_leak_unresolved.md). The file
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

// Allowed kinds. Delegated to types.NormalizeAnswerSymbolKind for
// the single-source-of-truth closed taxonomy — includes cross-
// language shapes (trait / protocol / module / package / crate /
// macro / decorator / annotation …) and the non-symbol terminal
// (literal) used when the resolution chain resolves to a value
// rather than a code identifier.

type emitAnswerSymbolParams struct {
	Items        []emitAnswerSymbolItem `json:"items"`
	Completeness string                 `json:"completeness"`
	// DeclaredCount is the LLM's self-declared item count.
	// Optional; when set must equal len(Items). Pre-commit-49
	// the description carried prose hints about expected
	// counts but no machine-checkable assertion — the LLM
	// could say "found 47 callers" and emit a 12-item slate
	// with no fail. Now: when the LLM provides a count, the
	// validator cross-checks; mismatch rejects with hint to
	// either revise items or revise count. Zero / absent =
	// no claim (back-compat: pre-commit-49 calls have no
	// field, so JSON decode leaves it at 0).
	DeclaredCount int `json:"count,omitempty"`
}

// emitAnswerSymbolAllowedCompleteness is the closed set of valid
// completeness claims the extractor (Turn B) can attach to a slate.
// Mirrors types.CompletenessClaim — duplicated here to keep the
// schema validator loop self-contained and avoid a cross-package
// string lookup during high-frequency tool calls.
var emitAnswerSymbolAllowedCompleteness = map[string]types.CompletenessClaim{
	"complete":    types.CompletenessComplete,
	"lower_bound": types.CompletenessLowerBound,
	"unknown":     types.CompletenessUnknown,
}

type emitAnswerSymbolItem struct {
	Name      string  `json:"name"`
	File      string  `json:"file"`
	Line      FlexInt `json:"line"` // FlexInt absorbs LLMs that emit "42" instead of 42
	Kind      string  `json:"kind"`
	Chain     string  `json:"chain,omitempty"`
	Rationale string  `json:"rationale,omitempty"`
}

// EmitAnswerSymbolProducer is the producer string stamped on every
// item the tool ingests. Exported so downstream consumers can identify
// the channel without grepping for a literal.
const EmitAnswerSymbolProducer = "explorer.emit_answer_symbol"

func (t *EmitAnswerSymbol) Name() string { return "emit_answer_symbol" }

func (t *EmitAnswerSymbol) Description() string {
	return "Emit one or more answer-symbol items as the structured answer slate for an enumeration / " +
		"list_of_symbols / call_chain question. Call this during the extraction stage AFTER the " +
		"investigation transcript has been read, with one item per terminal symbol the final answer " +
		"should list. The batched 'items' array preserves the 'one tool call per answer batch' write " +
		"pattern; do not call this tool once per item. Every item MUST cite name, file (repo-relative " +
		"path), line (gutter line number, never estimated), and kind. Kind is a closed cross-language " +
		"taxonomy (function, method, type, struct, class, interface, trait, enum, protocol, const, " +
		"var, field, property, module, package, crate, namespace, macro, decorator, annotation, " +
		"literal) — pick the shape that matches the symbol's language and role. Use `literal` when " +
		"the answer terminal is a value rather than a code identifier (e.g. a string returned by " +
		"Name(), a config key's default, an enum member's literal value). Items with line == 0 are " +
		"REJECTED. Items whose file path lives inside the per-trace work directory (a temporary blob " +
		"directory) are REJECTED — that is a sign the LLM mistook a tool-output blob for a repo " +
		"file. The call MUST also carry a 'completeness' field declaring the set-level authority of " +
		"the slate: 'complete' (these are ALL the answers), 'lower_bound' (these are confirmed " +
		"present but more may exist), or 'unknown' (investigated but no definitive claim). Claiming " +
		"'complete' is a falsifiable honesty assertion — it is cross-checked against the " +
		"expected answer count (the larger of: how many items the investigation found, and " +
		"how many the classification declared required), and downgraded to 'lower_bound' on " +
		"mismatch with a warning."
}

func (t *EmitAnswerSymbol) Parameters() json.RawMessage {
	// Kind enum is sourced from types.AnswerSymbolKindSchemaEnum so
	// schema and validator never drift. Building the schema with a
	// Sprintf is cheap — Parameters() runs once per dispatch, not per
	// tool call.
	return json.RawMessage(fmt.Sprintf(`{
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
          "kind":      {"type": "string", "enum": [%s], "description": "Closed cross-language taxonomy. Canonical kinds cover callables (function/method), type-shape definitions (type/struct/class/interface/trait/enum/protocol), data bindings (const/var/field/property), module scopes (module/package/crate/namespace), metaprogramming (macro/decorator/annotation), and non-symbol terminals (literal). Language shorthand is accepted and normalised (func/fn → function). Use 'literal' when the terminal is a value (string/number/bool returned by a Name()/Type()/Kind() method, a config default, an enum value) rather than a code identifier."},
          "chain":     {"type": "string", "description": "Optional resolution chain text that yielded this symbol (e.g. 'X registers Y which returns Y.Name() = \"foo\"'). Empty when the symbol is a direct read."},
          "rationale": {"type": "string", "description": "Optional rationale for why this terminal answers the question — what role it plays, not just where it lives. Keep it concrete but natural; downstream rendering presents this as a column alongside the location, so restating file:line is a regression."}
        },
        "required": ["name", "file", "line", "kind"]
      }
    },
    "completeness": {
      "type": "string",
      "enum": ["complete", "lower_bound", "unknown"],
      "description": "Set-level authority claim for the slate. REQUIRED. 'complete' = these are ALL the answers (cross-checked against the expected answer count — the larger of: how many items the investigation found, and how many the classification declared required; downgraded to lower_bound on mismatch). 'lower_bound' = these are confirmed present but more may exist (honest default when a partial slate is the best available). 'unknown' = investigated but no definitive verdict (downstream rendering drops the section entirely)."
    },
    "count": {
      "type": "integer",
      "minimum": 0,
      "description": "Optional self-declared item count. When set, the tool validator REQUIRES len(items) == count and rejects on mismatch (with a hint to either trim/extend items or revise the count). Use this when your prose summary says 'found 47 callers' to make the count machine-checkable; omit (or set 0) to skip the cross-check. The check defends against quantitative claims drifting from the actual emitted slate."
    }
  },
  "required": ["items", "completeness"]
}`, types.AnswerSymbolKindSchemaEnum()))
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

	// Completeness is REQUIRED (P2.1 honesty contract). The schema
	// declares it as required so a compliant JSON-schema LLM will
	// always include it; the Go-side check defends against a lenient
	// model that drops the field at runtime. An empty string or any
	// value outside the closed enum is rejected loudly rather than
	// silently coerced to "unknown" — silent coercion is exactly the
	// class of bug P2.1 is trying to close (UNRESOLVED #1).
	claimRaw := strings.ToLower(strings.TrimSpace(p.Completeness))
	if claimRaw == "" {
		return failEmit(t.Name(), now, "completeness field is required — choose one of: complete, lower_bound, unknown. See the tool description for the honesty contract.")
	}
	claim, claimOK := emitAnswerSymbolAllowedCompleteness[claimRaw]
	if !claimOK {
		return failEmit(t.Name(), now, "unknown completeness value %q (allowed: complete, lower_bound, unknown)", p.Completeness)
	}

	// Empty-set handling. "How many Python files?" legitimately
	// returns 0; "List all deprecated methods" legitimately returns
	// zero items. Reject items=[] ONLY when paired with
	// completeness=lower_bound, which would assert "more exist than
	// these" — a nonsensical claim on zero items. complete (zero
	// confirmed, none more) and unknown (could not determine) are
	// both honest outcomes on an empty set and pass through.
	if len(p.Items) == 0 {
		if claim == types.CompletenessLowerBound {
			return failEmit(t.Name(), now, "items is empty but completeness=lower_bound claims more items exist; use completeness=complete for a confirmed empty set or completeness=unknown when you cannot determine")
		}
	}
	if ctx.AnalysisIR != nil && ctx.AnalysisIR.RequestModel.EnumerationBoundary != nil {
		boundary := ctx.AnalysisIR.RequestModel.EnumerationBoundary
		if boundary.DeclaredCount > 0 && len(p.Items) > boundary.DeclaredCount {
			return failEmit(t.Name(), now,
				"the user explicitly requested a bounded principal set %q (%d item(s)), so emit_answer_symbol must keep items[] within that boundary; got %d item(s)",
				boundary.SourceQuote, boundary.DeclaredCount, len(p.Items))
		}
	}
	// Self-declared count check (commit 49 read-mode Gap 2).
	// When the LLM emits a count, len(items) must match. This
	// closes the gap where prose summary said "found 47 callers"
	// while items[] held 12 entries — pre-commit-49 the system
	// had no machine-checkable structural assertion on the
	// quantity (only a description-level hint). Mismatch ejects
	// with a hint nudging either trim or recount. Zero / absent
	// = no claim (back-compat).
	if p.DeclaredCount > 0 && p.DeclaredCount != len(p.Items) {
		return failEmit(t.Name(), now,
			"declared count=%d does not match items length=%d. Either revise the count to match the slate (if the slate is right) OR add/remove items to match the count (if the count is right). Quantitative claims in the answer must agree with the structured slate.",
			p.DeclaredCount, len(p.Items))
	}

	workDir := strings.TrimSpace(ctx.WorkDir)
	var bundle *types.LogBundle
	if ctx.Mutable != nil {
		bundle = ctx.Mutable.LogTriage()
	}
	// Fix D: floor grounding. Build the ground context (TurnA snapshot
	// + dispatch buffer + bus tool results) so each item can be
	// line-text verified against what the investigation actually read.
	// When the cited file was read_file'd earlier in the pipeline and
	// the claimed line ±2 does not contain the symbol name, the item
	// is dropped before reaching the deterministic floor — catches the
	// u3a regression class where the extractor LLM emitted items whose
	// file:line pointed at a call-site line rather than the symbol's
	// definition line.
	groundCtx := ground.BuildContext(ctx)
	stepCandidates := compiledStepCandidateNames(types.BuildAnswerSurfacePlanForBusContext(ctx))
	built := make([]types.AnswerSymbol, 0, len(p.Items))
	var dropped []string
	for i, in := range p.Items {
		sym, perr := buildEmitAnswerSymbolItem(in, i, workDir, bundle, groundCtx, stepCandidates)
		if perr != nil {
			dropped = append(dropped, perr.Error())
			continue
		}
		built = append(built, sym)
	}

	// Partial-drop: when some items are invalid but others survive,
	// accept the valid slate and surface the drop reasons in Summary.
	// Hard-fail only when every item was rejected. This closes the
	// 2026-04-19 regression where one `line=0` hallucination killed
	// the whole ProposeSubAgents list → finalizer answered 0.
	if len(built) == 0 && len(dropped) > 0 {
		return failEmit(t.Name(), now, "%s", strings.Join(dropped, "; "))
	}

	// AuthorityCeiling axis: pin each AnswerSymbol's Authority to the
	// WEAKEST ceiling among supporting evidence items. The "weakest"
	// rule is load-bearing — if even one supporting evidence is
	// conditional/historical, the symbol's prose must hedge.
	{
		evidence := ctx.Mutable.EmittedEvidence()
		for i := range built {
			built[i].Authority = authority.LowestAuthorityFor(
				evidence, built[i].File, built[i].Line)
		}
	}

	// Set semantics: this REPLACES any previous slate + claim. On a
	// retry after validation downgrade, the LLM's second call wins
	// over the first. Phase 9's extractor-side validator runs at
	// ParseOutput time, not here, so that it has access to the full
	// AgentContext (Turn A baseline + AnalysisIR MustInclude) which
	// BusContext alone does not expose.
	ctx.Mutable.SetEmittedAnswerSymbols(built, claim)
	// Commit 55 Batch A.3 — persist the LLM's self-declared count
	// so the finalizer-stage Answer Shape Oracle can cross-check
	// `declared count` vs `rendered len(doc.Symbols)` to catch
	// post-emit item-drop drift.
	ctx.Mutable.SetEmittedAnswerSymbolDeclaredCount(p.DeclaredCount)

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   renderEmitAnswerSymbolSummary(built, claim, dropped),
		Timestamp: now,
	}, nil
}

// buildEmitAnswerSymbolItem validates a single decoded item and
// converts it into a types.AnswerSymbol. Validation is structural
// (path-shape, prefix, line bounds), never wordlist-based.
//
// bundle carries the log-triage context (nil when no log was
// attached). When the bundle is flagged external-source — structured
// errors extracted but zero frames resolved to repo files — the
// "file required" and "line > 0" rejections redirect the LLM toward
// symbols_completeness=unknown + empty items[] rather than letting
// it hammer the gates with manufactured anchors derived from log
// message names. This is the symmetric sibling of the kind=absent
// retirement on emit_evidence: both channels reject an unsatisfiable
// item and point at the proper whole-shape escape.
func buildEmitAnswerSymbolItem(in emitAnswerSymbolItem, index int, workDir string, bundle *types.LogBundle, groundCtx *ground.Context, stepCandidates map[string]string) (types.AnswerSymbol, error) {
	externalSource := bundle.IsExternalSource()
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return types.AnswerSymbol{}, fmt.Errorf("items[%d]: name is required", index)
	}
	file := strings.TrimSpace(in.File)
	if file == "" {
		if externalSource {
			return types.AnswerSymbol{}, fmt.Errorf("items[%d] (%s): file is empty — the attached log is external-source (no frames resolved to this repo), so symbols cannot be anchored at a repo file:line. For list_of_symbols or multi-topic explanation answers drawn from external-log semantics, set symbols_completeness=\"unknown\" and omit items[] entirely; the summary prose carries the answer. Do NOT manufacture a file name for log-message identifiers that have no repo counterpart.", index, name)
		}
		return types.AnswerSymbol{}, fmt.Errorf("items[%d]: file is required", index)
	}
	if !emitLooksLikePath(file) {
		return types.AnswerSymbol{}, fmt.Errorf("items[%d]: file %q does not look like a repo-relative file path", index, in.File)
	}
	if isInsideWorkDir(file, workDir) {
		return types.AnswerSymbol{}, fmt.Errorf("items[%d]: file %q lives inside the per-trace WorkDir (%s) — that is a tool-output blob, not a repo file. Re-cite the original repo path that the blob was extracted from.", index, in.File, workDir)
	}
	lineN := in.Line.Int()
	if lineN <= 0 {
		if externalSource {
			return types.AnswerSymbol{}, fmt.Errorf("items[%d] (%s): line=0 is not valid — the attached log is external-source (no frames resolved to this repo), so symbols cannot be anchored at a concrete gutter line. For list_of_symbols or multi-topic explanation answers drawn from external-log semantics, set symbols_completeness=\"unknown\" and omit items[] entirely; the summary prose carries the answer. Do NOT manufacture a file:line for log-message identifiers that have no repo counterpart.", index, name)
		}
		return types.AnswerSymbol{}, fmt.Errorf("items[%d]: line must be > 0 (got %d). Pattern 2 line-hallucination guard — every answer symbol needs a concrete gutter line.", index, lineN)
	}
	kind, ok := types.NormalizeAnswerSymbolKind(in.Kind)
	if !ok {
		return types.AnswerSymbol{}, fmt.Errorf("items[%d]: unknown kind %q; see types.AllAnswerSymbolKinds for the closed taxonomy (function, method, type, struct, class, interface, trait, enum, protocol, const, var, field, property, module, package, crate, namespace, macro, decorator, annotation, literal)", index, in.Kind)
	}

	// Fix D: floor grounding. When the cited file was read_file'd
	// earlier in the pipeline AND the claimed line (±2) does not bear
	// the symbol name as a code identifier, drop the item. If the file
	// was never read (HasFileInIndex=false), we cannot verify — let
	// the item through and rely on downstream finalizer grounding.
	//
	// Leaf-name fallback: a name like "Type.Method" is accepted when
	// the line bears either the full form or the "Method" segment
	// alone (typical for method definitions where the receiver is
	// already visible in the surrounding def line).
	if groundCtx != nil && ground.HasFileInIndex(groundCtx, file) {
		if candidate := stepCandidates[compiledStepCandidateKey(file, lineN)]; candidate != "" &&
			answerSymbolNameGrounds(groundCtx, file, lineN, candidate) &&
			!answerSymbolNameGrounds(groundCtx, file, lineN, name) {
			name = candidate
		}
	}
	if groundCtx != nil && ground.HasFileInIndex(groundCtx, file) {
		if _, ok := ground.VerifyLineAnchor(groundCtx, file, lineN, name, 2); !ok {
			leaf := name
			if idx := strings.LastIndex(name, "."); idx >= 0 && idx+1 < len(name) {
				leaf = name[idx+1:]
			}
			if leaf == name {
				return types.AnswerSymbol{}, fmt.Errorf("items[%d] (%s): name %q is not corroborated by the cited line (the line and ±2 neighbours at %s:%d contain no identifier token matching it). Cite the line where the symbol is DEFINED, not a line that merely references it (e.g. a call site). If you picked this line from an evidence bullet of the shape '[relationship] X calls Y — file:line', that line is the call site (Y's callsite), not X's definition — X is defined at a different line", index, name, name, file, lineN)
			}
			if _, ok := ground.VerifyLineAnchor(groundCtx, file, lineN, leaf, 2); !ok {
				return types.AnswerSymbol{}, fmt.Errorf("items[%d] (%s): name %q is not corroborated by the cited line (the line and ±2 neighbours at %s:%d contain neither %q nor its leaf %q). Cite the line where the symbol is DEFINED, not a line that merely references it (e.g. a call site). If you picked this line from an evidence bullet of the shape '[relationship] X calls Y — file:line', that line is the call site (Y's callsite), not X's definition — X is defined at a different line", index, name, name, file, lineN, name, leaf)
			}
		}
	}

	return types.AnswerSymbol{
		Name:      name,
		File:      file,
		Line:      lineN,
		Kind:      kind,
		Chain:     strings.TrimSpace(in.Chain),
		Rationale: strings.TrimSpace(in.Rationale),
	}, nil
}

func answerSymbolNameGrounds(groundCtx *ground.Context, file string, line int, name string) bool {
	if groundCtx == nil || strings.TrimSpace(file) == "" || line <= 0 || strings.TrimSpace(name) == "" {
		return false
	}
	if _, ok := ground.VerifyLineAnchor(groundCtx, file, line, name, 2); ok {
		return true
	}
	if idx := strings.LastIndex(name, "."); idx >= 0 && idx+1 < len(name) {
		if _, ok := ground.VerifyLineAnchor(groundCtx, file, line, name[idx+1:], 2); ok {
			return true
		}
	}
	return false
}

func compiledStepCandidateNames(plan *types.AnswerSurfacePlan) map[string]string {
	if plan == nil || len(plan.StepBackbone) == 0 {
		return nil
	}
	out := make(map[string]string, len(plan.StepBackbone))
	for _, anchor := range plan.StepBackbone {
		name := strings.TrimSpace(anchor.Name)
		file := strings.TrimSpace(anchor.File)
		if name == "" || file == "" || anchor.Line <= 0 {
			continue
		}
		out[compiledStepCandidateKey(file, anchor.Line)] = name
	}
	return out
}

func compiledStepCandidateKey(file string, line int) string {
	return strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`)) + "#" + fmt.Sprintf("%d", line)
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
// "/tmp/codrax-trace-foo-sibling/file.go". Short-term mitigation for
// the UNRESOLVED blob-file leak bug: structural prefix only, no
// extension list.
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

func renderEmitAnswerSymbolSummary(items []types.AnswerSymbol, claim types.CompletenessClaim, dropped []string) string {
	var b strings.Builder
	claimText := string(claim)
	if claimText == "" {
		claimText = "unknown"
	}
	fmt.Fprintf(&b, "emit_answer_symbol accepted %d item(s) with completeness=%s\n", len(items), claimText)
	for _, it := range items {
		fmt.Fprintf(&b, "  %s (%s:%d) [%s]\n", it.Name, it.File, it.Line, it.Kind)
	}
	if len(dropped) > 0 {
		fmt.Fprintf(&b, "dropped %d invalid item(s):\n", len(dropped))
		for _, d := range dropped {
			fmt.Fprintf(&b, "  - %s\n", d)
		}
	}
	return b.String()
}
