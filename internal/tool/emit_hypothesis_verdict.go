package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// EmitHypothesisVerdict is the structured channel through which the
// extractor agent (P2.1 Turn B) emits a verdict for each hypothesis
// in AnalysisIR.HypothesisSet. It is the LLM-write half of the D7
// deferred item from project_p1_3_deferred_items.md: P1.3 wired
// hdp.Plan/Bind at analyze time + gate.Run at quality-gate time, but
// nothing wrote back the per-hypothesis verdict during exploration.
// P2.1 Turn B closes that loop.
//
// Status enum is closed: confirmed, rejected, inconclusive.
//   - confirmed: evidence in the transcript supports the hypothesis
//   - rejected:  evidence in the transcript falsifies the hypothesis
//   - inconclusive: the hypothesis was investigated but evidence is
//                   insufficient. This is structurally distinct from
//                   the IR-default "unknown" (= never investigated).
//
// HypothesisID is matched against AnalysisIR.HypothesisSet[*].ID at
// drain time (in MutableState.MarkHypothesis, landing in P6). Unknown
// IDs are diagnosed loudly so a typo does not silently disappear a
// real hypothesis.
//
// Citation is required for confirmed/rejected (you cannot confirm or
// falsify without a concrete cite) and optional for inconclusive
// (the whole point of inconclusive is "we looked but couldn't find a
// definitive cite"). The cite shape is the same file:line[-end] used
// by runContractCheck — structural, no extension list.
//
// Like the other emit_* tools: ReadOnly + NonEvidenceTool. Mutating
// BusContext is not a filesystem write, and the verdict carries
// evaluative judgement, not a repo fact.
type EmitHypothesisVerdict struct {
	ReadOnly
	NonEvidenceTool
}

var emitHypothesisVerdictAllowedStatuses = map[string]types.HypothesisStatus{
	"confirmed":    types.HypConfirmed,
	"rejected":     types.HypRejected,
	"inconclusive": types.HypInconclusive,
}

type emitHypothesisVerdictParams struct {
	Items []emitHypothesisVerdictItem `json:"items"`
}

type emitHypothesisVerdictItem struct {
	HypothesisID string `json:"hypothesis_id"`
	Status       string `json:"status"`
	Rationale    string `json:"rationale,omitempty"`
	Citation     string `json:"citation,omitempty"`
}

const EmitHypothesisVerdictProducer = "explorer.emit_hypothesis_verdict"

func (t *EmitHypothesisVerdict) Name() string { return "emit_hypothesis_verdict" }

func (t *EmitHypothesisVerdict) Description() string {
	return "Emit a verdict for each hypothesis from the analyzer's hypothesis set that the " +
		"investigation has reached a conclusion on. Call this during the extraction stage AFTER the " +
		"investigation transcript has been read, with one item per hypothesis you can confidently " +
		"judge. hypothesis_id MUST match a real ID from the analyzer's hypothesis set; typos are " +
		"flagged. status is one of: confirmed (transcript supports it), rejected (transcript " +
		"falsifies it), inconclusive (investigated but evidence is insufficient — distinct from " +
		"'never investigated'). Both confirmed and rejected REQUIRE a citation in the form " +
		"'path:line' or 'path:line-end' — you cannot confirm or falsify without pointing at concrete " +
		"code. Inconclusive verdicts may omit the citation."
}

func (t *EmitHypothesisVerdict) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "items": {
      "type": "array",
      "description": "Batch of hypothesis verdicts. Send the full batch in one call — do not invoke the tool per item.",
      "items": {
        "type": "object",
        "properties": {
          "hypothesis_id": {"type": "string", "description": "ID from AnalysisIR.HypothesisSet[*].ID. Required. Unknown IDs are diagnosed."},
          "status":        {"type": "string", "enum": ["confirmed", "rejected", "inconclusive"], "description": "Verdict. confirmed/rejected require a citation; inconclusive may omit it."},
          "rationale":     {"type": "string", "description": "Rationale for the verdict — explain the mechanism or invariant that produced the status. Reference load-bearing identifiers with inline `+"`"+`code`+"`"+`. Strongly recommended."},
          "citation":      {"type": "string", "description": "Concrete code anchor in the form 'path:line' or 'path:line-end'. Required when status is confirmed or rejected."}
        },
        "required": ["hypothesis_id", "status"]
      }
    }
  },
  "required": ["items"]
}`)
}

func (t *EmitHypothesisVerdict) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	now := time.Now()
	if ctx == nil || ctx.Mutable == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_hypothesis_verdict requires BusContext.Mutable; the caller did not provide one (sub-agents are not supported)",
			Timestamp: now,
		}, nil
	}

	dec := json.NewDecoder(bytes.NewReader(params))
	dec.DisallowUnknownFields()
	var p emitHypothesisVerdictParams
	if err := dec.Decode(&p); err != nil {
		return failEmit(t.Name(), now, "invalid params: %v", err)
	}
	if len(p.Items) == 0 {
		return failEmit(t.Name(), now, "items is empty; emit at least one verdict object per call")
	}

	built := make([]types.HypothesisVerdict, 0, len(p.Items))
	for i, in := range p.Items {
		v, perr := buildEmitHypothesisVerdictItem(in, i)
		if perr != nil {
			return failEmit(t.Name(), now, "%v", perr)
		}
		built = append(built, v)
	}

	ctx.Mutable.AppendEmittedHypothesisVerdicts(built)

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   renderEmitHypothesisVerdictSummary(built),
		Timestamp: now,
	}, nil
}

// buildEmitHypothesisVerdictItem validates one decoded item and
// converts it into a types.HypothesisVerdict. All validation is
// structural — no wordlist checks, no IR lookup (the cross-reference
// against HypothesisSet[*].ID is the drain layer's responsibility,
// see MutableState.MarkHypothesis in P6).
func buildEmitHypothesisVerdictItem(in emitHypothesisVerdictItem, index int) (types.HypothesisVerdict, error) {
	hypID := strings.TrimSpace(in.HypothesisID)
	if hypID == "" {
		return types.HypothesisVerdict{}, fmt.Errorf("items[%d]: hypothesis_id is required", index)
	}
	statusKey := strings.ToLower(strings.TrimSpace(in.Status))
	status, ok := emitHypothesisVerdictAllowedStatuses[statusKey]
	if !ok {
		return types.HypothesisVerdict{}, fmt.Errorf("items[%d]: unknown status %q (allowed: confirmed, rejected, inconclusive)", index, in.Status)
	}
	citation := strings.TrimSpace(in.Citation)
	rationale := strings.TrimSpace(in.Rationale)

	// confirmed REQUIRES a concrete citation — asserting "this is the
	// case" without pointing at a line is the exact failure mode this
	// gate exists to prevent. rejected can be justified two ways:
	//   1. Positive counter-evidence — a file:line that contradicts
	//      the hypothesis. Preferred when available.
	//   2. Absence of evidence — the hypothesis claimed something that
	//      would exist in the repo, a thorough investigation found
	//      zero matches, so the hypothesis is rejected by absence
	//      ("no .py files" → "no Python bindings" is rejected). A
	//      line citation for absence does not exist; the rationale
	//      MUST be non-empty and load-bearing.
	// inconclusive requires neither — it is the honest "investigated
	// but could not decide" verdict.
	switch status {
	case types.HypConfirmed:
		if citation == "" {
			return types.HypothesisVerdict{}, fmt.Errorf("items[%d]: status %q requires a citation (path:line or path:line-end). Use 'inconclusive' if you cannot point at concrete code.", index, status)
		}
		if !looksLikeCitation(citation) {
			return types.HypothesisVerdict{}, fmt.Errorf("items[%d]: citation %q does not look like 'path:line' or 'path:line-end'", index, in.Citation)
		}
	case types.HypRejected:
		if citation != "" {
			// When provided, it must still be shape-valid.
			if !looksLikeCitation(citation) {
				return types.HypothesisVerdict{}, fmt.Errorf("items[%d]: citation %q does not look like 'path:line' or 'path:line-end'", index, in.Citation)
			}
		} else if rationale == "" {
			// Absence-based rejection requires rationale to carry the
			// load since there is no cite to anchor on.
			return types.HypothesisVerdict{}, fmt.Errorf("items[%d]: status 'rejected' without a citation requires a non-empty rationale explaining the absence (e.g. 'no .py files found in repo').", index)
		}
	}

	return types.HypothesisVerdict{
		HypothesisID: hypID,
		Status:       status,
		Rationale:    rationale,
		Citation:     citation,
	}, nil
}

// looksLikeCitation reports whether s is shaped like 'path:line' or
// 'path:line-end'. Reuses the same structural shape the contract
// checker accepts (no curated extension list, accepts nested paths
// and bare README.md). Logic is intentionally simple: split on the
// LAST ':' so paths containing ':' (rare on Unix, possible) still
// parse, then verify the trailing segment is a positive integer or
// 'int-int' range.
func looksLikeCitation(s string) bool {
	if !emitLooksLikePath(s) {
		return false
	}
	idx := strings.LastIndex(s, ":")
	if idx <= 0 || idx == len(s)-1 {
		return false
	}
	pathPart := s[:idx]
	linePart := s[idx+1:]
	if !emitLooksLikePath(pathPart) {
		return false
	}
	// linePart is either "N" or "N-M" with N, M positive ints.
	if dash := strings.Index(linePart, "-"); dash > 0 {
		first := linePart[:dash]
		second := linePart[dash+1:]
		return isPositiveInt(first) && isPositiveInt(second)
	}
	return isPositiveInt(linePart)
}

func isPositiveInt(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	// Reject leading zeros like "0", "01" — line 0 fails the
	// positive-line guard everywhere else in the codebase.
	if s == "0" {
		return false
	}
	return true
}

func renderEmitHypothesisVerdictSummary(items []types.HypothesisVerdict) string {
	var b strings.Builder
	fmt.Fprintf(&b, "emit_hypothesis_verdict accepted %d verdict(s)\n", len(items))
	counts := make(map[types.HypothesisStatus]int)
	for _, v := range items {
		counts[v.Status]++
	}
	for _, st := range []types.HypothesisStatus{types.HypConfirmed, types.HypRejected, types.HypInconclusive} {
		if n := counts[st]; n > 0 {
			fmt.Fprintf(&b, "  %s: %d\n", st, n)
		}
	}
	return b.String()
}
