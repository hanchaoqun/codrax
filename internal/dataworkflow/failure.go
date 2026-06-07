package dataworkflow

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func ViolationNodeKey(v dataquery.DataTaskViolation) string {
	id := strings.TrimSpace(v.ActionID)
	if id == "" {
		return ""
	}
	kind := strings.TrimSpace(v.ActionKind)
	return id + "|" + kind
}

func RepeatedNodeFailureFromErrors(previousErrors []string, currentErr string, limit int) (key string, count int, repeated bool) {
	if limit <= 0 {
		limit = 1
	}
	current := dataquery.ClassifyExecutionError(currentErr)
	key = ViolationNodeKey(current)
	if key == "" {
		return "", 0, false
	}
	count = 1
	for _, errText := range previousErrors {
		if strings.TrimSpace(errText) == "" {
			continue
		}
		if ViolationNodeKey(dataquery.ClassifyExecutionError(errText)) == key {
			count++
		}
	}
	return key, count, count >= limit
}

func RepeatedCustomTransformGuardResult(action dataquery.DataAction, previousErrors []string, limit int) GuardResult {
	if NormalizeActionKind(action.Kind) != dataquery.DataActionCustomTransform {
		return GuardResult{}
	}
	actionID := strings.TrimSpace(action.ID)
	if actionID == "" {
		return GuardResult{}
	}
	if limit <= 0 {
		limit = 1
	}
	key := actionID + "|" + string(dataquery.DataActionCustomTransform)
	count := 0
	for _, errText := range previousErrors {
		if strings.TrimSpace(errText) == "" {
			continue
		}
		if ViolationNodeKey(dataquery.ClassifyExecutionError(errText)) == key {
			count++
		}
	}
	if count < limit {
		return GuardResult{}
	}
	msg := fmt.Sprintf("data planning incomplete: custom_transform node %q already failed %d time(s). Do not retry the same free-form script node; replace it with smaller typed atomic actions such as extract_records, derive_rules, derive_fields, normalize_entities, enrich_records, join_records, compute_contributions, and reconcile_artifacts, or emit a new narrow custom_transform over one known artifact.",
		actionID, count)
	return NewGuardResult(
		"repeated_custom_transform_node",
		"error",
		RepairNeedsTypedAction,
		msg,
		WorkflowViolation{
			Code:              "repeated_custom_transform_node",
			Severity:          "error",
			Repairability:     RepairNeedsTypedAction,
			ActionID:          actionID,
			ActionKind:        string(dataquery.DataActionCustomTransform),
			IdempotencyKey:    actionID + "|" + string(dataquery.DataActionCustomTransform),
			RepairActionHints: typedRepairActionHints(),
			Reason:            msg,
		},
	)
}

func CustomTransformFailureClassStats(errorTexts []string) (total int, topCode string, topCodeCount int) {
	counts := map[string]int{}
	for _, errText := range errorTexts {
		if strings.TrimSpace(errText) == "" {
			continue
		}
		total++
		v := dataquery.ClassifyExecutionError(errText)
		code := strings.TrimSpace(v.Code)
		if code == "" {
			code = "unknown_custom_transform_failure"
		}
		counts[code]++
		if counts[code] > topCodeCount {
			topCode = code
			topCodeCount = counts[code]
		}
	}
	return total, topCode, topCodeCount
}

func RepeatedCustomTransformClassGuardResult(action dataquery.DataAction, broadOrWholeWorkflow bool, previousErrors []string, totalLimit, topCodeLimit int) GuardResult {
	if NormalizeActionKind(action.Kind) != dataquery.DataActionCustomTransform || strings.TrimSpace(action.Script) == "" {
		return GuardResult{}
	}
	if !broadOrWholeWorkflow {
		return GuardResult{}
	}
	if totalLimit <= 0 {
		totalLimit = 1
	}
	if topCodeLimit <= 0 {
		topCodeLimit = 1
	}
	total, topCode, topCodeCount := CustomTransformFailureClassStats(previousErrors)
	if total < totalLimit && topCodeCount < topCodeLimit {
		return GuardResult{}
	}
	codeHint := ""
	if topCode != "" {
		codeHint = fmt.Sprintf(" most_common_failure=%s count=%d.", topCode, topCodeCount)
	}
	msg := fmt.Sprintf("data planning incomplete: workflow already has %d custom_transform failure(s).%s Do not bypass this by changing action_id; avoid another broad free-form script. Continue with typed atomic actions that produce reusable artifacts and consume fields from artifact_access.",
		total, codeHint)
	return NewGuardResult(
		"repeated_custom_transform_class",
		"error",
		RepairNeedsTypedAction,
		msg,
		WorkflowViolation{
			Code:              "repeated_custom_transform_class",
			Severity:          "error",
			Repairability:     RepairNeedsTypedAction,
			ActionID:          strings.TrimSpace(action.ID),
			ActionKind:        string(dataquery.DataActionCustomTransform),
			RepairActionHints: typedRepairActionHints(),
			Reason:            msg,
		},
	)
}

func typedRepairActionHints() []string {
	return []string{
		string(dataquery.DataActionExtractRecords),
		string(dataquery.DataActionDeriveRules),
		string(dataquery.DataActionDeriveFields),
		string(dataquery.DataActionNormalizeEntities),
		string(dataquery.DataActionEnrichRecords),
		string(dataquery.DataActionJoinRecords),
		string(dataquery.DataActionComputeContribs),
		string(dataquery.DataActionReconcile),
	}
}
