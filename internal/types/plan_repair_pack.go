package types

import (
	"encoding/json"
	"strconv"
	"strings"
)

const (
	PlanRepairPackVersion = 1
	PlanRepairToolCode    = "write_plan_repair_pack"
	PlanRepairMetadataKey = "plan_repair_pack"
)

// PlanRepairPack is the compact, typed repair payload for write-plan emit
// failures. It is advisory input for planner retries; hard gates continue to
// read the original schema validators, path policy, and verification reports.
type PlanRepairPack struct {
	Version             int                      `json:"version"`
	ToolName            string                   `json:"tool_name,omitempty"`
	ReasonCode          string                   `json:"reason_code"`
	Message             string                   `json:"message,omitempty"`
	FailingFieldPaths   []string                 `json:"failing_field_paths,omitempty"`
	FailingPaths        []string                 `json:"failing_paths,omitempty"`
	AcceptedEnums       map[string][]string      `json:"accepted_enums,omitempty"`
	CurrentBytes        []PlanRepairCurrentBytes `json:"current_bytes,omitempty"`
	RetryInstruction    string                   `json:"retry_instruction,omitempty"`
	PartialPlanRetained bool                     `json:"partial_plan_retained,omitempty"`
	Metadata            map[string]string        `json:"metadata,omitempty"`
}

type PlanRepairCurrentBytes struct {
	Path                 string                          `json:"path,omitempty"`
	EditIndex            int                             `json:"edit_index,omitempty"`
	FileLineCount        int                             `json:"file_line_count,omitempty"`
	StartLine            int                             `json:"start_line,omitempty"`
	EndLine              int                             `json:"end_line,omitempty"`
	AnchorLine           int                             `json:"anchor_line,omitempty"`
	CurrentBytes         string                          `json:"current_bytes,omitempty"`
	SuppliedOldText      string                          `json:"supplied_old_text,omitempty"`
	ExpectedOldText      string                          `json:"expected_old_text,omitempty"`
	CurrentByteLen       int                             `json:"current_byte_len,omitempty"`
	SuppliedByteLen      int                             `json:"supplied_byte_len,omitempty"`
	SafeEditKinds        []string                        `json:"safe_edit_kinds,omitempty"`
	RelocationCandidates []PlanRepairRelocationCandidate `json:"relocation_candidates,omitempty"`
}

type PlanRepairRelocationCandidate struct {
	Path      string `json:"path,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	Source    string `json:"source,omitempty"`
}

func NormalizePlanRepairPack(in PlanRepairPack) PlanRepairPack {
	if in.Version <= 0 {
		in.Version = PlanRepairPackVersion
	}
	in.ToolName = strings.TrimSpace(in.ToolName)
	in.ReasonCode = strings.TrimSpace(in.ReasonCode)
	in.Message = trimPlanRepairText(in.Message, 640)
	in.RetryInstruction = trimPlanRepairText(in.RetryInstruction, 480)
	in.FailingFieldPaths = normalizePlanRepairStrings(in.FailingFieldPaths, 16, 120)
	in.FailingPaths = normalizePlanRepairStrings(in.FailingPaths, 16, 160)
	for k, vals := range in.AcceptedEnums {
		key := strings.TrimSpace(k)
		if key == "" {
			delete(in.AcceptedEnums, k)
			continue
		}
		normalized := normalizePlanRepairStrings(vals, 32, 80)
		if len(normalized) == 0 {
			delete(in.AcceptedEnums, k)
			continue
		}
		if key != k {
			delete(in.AcceptedEnums, k)
		}
		if in.AcceptedEnums == nil {
			in.AcceptedEnums = map[string][]string{}
		}
		in.AcceptedEnums[key] = normalized
	}
	if len(in.AcceptedEnums) == 0 {
		in.AcceptedEnums = nil
	}
	for i := range in.CurrentBytes {
		in.CurrentBytes[i].Path = strings.TrimSpace(in.CurrentBytes[i].Path)
		in.CurrentBytes[i].CurrentBytes = truncatePlanRepairBytes(in.CurrentBytes[i].CurrentBytes, 1200)
		in.CurrentBytes[i].SuppliedOldText = truncatePlanRepairBytes(in.CurrentBytes[i].SuppliedOldText, 1200)
		in.CurrentBytes[i].ExpectedOldText = truncatePlanRepairBytes(in.CurrentBytes[i].ExpectedOldText, 1200)
		in.CurrentBytes[i].SafeEditKinds = normalizePlanRepairStrings(in.CurrentBytes[i].SafeEditKinds, 16, 80)
		candidates := in.CurrentBytes[i].RelocationCandidates[:0]
		seenRelocation := map[string]bool{}
		for _, c := range in.CurrentBytes[i].RelocationCandidates {
			c.Path = strings.TrimSpace(c.Path)
			c.Source = trimPlanRepairText(c.Source, 80)
			if c.Path == "" || c.StartLine <= 0 || c.EndLine < c.StartLine {
				continue
			}
			key := c.Path + "#" + strconv.Itoa(c.StartLine) + ":" + strconv.Itoa(c.EndLine)
			if seenRelocation[key] {
				continue
			}
			seenRelocation[key] = true
			candidates = append(candidates, c)
			if len(candidates) >= 4 {
				break
			}
		}
		in.CurrentBytes[i].RelocationCandidates = candidates
	}
	outBytes := in.CurrentBytes[:0]
	for _, b := range in.CurrentBytes {
		if b.Path == "" && b.CurrentBytes == "" && b.ExpectedOldText == "" {
			continue
		}
		outBytes = append(outBytes, b)
	}
	in.CurrentBytes = outBytes
	if len(in.CurrentBytes) == 0 {
		in.CurrentBytes = nil
	}
	for k, v := range in.Metadata {
		key := strings.TrimSpace(k)
		val := trimPlanRepairText(v, 240)
		if key == "" || val == "" {
			delete(in.Metadata, k)
			continue
		}
		if key != k {
			delete(in.Metadata, k)
			in.Metadata[key] = val
		} else {
			in.Metadata[k] = val
		}
	}
	if len(in.Metadata) == 0 {
		in.Metadata = nil
	}
	return in
}

func PlanRepairPackJSON(in PlanRepairPack) string {
	normalized := NormalizePlanRepairPack(in)
	data, err := json.Marshal(normalized)
	if err != nil {
		return ""
	}
	return string(data)
}

func PlanRepairPackFromJSON(raw string) (PlanRepairPack, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return PlanRepairPack{}, false
	}
	var pack PlanRepairPack
	if err := json.Unmarshal([]byte(raw), &pack); err != nil {
		return PlanRepairPack{}, false
	}
	pack = NormalizePlanRepairPack(pack)
	if pack.ReasonCode == "" {
		return PlanRepairPack{}, false
	}
	return pack, true
}

func PlanRepairPackFromToolRepair(repair *ToolRepair) (PlanRepairPack, bool) {
	if repair == nil {
		return PlanRepairPack{}, false
	}
	code := strings.TrimSpace(repair.Code)
	if code != "" && code != PlanRepairToolCode {
		return PlanRepairPack{}, false
	}
	if repair.Metadata == nil {
		return PlanRepairPack{}, false
	}
	return PlanRepairPackFromJSON(repair.Metadata[PlanRepairMetadataKey])
}

func PlanRepairPackFromToolResult(result ToolResult) (PlanRepairPack, bool) {
	return PlanRepairPackFromToolRepair(result.Repair)
}

func trimPlanRepairText(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max]) + "..."
}

func truncatePlanRepairBytes(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func normalizePlanRepairStrings(in []string, maxItems, maxLen int) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = trimPlanRepairText(s, maxLen)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
		if maxItems > 0 && len(out) >= maxItems {
			break
		}
	}
	return out
}
