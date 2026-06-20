package types

import "strings"

// ToolResultPreserveFunc identifies a high-value tool result that may be kept
// even when the oldest prefix would otherwise be dropped by a Turn-A handoff
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

// BoundTurnAToolResults applies count + byte caps to a chronological tool
// result slice, dropping oldest entries first and keeping chronological order.
// Slices already inside both caps are returned unchanged.
func BoundTurnAToolResults(results []ToolResult, countCap, byteCap int, preserve ToolResultPreserveFunc) []ToolResult {
	if len(results) == 0 || countCap <= 0 || byteCap <= 0 {
		return results
	}
	total := 0
	for _, r := range results {
		total += TurnAToolResultBytes(r)
	}
	if len(results) <= countCap && total <= byteCap {
		return results
	}
	start := len(results)
	kept := 0
	keptBytes := 0
	for i := len(results) - 1; i >= 0; i-- {
		size := TurnAToolResultBytes(results[i])
		if kept+1 > countCap || keptBytes+size > byteCap {
			break
		}
		kept++
		keptBytes += size
		start = i
	}
	if start == 0 {
		return results
	}
	out := results[start:]
	if preserve != nil && !hasPreservedToolResult(out, preserve) {
		for i := start - 1; i >= 0; i-- {
			if preserve(results[i]) {
				preserved := make([]ToolResult, 0, len(out)+1)
				preserved = append(preserved, results[i])
				preserved = append(preserved, out...)
				return preserved
			}
		}
	}
	return append([]ToolResult(nil), out...)
}

func hasPreservedToolResult(results []ToolResult, preserve ToolResultPreserveFunc) bool {
	for _, r := range results {
		if preserve(r) {
			return true
		}
	}
	return false
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
