package operation

import (
	"fmt"
	"strings"
)

// OperationMaterial is a compact, prompt-safe reference to external operation
// material that may need bounded follow-up reads, searches, parsing, provider
// actions, or final-answer mention. It is not current-source evidence and it
// does not grant execution permission.
type OperationMaterial struct {
	Source          string
	Kind            string
	Role            string
	Ref             string
	Summary         string
	Lines           int
	Bytes           int
	CompletePreview bool
}

const (
	MaterialSourceCommand  = "command"
	MaterialSourceProvider = "provider"
	MaterialSourceMemory   = "memory"

	MaterialKindPayloadRef    = "payload_ref"
	MaterialKindArtifactRef   = "artifact_ref"
	MaterialKindNextAction    = "next_action"
	MaterialKindReturnAction  = "return_action"
	MaterialKindWorkflowState = "workflow_state"
	MaterialKindPreview       = "preview"

	MaterialRoleSavedPayload      = "saved_payload"
	MaterialRoleGeneratedArtifact = "generated_artifact"
	MaterialRoleFollowupAction    = "followup_action"
	MaterialRoleReturnHandoff     = "return_handoff"
	MaterialRoleWorkflowContext   = "workflow_context"
	MaterialRolePreviewOnly       = "preview_only"
)

func MaterialsFromCommandStep(step CommandStepResult) []OperationMaterial {
	summary := SummarizeStepOutput(step)
	var out []OperationMaterial
	if strings.TrimSpace(step.PayloadRef) != "" {
		out = append(out, OperationMaterial{
			Source:          MaterialSourceCommand,
			Kind:            MaterialKindPayloadRef,
			Role:            MaterialRoleSavedPayload,
			Ref:             strings.TrimSpace(step.PayloadRef),
			Summary:         summary.Summary,
			Lines:           summary.Lines,
			Bytes:           summary.Bytes,
			CompletePreview: false,
		})
	}
	if len(out) == 0 && commandPreviewLooksPartial(step, summary) {
		out = append(out, OperationMaterial{
			Source:          MaterialSourceCommand,
			Kind:            MaterialKindPreview,
			Role:            MaterialRolePreviewOnly,
			Summary:         summary.Summary,
			Lines:           summary.Lines,
			Bytes:           summary.Bytes,
			CompletePreview: false,
		})
	}
	return out
}

func MaterialsFromProviderResult(provider, tool, summary, payloadRef string, artifactRefs []string, observations int, nextActions []WorkflowNextAction, returnAction WorkflowNextAction, workflowState WorkflowState) []OperationMaterial {
	var out []OperationMaterial
	providerLabel := strings.Trim(strings.TrimSpace(provider)+"::"+strings.TrimSpace(tool), ":")
	add := func(kind, role, ref, text string, complete bool) {
		ref = strings.TrimSpace(ref)
		text = strings.TrimSpace(text)
		if ref == "" && text == "" {
			return
		}
		if text == "" {
			text = providerLabel
		}
		out = append(out, OperationMaterial{
			Source:          MaterialSourceProvider,
			Kind:            kind,
			Role:            role,
			Ref:             ref,
			Summary:         text,
			CompletePreview: complete,
		})
	}
	if payloadRef != "" {
		add(MaterialKindPayloadRef, MaterialRoleSavedPayload, payloadRef, firstMaterialNonEmpty(summary, providerLabel), false)
	}
	for _, ref := range artifactRefs {
		add(MaterialKindArtifactRef, MaterialRoleGeneratedArtifact, ref, firstMaterialNonEmpty(summary, providerLabel), false)
	}
	for i, action := range nextActions {
		add(MaterialKindNextAction, MaterialRoleFollowupAction, action.Compact(), fmt.Sprintf("provider suggested next action %d", i+1), true)
	}
	if text := strings.TrimSpace(returnAction.Compact()); text != "" {
		add(MaterialKindReturnAction, MaterialRoleReturnHandoff, text, "provider return action", true)
	}
	if text := strings.TrimSpace(workflowState.Compact()); text != "" {
		add(MaterialKindWorkflowState, MaterialRoleWorkflowContext, text, "provider workflow state", true)
	}
	if len(out) == 0 && (observations > 0 || strings.TrimSpace(summary) != "") {
		add(MaterialKindPreview, MaterialRolePreviewOnly, "", firstMaterialNonEmpty(summary, providerLabel), true)
	}
	return out
}

func MaterialsFromMemoryEntry(entry MemoryEntry) []OperationMaterial {
	var out []OperationMaterial
	source := MaterialSourceMemory
	if entry.Provider != "" {
		source = MaterialSourceProvider
	}
	for _, ref := range entry.PayloadRefs {
		if strings.TrimSpace(ref) == "" {
			continue
		}
		out = append(out, OperationMaterial{Source: source, Kind: MaterialKindPayloadRef, Role: MaterialRoleSavedPayload, Ref: ref, Summary: entry.Summary, Lines: entry.OutputLines, Bytes: entry.OutputBytes, CompletePreview: false})
	}
	for _, ref := range entry.ArtifactRefs {
		if strings.TrimSpace(ref) == "" {
			continue
		}
		out = append(out, OperationMaterial{Source: source, Kind: MaterialKindArtifactRef, Role: MaterialRoleGeneratedArtifact, Ref: ref, Summary: entry.Summary, CompletePreview: false})
	}
	for _, action := range entry.NextActions {
		if strings.TrimSpace(action) == "" {
			continue
		}
		out = append(out, OperationMaterial{Source: source, Kind: MaterialKindNextAction, Role: MaterialRoleFollowupAction, Ref: action, Summary: "historical provider next action", CompletePreview: true})
	}
	if strings.TrimSpace(entry.ReturnAction) != "" {
		out = append(out, OperationMaterial{Source: source, Kind: MaterialKindReturnAction, Role: MaterialRoleReturnHandoff, Ref: entry.ReturnAction, Summary: "historical provider return action", CompletePreview: true})
	}
	if strings.TrimSpace(entry.WorkflowState) != "" {
		out = append(out, OperationMaterial{Source: source, Kind: MaterialKindWorkflowState, Role: MaterialRoleWorkflowContext, Ref: entry.WorkflowState, Summary: "historical provider workflow state", CompletePreview: true})
	}
	return out
}

func RenderMaterialsForPrompt(materials []OperationMaterial, limit int) string {
	if len(materials) == 0 {
		return ""
	}
	if limit <= 0 {
		limit = len(materials)
	}
	var b strings.Builder
	b.WriteString("operation_materials: these are external operation materials, not current-source evidence; use refs with bounded read/search/extract/provider follow-up when the user goal depends on omitted content.\n")
	written := 0
	for _, m := range materials {
		m = cleanOperationMaterial(m)
		if m.Kind == "" && m.Ref == "" && m.Summary == "" {
			continue
		}
		if written >= limit {
			fmt.Fprintf(&b, "  ...(+%d material ref(s))\n", len(materials)-written)
			break
		}
		fmt.Fprintf(&b, "  material[%d] source=%s kind=%s role=%s", written+1, dashMaterial(m.Source), dashMaterial(m.Kind), dashMaterial(m.Role))
		if m.Ref != "" {
			fmt.Fprintf(&b, " ref=%q", oneLine(m.Ref, 260))
		}
		if m.Summary != "" {
			fmt.Fprintf(&b, " summary=%q", oneLine(m.Summary, 260))
		}
		if m.Lines > 0 {
			fmt.Fprintf(&b, " lines=%d", m.Lines)
		}
		if m.Bytes > 0 {
			fmt.Fprintf(&b, " bytes=%d", m.Bytes)
		}
		fmt.Fprintf(&b, " complete_preview=%t\n", m.CompletePreview)
		written++
	}
	return strings.TrimRight(b.String(), "\n")
}

func RenderMaterialsInline(materials []OperationMaterial, limit int) string {
	if len(materials) == 0 {
		return ""
	}
	if limit <= 0 {
		limit = len(materials)
	}
	var out []string
	for _, m := range materials {
		m = cleanOperationMaterial(m)
		if m.Kind == "" && m.Ref == "" && m.Summary == "" {
			continue
		}
		if len(out) >= limit {
			out = append(out, fmt.Sprintf("...(+%d)", len(materials)-len(out)))
			break
		}
		parts := []string{}
		if m.Source != "" {
			parts = append(parts, "source="+m.Source)
		}
		if m.Kind != "" {
			parts = append(parts, "kind="+m.Kind)
		}
		if m.Role != "" {
			parts = append(parts, "role="+m.Role)
		}
		if m.Ref != "" {
			parts = append(parts, "ref="+oneLine(m.Ref, 180))
		}
		if m.Summary != "" {
			parts = append(parts, "summary="+oneLine(m.Summary, 180))
		}
		parts = append(parts, fmt.Sprintf("complete_preview=%t", m.CompletePreview))
		out = append(out, strings.Join(parts, " "))
	}
	return strings.Join(out, " | ")
}

func cleanOperationMaterial(m OperationMaterial) OperationMaterial {
	m.Source = strings.TrimSpace(m.Source)
	m.Kind = strings.TrimSpace(m.Kind)
	m.Role = strings.TrimSpace(m.Role)
	m.Ref = strings.TrimSpace(m.Ref)
	m.Summary = strings.TrimSpace(m.Summary)
	return m
}

func commandPreviewLooksPartial(step CommandStepResult, summary OutputSummary) bool {
	lower := strings.ToLower(step.OutputPreview)
	return summary.Kind == "large_output_summary" ||
		strings.Contains(lower, "truncated") ||
		strings.Contains(lower, "payload_ref") ||
		strings.Contains(lower, "full output saved")
}

func firstMaterialNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func dashMaterial(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "-"
	}
	return s
}
