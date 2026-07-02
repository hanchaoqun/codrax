package types

import (
	"fmt"
	"sort"
	"strings"
)

// ToolResultPreserveFunc identifies a high-value tool result that may be kept
// even when lower-value entries would otherwise be dropped by a Turn-A handoff
// bound. Callers choose the precise preservation rule for their merge point.
type ToolResultPreserveFunc func(ToolResult) bool

// MCPResponsePreserveFunc is the MCP companion to ToolResultPreserveFunc.
type MCPResponsePreserveFunc func(MCPResponse) bool

// TurnAToolResultBytes is the deterministic byte accounting for Turn-A handoff
// bounds: the carried text surfaces of the result plus attached typed
// observation rows. It intentionally over-counts a little rather than
// marshaling for an exact figure.
func TurnAToolResultBytes(r ToolResult) int {
	n := len(r.ToolName) + len(r.Summary) + len(r.RawRef) + 64
	if r.Handoff != nil {
		n += ToolHandoffCarrierBytes(*r.Handoff)
	}
	for _, obs := range r.Observations {
		n += len(obs.ID) + len(obs.ClaimKey) + len(obs.Subject) + len(obs.Predicate) +
			len(obs.Object) + len(obs.Value) + len(obs.Summary) + len(obs.RawExcerpt)
		for _, note := range obs.RichNotes {
			n += len(note)
		}
		for _, ref := range obs.SupportRefs {
			n += len(ref)
		}
		n += 64
	}
	return n
}

// ToolResultTruncationSummary is the typed record of tool results dropped by
// Turn-A window-capture/merge budgets. It exists so checkpoint and
// continuation prompts can disclose "truncated N tool results (tool×count)"
// instead of silently shrinking history (Batch E1, 2026-07-02). Counts are
// cumulative across retry-window merges. System-derived only; never
// model-emitted, never a gate input.
type ToolResultTruncationSummary struct {
	Dropped int            `json:"dropped,omitempty"`
	ByTool  map[string]int `json:"by_tool,omitempty"`
}

// Active reports whether any tool result has been dropped.
func (s *ToolResultTruncationSummary) Active() bool {
	return s != nil && s.Dropped > 0
}

// Label renders the summary as "N dropped (tool×count, ...)" for advisory
// checkpoint/continuation prompt lines.
func (s *ToolResultTruncationSummary) Label() string {
	if !s.Active() {
		return ""
	}
	categories := formatCategoryCounts(s.ByTool)
	if categories == "" {
		return fmt.Sprintf("%d dropped", s.Dropped)
	}
	return fmt.Sprintf("%d dropped (%s)", s.Dropped, categories)
}

// CloneToolResultTruncationSummary deep-copies a truncation summary.
func CloneToolResultTruncationSummary(in *ToolResultTruncationSummary) *ToolResultTruncationSummary {
	if in == nil {
		return nil
	}
	out := &ToolResultTruncationSummary{Dropped: in.Dropped}
	if len(in.ByTool) > 0 {
		out.ByTool = make(map[string]int, len(in.ByTool))
		for tool, count := range in.ByTool {
			out.ByTool[tool] = count
		}
	}
	return out
}

// MergeToolResultTruncationSummaries sums truncation summaries across
// capture/merge sites. Dropped entries never survive into the next merge
// input, so summation cannot double-count.
func MergeToolResultTruncationSummaries(parts ...*ToolResultTruncationSummary) *ToolResultTruncationSummary {
	var out *ToolResultTruncationSummary
	for _, part := range parts {
		if !part.Active() {
			continue
		}
		if out == nil {
			out = &ToolResultTruncationSummary{ByTool: map[string]int{}}
		}
		out.Dropped += part.Dropped
		for tool, count := range part.ByTool {
			out.ByTool[tool] += count
		}
	}
	return out
}

// toolResultCarriesDeterministicRuntimeObservation reports whether a tool
// result carries at least one deterministic runtime-artifact observation row
// (trace_query-class producer). This is the precise typed floor signal for
// window-budget retention: such rows summarize a runtime artifact that may be
// gigabytes large and cannot be re-derived by re-running a repo search, while
// grep/read_file output can.
func toolResultCarriesDeterministicRuntimeObservation(r ToolResult) bool {
	for _, obs := range r.Observations {
		if obs.Origin == AnswerEvidenceOriginRuntimeArtifact &&
			runtimeObservationProducerIsDeterministicQuery(obs.Producer) {
			return true
		}
	}
	return false
}

// turnAToolResultRetentionRank orders tool results by retention value for the
// Turn-A window budgets (lower = more valuable, mirroring
// observationRecordRank's convention and its origin/producer weighting at the
// tool-result granularity). Only precise typed signals participate:
// deterministic runtime observation rows rank highest, then typed handoff
// carriers and caller-preserved investigation results, then successful results
// with typed payloads; bare failures (retriable noise) drop first. Within the
// same value class newer results are kept before older ones, preserving the
// previous recency semantics.
func turnAToolResultRetentionRank(r ToolResult, preserve ToolResultPreserveFunc) int {
	rank := 1000
	if toolResultCarriesDeterministicRuntimeObservation(r) {
		rank -= 600
	}
	if r.Handoff != nil {
		rank -= 240
	}
	if preserve != nil && preserve(r) {
		rank -= 160
	}
	if len(r.Observations) > 0 {
		rank -= 80
	}
	if r.SourceInventory != nil || r.CommandMeasurement != nil || r.VCSHistory != nil {
		rank -= 60
	}
	if r.Success {
		rank -= 40
	}
	if r.Repair != nil || r.Refinement != nil {
		rank -= 20
	}
	if strings.TrimSpace(r.Summary) != "" || strings.TrimSpace(r.RawRef) != "" {
		rank -= 10
	}
	return rank
}

// BoundTurnAToolResults applies count + byte caps to a chronological tool
// result slice. See BoundTurnAToolResultsWithTruncation for the retention
// policy; this wrapper discards the truncation summary for callers that only
// need the bounded slice.
func BoundTurnAToolResults(results []ToolResult, countCap, byteCap int, preserve ToolResultPreserveFunc) []ToolResult {
	out, _ := BoundTurnAToolResultsWithTruncation(results, countCap, byteCap, preserve)
	return out
}

// BoundTurnAToolResultsWithTruncation applies count + byte caps to a
// chronological tool result slice and reports what was dropped.
//
// Retention is VALUE-ordered, not oldest-first (Batch E1, 2026-07-02): before
// the previous policy dropped the oldest prefix by timestamp, a retry window
// full of fresh grep noise could evict earlier deterministic trace_query
// observations — the highest-value, non-re-derivable results. Entries are
// ranked by turnAToolResultRetentionRank (newer first within equal value) and
// kept greedily while both caps allow; the output preserves chronological
// order. Two floors survive even a pathological byte budget: at least the
// newest result carrying deterministic runtime observation rows, and at least
// one caller-preserved result (the pre-existing guarantee for the
// structural-empty gate). Slices already inside both caps are returned
// unchanged with a nil summary.
func BoundTurnAToolResultsWithTruncation(results []ToolResult, countCap, byteCap int, preserve ToolResultPreserveFunc) ([]ToolResult, *ToolResultTruncationSummary) {
	if len(results) == 0 || countCap <= 0 || byteCap <= 0 {
		return results, nil
	}
	total := 0
	sizes := make([]int, len(results))
	for i, r := range results {
		sizes[i] = TurnAToolResultBytes(r)
		total += sizes[i]
	}
	if len(results) <= countCap && total <= byteCap {
		return results, nil
	}
	ranks := make([]int, len(results))
	for i, r := range results {
		ranks[i] = turnAToolResultRetentionRank(r, preserve)
	}
	order := make([]int, len(results))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		if ranks[order[a]] != ranks[order[b]] {
			return ranks[order[a]] < ranks[order[b]]
		}
		return order[a] > order[b]
	})
	keep := make([]bool, len(results))
	kept := 0
	keptBytes := 0
	for _, idx := range order {
		if kept >= countCap {
			break
		}
		if keptBytes+sizes[idx] > byteCap {
			continue
		}
		keep[idx] = true
		kept++
		keptBytes += sizes[idx]
	}
	forceKeepNewest(results, keep, toolResultCarriesDeterministicRuntimeObservation)
	if preserve != nil {
		forceKeepNewest(results, keep, preserve)
	}
	out := make([]ToolResult, 0, kept)
	var truncation *ToolResultTruncationSummary
	for i, r := range results {
		if keep[i] {
			out = append(out, r)
			continue
		}
		if truncation == nil {
			truncation = &ToolResultTruncationSummary{ByTool: map[string]int{}}
		}
		truncation.Dropped++
		name := strings.TrimSpace(r.ToolName)
		if name == "" {
			name = "unknown"
		}
		truncation.ByTool[name]++
	}
	if truncation == nil {
		return results, nil
	}
	return out, truncation
}

// forceKeepNewest marks the newest matching result as kept when no matching
// result survived the greedy pass. It may exceed the caps by one entry — the
// same overflow allowance the previous oldest-first policy granted its single
// preserved result.
func forceKeepNewest(results []ToolResult, keep []bool, match func(ToolResult) bool) {
	newestUnkept := -1
	for i := len(results) - 1; i >= 0; i-- {
		if !match(results[i]) {
			continue
		}
		if keep[i] {
			return
		}
		if newestUnkept < 0 {
			newestUnkept = i
		}
	}
	if newestUnkept >= 0 {
		keep[newestUnkept] = true
	}
}

// TurnAMCPResponseBytes is the MCP companion to TurnAToolResultBytes.
func TurnAMCPResponseBytes(r MCPResponse) int {
	n := len(r.ServerName) + len(r.Method) + len(r.Summary) + len(r.RawRef) +
		len(r.PayloadRef) + len(r.RowSetRef) + len(r.PageRef) + len(r.ResourceURI) +
		len(r.MIMEType) + len(r.JSONPointer) + len(r.Selector) + 96
	for _, obs := range r.Observations {
		n += len(obs.Summary) + len(obs.RawRef) + len(obs.PayloadRef) + len(obs.RowSetRef) +
			len(obs.PageRef) + len(obs.ResourceURI) + len(obs.MIMEType) + len(obs.JSONPointer) +
			len(obs.Selector) + 64
	}
	return n
}

// BoundTurnAMCPResponses applies count + byte caps to a chronological MCP
// response slice, dropping oldest entries first and keeping chronological order.
// Slices already inside both caps are returned unchanged.
func BoundTurnAMCPResponses(responses []MCPResponse, countCap, byteCap int, preserve MCPResponsePreserveFunc) []MCPResponse {
	if len(responses) == 0 || countCap <= 0 || byteCap <= 0 {
		return responses
	}
	total := 0
	for _, r := range responses {
		total += TurnAMCPResponseBytes(r)
	}
	if len(responses) <= countCap && total <= byteCap {
		return responses
	}
	start := len(responses)
	kept := 0
	keptBytes := 0
	for i := len(responses) - 1; i >= 0; i-- {
		size := TurnAMCPResponseBytes(responses[i])
		if kept+1 > countCap || keptBytes+size > byteCap {
			break
		}
		kept++
		keptBytes += size
		start = i
	}
	if start == 0 {
		return responses
	}
	out := responses[start:]
	if preserve != nil && !hasPreservedMCPResponse(out, preserve) {
		for i := start - 1; i >= 0; i-- {
			if preserve(responses[i]) {
				preserved := make([]MCPResponse, 0, len(out)+1)
				preserved = append(preserved, responses[i])
				preserved = append(preserved, out...)
				return preserved
			}
		}
	}
	return append([]MCPResponse(nil), out...)
}

func hasPreservedMCPResponse(responses []MCPResponse, preserve MCPResponsePreserveFunc) bool {
	for _, r := range responses {
		if preserve(r) {
			return true
		}
	}
	return false
}

// PreserveSuccessfulToolResultWithPayload is a generic Turn-A fork-merge
// preservation predicate for successful results that carry useful handoff
// material. Agent-specific gates may pass a stricter predicate.
func PreserveSuccessfulToolResultWithPayload(r ToolResult) bool {
	if r.Handoff != nil {
		return true
	}
	return r.Success && (strings.TrimSpace(r.Summary) != "" ||
		strings.TrimSpace(r.RawRef) != "" ||
		len(r.Observations) > 0 ||
		r.Repair != nil)
}

// ToolHandoffCarrierBytes is the deterministic byte accounting companion for
// Turn-A handoff carriers. It intentionally counts typed identity fields rather
// than marshaling, preserving stable behavior while preventing hidden carrier
// growth from bypassing handoff budgets.
func ToolHandoffCarrierBytes(c ToolHandoffCarrier) int {
	n := len(c.ToolName) + len(c.ReasonCode) + len(c.RepairCode) + 64
	if c.Repair != nil {
		n += len(c.Repair.Code) + len(c.Repair.Hint) + 64
		for _, field := range c.Repair.Fields {
			n += len(field)
		}
		for _, target := range c.Repair.Targets {
			n += len(target.File) + len(target.Action) + len(target.Lines)*8 + 32
		}
		for k, v := range c.Repair.Metadata {
			n += len(k) + len(v)
		}
	}
	if c.PlanRepairPack != nil {
		n += len(c.PlanRepairPack.ToolName) + len(c.PlanRepairPack.ReasonCode) +
			len(c.PlanRepairPack.Message) + len(c.PlanRepairPack.RetryInstruction) + 96
		for _, field := range c.PlanRepairPack.FailingFieldPaths {
			n += len(field)
		}
		for _, p := range c.PlanRepairPack.FailingPaths {
			n += len(p)
		}
	}
	if c.SupportedJSON != nil {
		n += len(c.SupportedJSON.ToolName) + len(c.SupportedJSON.ReasonCode) + 64
		for _, field := range c.SupportedJSON.FailingFieldPaths {
			n += len(field)
		}
		for _, field := range c.SupportedJSON.AcceptedFieldPaths {
			n += len(field)
		}
		for k, vals := range c.SupportedJSON.AcceptedEnums {
			n += len(k)
			for _, val := range vals {
				n += len(val)
			}
		}
	}
	for _, ref := range c.AcceptedEvidence {
		n += len(ref.ID) + len(ref.Source) + len(ref.Subject) + len(ref.OwnerSymbol) +
			len(ref.AnchorSymbol) + len(ref.SourcePathRole) + 96
	}
	for _, ref := range c.ObservationRefs {
		n += len(ref.ID) + len(ref.Producer) + len(ref.Source) + len(ref.ClaimKey) +
			len(ref.Subject) + 96
	}
	return n
}

// PreserveSuccessfulMCPResponseWithPayload is the MCP companion used by the
// generic Turn-A fork merge point.
func PreserveSuccessfulMCPResponseWithPayload(r MCPResponse) bool {
	return r.Success && (strings.TrimSpace(r.Summary) != "" ||
		strings.TrimSpace(r.RawRef) != "" ||
		strings.TrimSpace(r.PayloadRef) != "" ||
		strings.TrimSpace(r.RowSetRef) != "" ||
		strings.TrimSpace(r.PageRef) != "" ||
		strings.TrimSpace(r.ResourceURI) != "" ||
		len(r.Observations) > 0)
}
