package tool

import (
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	planRepairToolCode    = types.PlanRepairToolCode
	planRepairMetadataKey = types.PlanRepairMetadataKey
	planRepairSummaryTag  = "PLAN_REPAIR_PACK:"
)

func rejectPlanToolResult(toolName, summary string, pack *types.PlanRepairPack) types.ToolResult {
	res := types.ToolResult{
		ToolName:  toolName,
		Success:   false,
		Summary:   strings.TrimSpace(summary),
		Timestamp: time.Now(),
	}
	return attachPlanRepairPack(res, pack)
}

func attachPlanRepairPack(res types.ToolResult, pack *types.PlanRepairPack) types.ToolResult {
	if pack == nil {
		return res
	}
	normalized := types.NormalizePlanRepairPack(*pack)
	if strings.TrimSpace(normalized.ReasonCode) == "" {
		return res
	}
	if strings.TrimSpace(normalized.ToolName) == "" {
		normalized.ToolName = res.ToolName
	}
	encoded := types.PlanRepairPackJSON(normalized)
	if encoded == "" {
		return res
	}
	if res.Repair == nil {
		res.Repair = &types.ToolRepair{}
	}
	res.Repair.Code = planRepairToolCode
	if res.Repair.Hint == "" {
		res.Repair.Hint = "Use the plan_repair_pack metadata as the bounded typed repair input for the next plan emit. Do not infer hard routing from prose."
	}
	res.Repair.Fields = appendUniqueStrings(res.Repair.Fields, normalized.FailingFieldPaths...)
	for _, p := range normalized.FailingPaths {
		res.Repair.Targets = append(res.Repair.Targets, types.ToolRepairTarget{File: p, Action: "repair_plan_emit"})
	}
	if res.Repair.Metadata == nil {
		res.Repair.Metadata = map[string]string{}
	}
	res.Repair.Metadata[planRepairMetadataKey] = encoded
	res.Repair.Metadata["reason_code"] = normalized.ReasonCode
	if res.Summary != "" && !strings.Contains(res.Summary, planRepairSummaryTag) {
		res.Summary += "\n" + planRepairSummaryTag + encoded
	}
	return res
}

func planRepairPackFromToolRepair(toolName, reasonCode, message string, repair *types.ToolRepair) *types.PlanRepairPack {
	pack := &types.PlanRepairPack{
		ToolName:          toolName,
		ReasonCode:        reasonCode,
		Message:           message,
		FailingFieldPaths: append([]string(nil), repairFields(repair)...),
		RetryInstruction:  repairHint(repair),
		Metadata:          map[string]string{},
	}
	if repair != nil {
		if repair.Code != "" {
			pack.Metadata["tool_repair_code"] = repair.Code
		}
		for k, v := range repair.Metadata {
			pack.Metadata[k] = v
		}
	}
	normalized := types.NormalizePlanRepairPack(*pack)
	return &normalized
}

func planRepairPackFromStructuredEditDiagnostic(toolName, message string, diagnostic structuredEditDiagnostic) *types.PlanRepairPack {
	reason := strings.TrimSpace(diagnostic.ReasonCode)
	if reason == "" {
		reason = "structured_edit_rejected"
	}
	pack := &types.PlanRepairPack{
		ToolName:          toolName,
		ReasonCode:        reason,
		Message:           message,
		FailingFieldPaths: []string{"$.changes[].edits"},
		FailingPaths:      []string{diagnostic.Path},
		RetryInstruction:  diagnostic.RetryInstruction,
		CurrentBytes: []types.PlanRepairCurrentBytes{{
			Path:            diagnostic.Path,
			EditIndex:       diagnostic.EditIndex,
			FileLineCount:   diagnostic.FileLineCount,
			StartLine:       diagnostic.StartLine,
			EndLine:         diagnostic.EndLine,
			AnchorLine:      diagnostic.AnchorLine,
			CurrentBytes:    diagnostic.CurrentBytes,
			SuppliedOldText: diagnostic.SuppliedOldText,
			ExpectedOldText: diagnostic.ExpectedOldText,
			CurrentByteLen:  diagnostic.CurrentByteLen,
			SuppliedByteLen: diagnostic.SuppliedByteLen,
			SafeEditKinds:   append([]string(nil), diagnostic.SafeEditKinds...),
			RelocationCandidates: append([]types.PlanRepairRelocationCandidate(nil),
				diagnostic.RelocationCandidates...),
		}},
	}
	normalized := types.NormalizePlanRepairPack(*pack)
	return &normalized
}

func planRepairPackFromReason(toolName, reasonCode, message string, fields []string, paths []string) *types.PlanRepairPack {
	pack := &types.PlanRepairPack{
		ToolName:          toolName,
		ReasonCode:        reasonCode,
		Message:           message,
		FailingFieldPaths: append([]string(nil), fields...),
		FailingPaths:      append([]string(nil), paths...),
	}
	normalized := types.NormalizePlanRepairPack(*pack)
	return &normalized
}

func planRepairPackWithEnums(toolName, reasonCode, message string, fields []string, enums map[string][]string) *types.PlanRepairPack {
	pack := planRepairPackFromReason(toolName, reasonCode, message, fields, nil)
	if pack == nil {
		return nil
	}
	pack.AcceptedEnums = enums
	normalized := types.NormalizePlanRepairPack(*pack)
	return &normalized
}

func repairFields(repair *types.ToolRepair) []string {
	if repair == nil {
		return nil
	}
	return append([]string(nil), repair.Fields...)
}

func repairHint(repair *types.ToolRepair) string {
	if repair == nil {
		return ""
	}
	return repair.Hint
}

func appendUniqueStrings(base []string, add ...string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(base)+len(add))
	for _, s := range append(append([]string(nil), base...), add...) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func planRepairPackFromMetadataJSON(raw string) (types.PlanRepairPack, bool) {
	return types.PlanRepairPackFromJSON(raw)
}
