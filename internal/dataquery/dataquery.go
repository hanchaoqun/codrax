package dataquery

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type OutputFormat string

const (
	OutputFreeform        OutputFormat = "freeform"
	OutputPlainSingleLine OutputFormat = "plain_single_line"
	OutputCSVLine         OutputFormat = "csv_line"
	OutputJSONOnly        OutputFormat = "json_only"
	OutputMarkdownTable   OutputFormat = "markdown_table"
	OutputMarkdown        OutputFormat = "markdown"
	OutputFilePath        OutputFormat = "file_path"
)

type OutputContract struct {
	Format             OutputFormat `json:"format"`
	ExplanationAllowed bool         `json:"explanation_allowed"`
	Delimiter          string       `json:"delimiter,omitempty"`
}

func (c OutputContract) Normalize() OutputContract {
	switch c.Format {
	case OutputPlainSingleLine, OutputCSVLine, OutputJSONOnly, OutputMarkdownTable, OutputMarkdown, OutputFilePath, OutputFreeform:
	default:
		c.Format = OutputFreeform
	}
	if c.Format == OutputCSVLine && c.Delimiter == "" {
		c.Delimiter = ","
	}
	return c
}

type TaskPlan struct {
	Status              string           `json:"status"`
	InputPaths          []string         `json:"input_paths"`
	OutputContract      OutputContract   `json:"output_contract"`
	CoverageContract    CoverageContract `json:"coverage_contract,omitempty"`
	Actions             []DataAction     `json:"actions,omitempty"`
	Script              string           `json:"script"`
	Questions           []Question       `json:"questions,omitempty"`
	BlockReason         string           `json:"block_reason,omitempty"`
	Goal                string           `json:"goal,omitempty"`
	KnownConstraints    []string         `json:"known_constraints,omitempty"`
	MissingObservations []string         `json:"missing_observations,omitempty"`
	SuccessCriteria     []string         `json:"success_criteria,omitempty"`
	NextBatch           string           `json:"next_batch,omitempty"`
	WhyThisBatch        string           `json:"why_this_batch,omitempty"`
	ContinueAfter       bool             `json:"continue_after,omitempty"`
}

type DataActionKind string

const (
	DataActionMaterialInventory DataActionKind = "material_inventory"
	DataActionInspectMaterial   DataActionKind = "inspect_material"
	DataActionExtractRecords    DataActionKind = "extract_records"
	DataActionDeriveRules       DataActionKind = "derive_rules"
	DataActionNormalizeEntities DataActionKind = "normalize_entities"
	DataActionComputeContribs   DataActionKind = "compute_contributions"
	DataActionReconcile         DataActionKind = "reconcile_artifacts"
	DataActionCustomTransform   DataActionKind = "custom_transform"
)

type DataAction struct {
	ID              string            `json:"id,omitempty"`
	Kind            DataActionKind    `json:"kind"`
	Purpose         string            `json:"purpose,omitempty"`
	InputPaths      []string          `json:"input_paths,omitempty"`
	OutputArtifact  string            `json:"output_artifact,omitempty"`
	Script          string            `json:"script,omitempty"`
	Params          map[string]string `json:"params,omitempty"`
	SuccessCriteria []string          `json:"success_criteria,omitempty"`
}

type DataArtifact struct {
	ID          string            `json:"id,omitempty"`
	Kind        string            `json:"kind,omitempty"`
	SourcePaths []string          `json:"source_paths,omitempty"`
	Summary     string            `json:"summary,omitempty"`
	Fields      map[string]string `json:"fields,omitempty"`
	Headers     []string          `json:"headers,omitempty"`
	Sample      []string          `json:"sample,omitempty"`
	RowCount    int               `json:"row_count,omitempty"`
	Children    []DataArtifact    `json:"children,omitempty"`
}

type CoverageContract struct {
	RequiredMaterials          []CoverageMaterial `json:"required_materials,omitempty"`
	OptionalMaterials          []CoverageMaterial `json:"optional_materials,omitempty"`
	ValidationRules            []string           `json:"validation_rules,omitempty"`
	DecisionRecordsRequired    bool               `json:"decision_records_required,omitempty"`
	RuleCoverageRequired       bool               `json:"rule_coverage_required,omitempty"`
	ContributionLedgerRequired bool               `json:"contribution_ledger_required,omitempty"`
	EntityResolutionRequired   bool               `json:"entity_resolution_required,omitempty"`
	ReconcileRequired          bool               `json:"reconcile_required,omitempty"`
}

type CoverageMaterialUseMode string

const (
	MaterialUseScriptConsumed       CoverageMaterialUseMode = "script_consumed"
	MaterialUseTextEvidenceConsumed CoverageMaterialUseMode = "text_evidence_consumed"
	MaterialUsePlannerDistilled     CoverageMaterialUseMode = "planner_distilled"
	MaterialUseReferenceOnly        CoverageMaterialUseMode = "reference_only"
)

type CoverageMaterial struct {
	ID               string                  `json:"id,omitempty"`
	Path             string                  `json:"path,omitempty"`
	Purpose          string                  `json:"purpose,omitempty"`
	Required         bool                    `json:"required,omitempty"`
	UsageMode        CoverageMaterialUseMode `json:"usage_mode,omitempty"`
	TextEvidencePath string                  `json:"text_evidence_path,omitempty"`
	DistilledNotes   []string                `json:"distilled_notes,omitempty"`
}

func (c CoverageContract) RequiredPaths() []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range c.RequiredMaterials {
		path := normalizeMaterialPath(m.Path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func (c CoverageContract) RequiredRunnerInputPaths() []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range c.RequiredMaterials {
		mode := normalizeCoverageMaterialUseMode(m.UsageMode)
		var path string
		switch mode {
		case MaterialUseScriptConsumed:
			path = normalizeMaterialPath(m.Path)
		case MaterialUseTextEvidenceConsumed:
			path = normalizeMaterialPath(m.TextEvidencePath)
		default:
			continue
		}
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func (c CoverageContract) RequiredScriptPaths() []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range c.RequiredMaterials {
		if normalizeCoverageMaterialUseMode(m.UsageMode) != MaterialUseScriptConsumed {
			continue
		}
		path := normalizeMaterialPath(m.Path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func normalizeCoverageMaterialUseMode(mode CoverageMaterialUseMode) CoverageMaterialUseMode {
	switch CoverageMaterialUseMode(strings.ToLower(strings.TrimSpace(string(mode)))) {
	case "", MaterialUseScriptConsumed:
		return MaterialUseScriptConsumed
	case MaterialUseTextEvidenceConsumed:
		return MaterialUseTextEvidenceConsumed
	case MaterialUsePlannerDistilled:
		return MaterialUsePlannerDistilled
	case MaterialUseReferenceOnly:
		return MaterialUseReferenceOnly
	default:
		return MaterialUseScriptConsumed
	}
}

type NonTextRequiredMaterial struct {
	Path              string   `json:"path"`
	Kind              string   `json:"kind"`
	ExtractionStatus  string   `json:"extraction_status,omitempty"`
	TextEvidencePaths []string `json:"text_evidence_paths,omitempty"`
}

type MaterialExtraction struct {
	SourcePath string `json:"source_path"`
	TextPath   string `json:"text_path"`
	Kind       string `json:"kind,omitempty"`
	Confidence string `json:"confidence,omitempty"`
	Notes      string `json:"notes,omitempty"`
}

type DataRepairability string

const (
	RepairabilitySafePatch          DataRepairability = "safe_patch"
	RepairabilityNeedsRecompute     DataRepairability = "needs_recompute"
	RepairabilityNeedsClarification DataRepairability = "needs_clarification"
)

type DataTaskViolation struct {
	Code          string            `json:"code"`
	Summary       string            `json:"summary,omitempty"`
	JSONPath      string            `json:"json_path,omitempty"`
	ExpectedShape string            `json:"expected_shape,omitempty"`
	ActualSnippet string            `json:"actual_snippet,omitempty"`
	ActionID      string            `json:"action_id,omitempty"`
	ActionKind    string            `json:"action_kind,omitempty"`
	ScriptLine    int               `json:"script_line,omitempty"`
	RunnerLine    int               `json:"runner_line,omitempty"`
	Repairability DataRepairability `json:"repairability,omitempty"`
	RepairHint    string            `json:"repair_hint,omitempty"`
}

type DataValidationError struct {
	Violations []DataTaskViolation `json:"violations"`
}

func (e DataValidationError) Error() string {
	if len(e.Violations) == 0 {
		return "data validation failed"
	}
	v := e.Violations[0]
	if strings.TrimSpace(v.Summary) != "" {
		return v.Summary
	}
	if strings.TrimSpace(v.Code) != "" {
		return "data validation failed: " + v.Code
	}
	return "data validation failed"
}

// DataResultValidationError preserves the parsed result when validation fails
// after the runner already produced structured JSON. Callers can use the typed
// violations and bounded result snapshot for structural repair without asking
// the model to rewrite the whole script. Business-semantic failures still fall
// back to recomputation.
type DataResultValidationError struct {
	Result     Result              `json:"result"`
	Violations []DataTaskViolation `json:"violations"`
	Err        error               `json:"-"`
}

func (e *DataResultValidationError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	if len(e.Violations) > 0 {
		return DataValidationError{Violations: e.Violations}.Error()
	}
	return "data result validation failed"
}

func (e *DataResultValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func wrapDataResultValidationError(res Result, err error) error {
	if err == nil {
		return nil
	}
	var validationErr DataValidationError
	if !errors.As(err, &validationErr) {
		return err
	}
	return &DataResultValidationError{
		Result:     res,
		Violations: append([]DataTaskViolation(nil), validationErr.Violations...),
		Err:        err,
	}
}

func dataValidationError(code, jsonPath, expectedShape, actualSnippet string, repairability DataRepairability, format string, args ...any) error {
	return DataValidationError{Violations: []DataTaskViolation{{
		Code:          code,
		Summary:       fmt.Sprintf(format, args...),
		JSONPath:      jsonPath,
		ExpectedShape: expectedShape,
		ActualSnippet: clampOneLine(actualSnippet, 300),
		Repairability: repairability,
	}}}
}

func ClassifyExecutionError(errText string) DataTaskViolation {
	text := strings.TrimSpace(errText)
	lower := strings.ToLower(text)
	scriptLine, runnerLine := parseDataScriptTracebackLocus(text)
	missingName, hasMissingName := parsePythonNameErrorMissingName(text)
	v := DataTaskViolation{
		Code:       "runtime_failure",
		Summary:    clampViolationText(text, 500),
		ScriptLine: scriptLine,
		RunnerLine: runnerLine,
		RepairHint: "Inspect the failing line or typed workflow violation, keep the same user goal and output contract, and emit a corrected bounded plan.",
	}
	switch {
	case strings.Contains(lower, "data action failed"):
		v.Code = "data_action_failed"
		v.ActionID = parseQuotedErrorField(text, "action_id")
		v.ActionKind = parseQuotedErrorField(text, "action_kind")
		v.RepairHint = "Repair the failed typed data action/node. Keep the action atomic; if the failure shows missing schema/material knowledge, split or insert inspect/extract actions before retrying the transform."
		if strings.Contains(lower, "path was not declared as an input") {
			v.Code = "undeclared_input_path"
			v.ActualSnippet = parseUndeclaredInputPath(text)
			v.RepairHint = "The failed action read an undeclared path. Add the path only if that action should consume it; otherwise expand the graph with inspect/extract actions and retry a smaller transform."
		}
	case strings.Contains(lower, "data planning incomplete") && strings.Contains(lower, "bounded data batch"):
		v.Code = "oversized_data_plan"
		v.RepairHint = "Split the plan into a smaller bounded batch, set continue_after=true when more work remains, and let the workflow feed real results into later batches."
	case strings.Contains(lower, "data task script redefines reserved helper"):
		v.Code = "reserved_helper_redefined"
		v.RepairHint = "Remove any function/variable assignment that redefines runner helpers; call the provided helper directly."
	case strings.Contains(lower, "name 'false' is not defined") ||
		strings.Contains(lower, "name 'true' is not defined") ||
		strings.Contains(lower, "name 'null' is not defined"):
		v.Code = "python_json_literal_name"
		v.RepairHint = "Use Python constants True/False/None, or rely on runner-provided true/false/null constants without redefining them."
	case hasMissingName && isLikelyDataTaskHelperName(missingName):
		v.Code = "unknown_runner_helper"
		v.RepairHint = "Use canonical runner helpers: add_decision, add_rule_coverage, add_contribution, add_resolution, emit_result, emit, csv_rows, tsv_rows, json_load, jsonl_rows, read_text, parse_money."
	case strings.Contains(lower, "path was not declared as an input"):
		v.Code = "undeclared_input_path"
		v.ActualSnippet = parseUndeclaredInputPath(text)
		v.RepairHint = "Every file read by a script/custom_transform must be declared in that node's input_paths. Add the path only if this atomic action should consume it; otherwise split a prior inspect/extract action or remove the read."
	case strings.Contains(lower, "invalid literal for int()"):
		v.Code = "numeric_parse_failure"
		v.RepairHint = "Do not parse identifiers as integers; keep identifiers as strings and parse only fields that are truly numeric."
	case strings.Contains(lower, "uses text_evidence_consumed but text_evidence_path is empty"):
		v.Code = "text_evidence_path_missing"
		v.RepairHint = "Set text_evidence_path for the required material or choose a different verifiable usage_mode."
	case strings.Contains(lower, "planner_distilled") && strings.Contains(lower, "distilled_notes is empty"):
		v.Code = "planner_distilled_notes_missing"
		v.RepairHint = "If the material is planner_distilled, include concrete distilled_notes; otherwise switch to script_consumed or text_evidence_consumed."
	case strings.Contains(lower, "cannot use reference_only"):
		v.Code = "required_material_reference_only"
		v.RepairHint = "Move advisory materials to optional_materials, or choose a verifiable required usage_mode."
	case strings.Contains(lower, "text evidence") && strings.Contains(lower, "was not consumed by the script"):
		v.Code = "text_evidence_not_consumed"
		v.RepairHint = "Read the declared text_evidence_path with a helper and use it in computation, validation, audit, or ledgers."
	case strings.Contains(lower, "required material") && strings.Contains(lower, "is not declared in input_paths"):
		v.Code = "required_material_not_declared"
		v.RepairHint = "Declare the runner-readable required path in input_paths, or use text_evidence_consumed/planner_distilled when direct script input is not correct."
	case strings.Contains(lower, "required material") && strings.Contains(lower, "was not consumed by the script"):
		v.Code = "required_material_not_consumed"
		v.RepairHint = "Read the material with a helper and use the content, or use a verifiable non-direct usage_mode with required evidence fields."
	case strings.Contains(lower, "coverage_contract.") && strings.Contains(lower, "but result.") && strings.Contains(lower, "is empty"):
		v.Code = "missing_required_ledger"
		v.RepairHint = "Emit the required generic ledger instead of weakening the contract."
	case strings.Contains(lower, "unsupported operation"):
		v.Code = "unsupported_contribution_operation"
		v.RepairHint = "Use canonical contribution operations such as add/sum/count/subtract/set/rank."
	case strings.Contains(lower, "references unknown rule_id"):
		v.Code = "unknown_rule_ref"
		v.RepairHint = "Every rule_ref must match an emitted rule_coverage.rule_id. Add the missing generic rule coverage record, or remove/replace the invalid rule_ref."
	case strings.Contains(lower, "entity_resolutions") && strings.Contains(lower, "missing status"):
		v.Code = "missing_entity_resolution_status"
		v.RepairHint = "Use add_resolution(...) or set status to resolved, ambiguous, unresolved, or not_applicable for every entity resolution record."
	case strings.Contains(lower, "has no matching contribution records"):
		v.Code = "reconcile_group_mismatch"
		v.RepairHint = "Align contribution group_key/metric with reconcile.groups, or correct the contribution records."
	case strings.Contains(lower, "contributions sum to"):
		v.Code = "reconcile_sum_mismatch"
		v.RepairHint = "Fix the computation or reconcile expected/actual values so they match contribution totals."
	case strings.Contains(lower, "json: cannot unmarshal") || strings.Contains(lower, "parse data task result"):
		v.Code = "result_schema_mismatch"
		v.RepairHint = "Emit valid result JSON using canonical fields or runner ledger helpers."
	case strings.Contains(lower, "output contract"):
		v.Code = "output_contract_violation"
		v.RepairHint = "Keep the computed answer but render it exactly according to output_contract."
	}
	return v
}

func parseQuotedErrorField(text, key string) string {
	marker := key + "="
	idx := strings.Index(text, marker)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(text[idx+len(marker):])
	if strings.HasPrefix(rest, `"`) {
		rest = strings.TrimPrefix(rest, `"`)
		if end := strings.Index(rest, `"`); end >= 0 {
			return strings.TrimSpace(rest[:end])
		}
		return strings.TrimSpace(rest)
	}
	end := strings.IndexAny(rest, " \t\r\n;:")
	if end >= 0 {
		rest = rest[:end]
	}
	return strings.Trim(strings.TrimSpace(rest), `"'`)
}

func parseUndeclaredInputPath(text string) string {
	const marker = "path was not declared as an input:"
	idx := strings.Index(strings.ToLower(text), marker)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(text[idx+len(marker):])
	if end := strings.IndexAny(rest, "\r\n"); end >= 0 {
		rest = rest[:end]
	}
	return strings.Trim(strings.TrimSpace(rest), `"'`)
}

func parsePythonNameErrorMissingName(text string) (string, bool) {
	const prefix = "NameError: name '"
	idx := strings.Index(text, prefix)
	if idx < 0 {
		return "", false
	}
	rest := text[idx+len(prefix):]
	end := strings.Index(rest, "'")
	if end < 0 {
		return "", false
	}
	name := strings.TrimSpace(rest[:end])
	if name == "" {
		return "", false
	}
	return name, true
}

func isLikelyDataTaskHelperName(name string) bool {
	switch {
	case strings.HasPrefix(name, "add_"):
		return true
	case strings.HasPrefix(name, "emit_"):
		return true
	case strings.HasSuffix(name, "_rows"):
		return true
	case name == "read_text" || name == "json_load" || name == "jsonl_rows" || name == "parse_money":
		return true
	default:
		return false
	}
}

func parseDataScriptTracebackLocus(text string) (scriptLine int, runnerLine int) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if scriptLine == 0 {
			if n, ok := parsePythonTracebackLineNumber(line, `File "<string>", line `); ok {
				scriptLine = n
			}
		}
		if runnerLine == 0 && strings.Contains(line, `_runner.py`) {
			if n, ok := parsePythonTracebackCommaLineNumber(line); ok {
				runnerLine = n
			}
		}
	}
	return scriptLine, runnerLine
}

func parsePythonTracebackLineNumber(line, marker string) (int, bool) {
	idx := strings.Index(line, marker)
	if idx < 0 {
		return 0, false
	}
	rest := line[idx+len(marker):]
	end := strings.Index(rest, ",")
	if end >= 0 {
		rest = rest[:end]
	}
	n, err := strconv.Atoi(strings.TrimSpace(rest))
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func parsePythonTracebackCommaLineNumber(line string) (int, bool) {
	const marker = ", line "
	idx := strings.Index(line, marker)
	if idx < 0 {
		return 0, false
	}
	rest := line[idx+len(marker):]
	end := strings.Index(rest, ",")
	if end >= 0 {
		rest = rest[:end]
	}
	n, err := strconv.Atoi(strings.TrimSpace(rest))
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func clampViolationText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "...[truncated]"
}

func snippetJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return clampViolationText(fmt.Sprintf("%v", value), 300)
	}
	return clampViolationText(string(raw), 300)
}

func FindNonTextRequiredMaterials(contract CoverageContract, candidates []CandidateFile) []NonTextRequiredMaterial {
	byPath := map[string]CandidateFile{}
	for _, c := range candidates {
		path := normalizeMaterialPath(c.Path)
		if path == "" {
			continue
		}
		byPath[path] = c
	}
	seen := map[string]bool{}
	var out []NonTextRequiredMaterial
	for _, req := range contract.RequiredMaterials {
		if normalizeCoverageMaterialUseMode(req.UsageMode) != MaterialUseScriptConsumed {
			continue
		}
		path := normalizeMaterialPath(req.Path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		candidate, ok := byPath[path]
		if !ok || !candidateNeedsTextExtraction(candidate) {
			continue
		}
		out = append(out, NonTextRequiredMaterial{
			Path:              candidate.Path,
			Kind:              candidate.Kind,
			ExtractionStatus:  candidate.ExtractionStatus,
			TextEvidencePaths: append([]string(nil), candidate.TextEvidencePaths...),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

type Question struct {
	ID          string   `json:"id,omitempty"`
	Question    string   `json:"question"`
	Suggestions []string `json:"suggestions,omitempty"`
}

type Result struct {
	Answer            string                   `json:"answer"`
	OutputContract    OutputContract           `json:"output_contract"`
	AuditSummary      string                   `json:"audit_summary,omitempty"`
	Artifacts         []DataArtifact           `json:"artifacts,omitempty"`
	Rows              []RowDecision            `json:"rows,omitempty"`
	RuleCoverage      []RuleCoverageRecord     `json:"rule_coverage,omitempty"`
	Contributions     []ContributionRecord     `json:"contributions,omitempty"`
	EntityResolutions []EntityResolutionRecord `json:"entity_resolutions,omitempty"`
	Reconcile         *ReconcileReport         `json:"reconcile,omitempty"`
	Metrics           []Metric                 `json:"metrics,omitempty"`
	ConsumedPaths     []string                 `json:"consumed_paths,omitempty"`
	ContractWarnings  []string                 `json:"contract_warnings,omitempty"`
	ResultPatches     []DataResultPatch        `json:"result_patches,omitempty"`
}

type DataResultPatch struct {
	Target string          `json:"target"`
	Op     string          `json:"op"`
	Path   string          `json:"path"`
	Value  json.RawMessage `json:"value,omitempty"`
	Reason string          `json:"reason,omitempty"`
}

type DataResultPatchPlan struct {
	Status     string            `json:"status,omitempty"`
	Patches    []DataResultPatch `json:"patches"`
	Reason     string            `json:"reason,omitempty"`
	Confidence string            `json:"confidence,omitempty"`
}

type LooseText string

func (v *LooseText) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		*v = ""
		return nil
	}
	*v = LooseText(rawJSONValueString(raw))
	return nil
}

func (v LooseText) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(v))
}

func (v LooseText) String() string {
	return string(v)
}

type RuleCoverageRecord struct {
	RuleID       LooseText `json:"rule_id,omitempty"`
	RuleText     LooseText `json:"rule_text,omitempty"`
	Status       LooseText `json:"status,omitempty"`
	EvidenceRefs []string  `json:"evidence_refs,omitempty"`
	Notes        LooseText `json:"notes,omitempty"`
}

func (r *RuleCoverageRecord) UnmarshalJSON(data []byte) error {
	type ruleCoverageAlias RuleCoverageRecord
	var known ruleCoverageAlias
	if err := json.Unmarshal(data, &known); err != nil {
		return err
	}
	*r = RuleCoverageRecord(known)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	if r.RuleID.String() == "" {
		r.RuleID = LooseText(rawAliasString(raw, "id", "rule", "rule_ref"))
	}
	if r.RuleText.String() == "" {
		r.RuleText = LooseText(rawAliasString(raw, "text", "description", "condition"))
	}
	if r.Status.String() == "" {
		r.Status = LooseText(rawAliasString(raw, "outcome", "decision"))
	}
	if r.Notes.String() == "" {
		r.Notes = LooseText(rawAliasString(raw, "reason", "summary", "details"))
	}
	return nil
}

type ContributionRecord struct {
	ItemID        LooseText `json:"item_id,omitempty"`
	Source        LooseText `json:"source,omitempty"`
	SourceLocator LooseText `json:"source_locator,omitempty"`
	GroupKey      LooseText `json:"group_key,omitempty"`
	Metric        LooseText `json:"metric,omitempty"`
	Value         LooseText `json:"value,omitempty"`
	Operation     LooseText `json:"operation,omitempty"`
	Reason        LooseText `json:"reason,omitempty"`
	EvidenceRefs  []string  `json:"evidence_refs,omitempty"`
	RuleRefs      []string  `json:"rule_refs,omitempty"`
}

func (r *ContributionRecord) UnmarshalJSON(data []byte) error {
	type contributionAlias ContributionRecord
	var known contributionAlias
	if err := json.Unmarshal(data, &known); err != nil {
		return err
	}
	*r = ContributionRecord(known)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	if r.ItemID.String() == "" {
		r.ItemID = LooseText(rawAliasString(raw, "row_id", "record_id", "id", "item"))
	}
	if r.SourceLocator.String() == "" {
		r.SourceLocator = LooseText(rawAliasString(raw, "locator", "location", "span", "cell", "row"))
	}
	if r.GroupKey.String() == "" {
		r.GroupKey = LooseText(rawAliasString(raw, "group", "dimensions", "dimension_key", "bucket"))
	}
	if r.Metric.String() == "" {
		r.Metric = LooseText(rawAliasString(raw, "measure", "field", "metric_name"))
	}
	if r.Operation.String() == "" {
		r.Operation = LooseText(rawAliasString(raw, "op", "aggregation"))
	}
	if r.Reason.String() == "" {
		r.Reason = LooseText(rawAliasString(raw, "notes", "summary", "details"))
	}
	if len(r.RuleRefs) == 0 {
		r.RuleRefs = rawAliasStringSlice(raw, "rule_refs", "rule_ref", "rule_id", "rule")
	}
	return nil
}

type EntityResolutionRecord struct {
	ItemID         LooseText         `json:"item_id,omitempty"`
	SourceValue    LooseText         `json:"source_value,omitempty"`
	CanonicalID    LooseText         `json:"canonical_id,omitempty"`
	CanonicalLabel LooseText         `json:"canonical_label,omitempty"`
	Status         LooseText         `json:"status,omitempty"`
	Candidates     []EntityCandidate `json:"candidates,omitempty"`
	EvidenceRefs   []string          `json:"evidence_refs,omitempty"`
	RuleRefs       []string          `json:"rule_refs,omitempty"`
	Reason         LooseText         `json:"reason,omitempty"`
}

func (r *EntityResolutionRecord) UnmarshalJSON(data []byte) error {
	type rawEntityResolutionRecord struct {
		ItemID         LooseText       `json:"item_id,omitempty"`
		SourceValue    LooseText       `json:"source_value,omitempty"`
		CanonicalID    LooseText       `json:"canonical_id,omitempty"`
		CanonicalLabel LooseText       `json:"canonical_label,omitempty"`
		Status         LooseText       `json:"status,omitempty"`
		Candidates     json.RawMessage `json:"candidates,omitempty"`
		EvidenceRefs   []string        `json:"evidence_refs,omitempty"`
		RuleRefs       []string        `json:"rule_refs,omitempty"`
		Reason         LooseText       `json:"reason,omitempty"`
	}
	var raw rawEntityResolutionRecord
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = EntityResolutionRecord{
		ItemID:         raw.ItemID,
		SourceValue:    raw.SourceValue,
		CanonicalID:    raw.CanonicalID,
		CanonicalLabel: raw.CanonicalLabel,
		Status:         raw.Status,
		EvidenceRefs:   raw.EvidenceRefs,
		RuleRefs:       raw.RuleRefs,
		Reason:         raw.Reason,
	}
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err == nil {
		if r.ItemID.String() == "" {
			r.ItemID = LooseText(rawAliasString(rawMap, "row_id", "record_id", "id", "item"))
		}
		if r.SourceValue.String() == "" {
			r.SourceValue = LooseText(rawAliasString(rawMap, "source", "raw", "raw_value", "value"))
		}
		if r.CanonicalID.String() == "" {
			r.CanonicalID = LooseText(rawAliasString(rawMap, "canonical", "normalized_id", "target_id"))
		}
		if r.CanonicalLabel.String() == "" {
			r.CanonicalLabel = LooseText(rawAliasString(rawMap, "label", "normalized_label", "target_label"))
		}
		if r.Reason.String() == "" {
			r.Reason = LooseText(rawAliasString(rawMap, "notes", "summary", "details"))
		}
		if len(r.RuleRefs) == 0 {
			r.RuleRefs = rawAliasStringSlice(rawMap, "rule_refs", "rule_ref", "rule_id", "rule")
		}
	}
	candidates, err := parseEntityCandidates(raw.Candidates)
	if err != nil {
		return err
	}
	r.Candidates = candidates
	return nil
}

func parseEntityCandidates(raw json.RawMessage) ([]EntityCandidate, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	if !bytes.HasPrefix(raw, []byte("[")) {
		var one EntityCandidate
		if err := unmarshalEntityCandidate(raw, &one); err != nil {
			return nil, err
		}
		return []EntityCandidate{one}, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	out := make([]EntityCandidate, 0, len(items))
	for _, item := range items {
		item = bytes.TrimSpace(item)
		if len(item) == 0 || bytes.Equal(item, []byte("null")) {
			continue
		}
		var candidate EntityCandidate
		if err := unmarshalEntityCandidate(item, &candidate); err != nil {
			return nil, err
		}
		out = append(out, candidate)
	}
	return out, nil
}

func unmarshalEntityCandidate(raw json.RawMessage, out *EntityCandidate) error {
	raw = bytes.TrimSpace(raw)
	if bytes.HasPrefix(raw, []byte("{")) {
		return json.Unmarshal(raw, out)
	}
	value := rawJSONValueString(raw)
	*out = EntityCandidate{ID: LooseText(value)}
	return nil
}

type EntityCandidate struct {
	ID         LooseText `json:"id,omitempty"`
	Label      LooseText `json:"label,omitempty"`
	Evidence   LooseText `json:"evidence,omitempty"`
	Confidence LooseText `json:"confidence,omitempty"`
}

type ReconcileReport struct {
	Status         LooseText        `json:"status,omitempty"`
	ExpectedAnswer LooseText        `json:"expected_answer,omitempty"`
	ActualAnswer   LooseText        `json:"actual_answer,omitempty"`
	Differences    []string         `json:"differences,omitempty"`
	Groups         []ReconcileGroup `json:"groups,omitempty"`
}

type ReconcileGroup struct {
	GroupKey   LooseText `json:"group_key,omitempty"`
	Metric     LooseText `json:"metric,omitempty"`
	Expected   LooseText `json:"expected,omitempty"`
	Actual     LooseText `json:"actual,omitempty"`
	Difference LooseText `json:"difference,omitempty"`
}

func (r *ReconcileGroup) UnmarshalJSON(data []byte) error {
	type reconcileGroupAlias ReconcileGroup
	var known reconcileGroupAlias
	if err := json.Unmarshal(data, &known); err != nil {
		return err
	}
	*r = ReconcileGroup(known)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	if r.GroupKey.String() == "" {
		r.GroupKey = LooseText(rawAliasString(raw, "group", "dimensions", "dimension_key", "bucket"))
	}
	if r.Metric.String() == "" {
		r.Metric = LooseText(rawAliasString(raw, "measure", "field", "metric_name"))
	}
	if r.Expected.String() == "" {
		r.Expected = LooseText(rawAliasString(raw, "expected_value", "expected_total"))
	}
	if r.Actual.String() == "" {
		r.Actual = LooseText(rawAliasString(raw, "actual_value", "actual_total"))
	}
	if r.Difference.String() == "" {
		r.Difference = LooseText(rawAliasString(raw, "delta", "diff"))
	}
	return nil
}

type RowDecision struct {
	RowID            string            `json:"row_id,omitempty"`
	Source           string            `json:"source,omitempty"`
	SourceLocator    string            `json:"source_locator,omitempty"`
	Decision         string            `json:"decision,omitempty"`
	Reason           string            `json:"reason,omitempty"`
	Value            string            `json:"value,omitempty"`
	Contribution     string            `json:"contribution,omitempty"`
	NormalizedFields map[string]string `json:"normalized_fields,omitempty"`
	EvidenceRef      []string          `json:"evidence_refs,omitempty"`
	RuleRefs         []string          `json:"rule_refs,omitempty"`
}

func (r *RowDecision) UnmarshalJSON(data []byte) error {
	type rowDecisionAlias RowDecision
	var known rowDecisionAlias
	if err := json.Unmarshal(data, &known); err != nil {
		return err
	}
	*r = RowDecision(known)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	knownKeys := map[string]bool{
		"row_id": true, "source": true, "source_locator": true, "decision": true,
		"reason": true, "value": true, "contribution": true, "normalized_fields": true,
		"evidence_refs": true, "rule_refs": true,
	}
	if r.RowID == "" {
		r.RowID = rawAliasString(raw, "item_id", "record_id", "id", "item")
	}
	if r.SourceLocator == "" {
		r.SourceLocator = rawAliasString(raw, "locator", "location", "span", "cell", "row")
	}
	if r.Decision == "" {
		r.Decision = rawAliasString(raw, "status", "outcome", "action")
	}
	if len(r.RuleRefs) == 0 {
		r.RuleRefs = rawAliasStringSlice(raw, "rule_refs", "rule_ref", "rule_id", "rule")
	}
	for key, value := range raw {
		if knownKeys[key] || len(value) == 0 || string(value) == "null" {
			continue
		}
		if r.NormalizedFields == nil {
			r.NormalizedFields = map[string]string{}
		}
		if _, exists := r.NormalizedFields[key]; !exists {
			r.NormalizedFields[key] = rawJSONValueString(value)
		}
	}
	return nil
}

func rawAliasString(raw map[string]json.RawMessage, names ...string) string {
	for _, name := range names {
		value, ok := raw[name]
		if !ok || len(value) == 0 || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			continue
		}
		text := rawJSONValueString(value)
		if strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func rawAliasStringSlice(raw map[string]json.RawMessage, names ...string) []string {
	for _, name := range names {
		value, ok := raw[name]
		if !ok {
			continue
		}
		value = bytes.TrimSpace(value)
		if len(value) == 0 || bytes.Equal(value, []byte("null")) {
			continue
		}
		if bytes.HasPrefix(value, []byte("[")) {
			var items []json.RawMessage
			if err := json.Unmarshal(value, &items); err == nil {
				out := make([]string, 0, len(items))
				for _, item := range items {
					text := strings.TrimSpace(rawJSONValueString(item))
					if text != "" {
						out = append(out, text)
					}
				}
				if len(out) > 0 {
					return out
				}
				continue
			}
		}
		text := strings.TrimSpace(rawJSONValueString(value))
		if text != "" {
			return []string{text}
		}
	}
	return nil
}

func rawJSONValueString(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return clampOneLine(s, 500)
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err == nil {
		return clampOneLine(buf.String(), 500)
	}
	return clampOneLine(string(raw), 500)
}

type Metric struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Unit  string `json:"unit,omitempty"`
}

type EvaluationStatus string

const (
	EvalComplete              EvaluationStatus = "complete"
	EvalContinueData          EvaluationStatus = "continue_data"
	EvalExpandGraph           EvaluationStatus = "expand_graph"
	EvalRepairNode            EvaluationStatus = "repair_node"
	EvalContinueTransform     EvaluationStatus = "continue_transform"
	EvalNeedsClarification    EvaluationStatus = "needs_clarification"
	EvalBlocked               EvaluationStatus = "blocked"
	EvalBudgetExhausted       EvaluationStatus = "budget_exhausted"
	EvalPartialAnswerPossible EvaluationStatus = "partial_answer_possible"
)

type Evaluation struct {
	Status        EvaluationStatus `json:"status"`
	Reason        string           `json:"reason,omitempty"`
	Confidence    string           `json:"confidence,omitempty"`
	MissingInputs []string         `json:"missing_inputs,omitempty"`
	ActionID      string           `json:"action_id,omitempty"`
	ActionKind    string           `json:"action_kind,omitempty"`
	RepairLocus   string           `json:"repair_locus,omitempty"`
}

func NormalizeEvaluationStatus(status string) EvaluationStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case string(EvalComplete):
		return EvalComplete
	case string(EvalExpandGraph), "expand", "graph", "continue_graph":
		return EvalExpandGraph
	case string(EvalRepairNode), "repair", "node_repair":
		return EvalRepairNode
	case string(EvalContinueTransform), "transform":
		return EvalContinueTransform
	case string(EvalContinueData), "continue", "ready":
		return EvalContinueData
	case string(EvalNeedsClarification):
		return EvalNeedsClarification
	case string(EvalBlocked):
		return EvalBlocked
	case string(EvalBudgetExhausted):
		return EvalBudgetExhausted
	case string(EvalPartialAnswerPossible):
		return EvalPartialAnswerPossible
	default:
		return EvalPartialAnswerPossible
	}
}

type CandidateFile struct {
	Path              string     `json:"path"`
	Size              int64      `json:"size"`
	Kind              string     `json:"kind"`
	ExtractionStatus  string     `json:"extraction_status,omitempty"`
	TextEvidencePaths []string   `json:"text_evidence_paths,omitempty"`
	Delimiter         string     `json:"delimiter,omitempty"`
	Lines             int        `json:"lines,omitempty"`
	Headers           []string   `json:"headers,omitempty"`
	Sample            []string   `json:"sample,omitempty"`
	SampleRows        [][]string `json:"sample_rows,omitempty"`
	InspectError      string     `json:"inspect_error,omitempty"`
}

type Runner struct {
	RepoRoot      string
	TempRoot      string
	Timeout       time.Duration
	MaxFileBytes  int64
	MaxTotalBytes int64
}

const (
	defaultTimeout       = 30 * time.Second
	defaultMaxFileBytes  = int64(32 << 20)
	defaultMaxTotalBytes = int64(128 << 20)
	resultMarker         = "__DATA_RESULT_JSON__"
)

func DiscoverCandidateFiles(root string, limit int) ([]CandidateFile, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 200
	}
	var out []CandidateFile
	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			switch name {
			case ".git", ".codrax", "node_modules", "vendor", "dist", "build", ".venv", "__pycache__":
				if path != absRoot {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if len(out) >= limit {
			return filepath.SkipAll
		}
		kind := dataKindForPath(path)
		if kind == "" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(absRoot, path)
		if err != nil {
			return nil
		}
		out = append(out, CandidateFile{
			Path: filepath.ToSlash(rel),
			Size: info.Size(),
			Kind: kind,
		})
		out[len(out)-1] = inspectCandidateFile(path, out[len(out)-1])
		return nil
	})
	if err != nil {
		return nil, err
	}
	attachRelatedTextEvidence(out)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Path < out[j].Path
	})
	return out, nil
}

func inspectCandidateFile(path string, f CandidateFile) CandidateFile {
	if isNonTextMaterialKind(f.Kind) {
		f.ExtractionStatus = "needs_text_extraction"
		f.InspectError = "non-text material; semantic content requires extracted text before deterministic data processing"
		return f
	}
	if isRunnerInputKind(f.Kind) {
		f.ExtractionStatus = "text_ready"
	}
	switch f.Kind {
	case "csv", "tsv":
		return inspectDelimitedCandidate(path, f)
	case "json":
		return inspectJSONCandidate(path, f)
	case "jsonl":
		return inspectJSONLCandidate(path, f)
	case "text":
		return inspectTextCandidate(path, f)
	default:
		return f
	}
}

func inspectDelimitedCandidate(path string, f CandidateFile) CandidateFile {
	file, err := os.Open(path)
	if err != nil {
		f.InspectError = err.Error()
		return f
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	f.Delimiter = ","
	if f.Kind == "tsv" {
		reader.Comma = '\t'
		f.Delimiter = "\\t"
	}
	header, err := reader.Read()
	if err != nil {
		if err != io.EOF {
			f.InspectError = err.Error()
		}
		return f
	}
	f.Headers = clampStringSlice(cleanStringSlice(header), 40)
	lines := 1
	for len(f.SampleRows) < 3 {
		row, err := reader.Read()
		if err != nil {
			break
		}
		lines++
		f.SampleRows = append(f.SampleRows, clampStringSlice(cleanStringSlice(row), 40))
	}
	for {
		_, err := reader.Read()
		if err != nil {
			break
		}
		lines++
		if lines > 1000000 {
			break
		}
	}
	f.Lines = lines
	return f
}

func inspectJSONCandidate(path string, f CandidateFile) CandidateFile {
	data, err := os.ReadFile(path)
	if err != nil {
		f.InspectError = err.Error()
		return f
	}
	if len(data) > 256<<10 {
		data = data[:256<<10]
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		f.Sample = []string{clampOneLine(string(data), 240)}
		return f
	}
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		f.Headers = clampStringSlice(keys, 40)
	case []any:
		f.Lines = len(x)
		if len(x) > 0 {
			if m, ok := x[0].(map[string]any); ok {
				keys := make([]string, 0, len(m))
				for k := range m {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				f.Headers = clampStringSlice(keys, 40)
			}
		}
	}
	raw, _ := json.Marshal(v)
	if len(raw) > 0 {
		f.Sample = []string{clampOneLine(string(raw), 240)}
	}
	return f
}

func inspectJSONLCandidate(path string, f CandidateFile) CandidateFile {
	file, err := os.Open(path)
	if err != nil {
		f.InspectError = err.Error()
		return f
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64<<10), 1024<<10)
	keys := map[string]bool{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		f.Lines++
		if len(f.Sample) < 3 {
			f.Sample = append(f.Sample, clampOneLine(line, 240))
		}
		if len(keys) < 80 {
			var m map[string]any
			if err := json.Unmarshal([]byte(line), &m); err == nil {
				for k := range m {
					keys[k] = true
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		f.InspectError = err.Error()
	}
	for k := range keys {
		f.Headers = append(f.Headers, k)
	}
	sort.Strings(f.Headers)
	f.Headers = clampStringSlice(f.Headers, 40)
	return f
}

func inspectTextCandidate(path string, f CandidateFile) CandidateFile {
	file, err := os.Open(path)
	if err != nil {
		f.InspectError = err.Error()
		return f
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64<<10), 1024<<10)
	for scanner.Scan() {
		f.Lines++
		if len(f.Sample) < 4 {
			f.Sample = append(f.Sample, clampOneLine(scanner.Text(), 240))
		}
		if f.Lines > 1000000 {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		f.InspectError = err.Error()
	}
	return f
}

func cleanStringSlice(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

func clampStringSlice(in []string, limit int) []string {
	if limit <= 0 || len(in) <= limit {
		return in
	}
	return append(append([]string(nil), in[:limit]...), fmt.Sprintf("...%d more", len(in)-limit))
}

func clampOneLine(s string, limit int) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if limit > 0 && len(s) > limit {
		return s[:limit] + "...[truncated]"
	}
	return s
}

func dataKindForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv":
		return "csv"
	case ".tsv":
		return "tsv"
	case ".json":
		return "json"
	case ".jsonl", ".ndjson":
		return "jsonl"
	case ".txt", ".md":
		return "text"
	case ".png", ".jpg", ".jpeg", ".webp", ".bmp", ".gif", ".heic":
		return "image"
	case ".pdf":
		return "pdf"
	default:
		return ""
	}
}

func isRunnerInputKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "csv", "tsv", "json", "jsonl", "text":
		return true
	default:
		return false
	}
}

func isNonTextMaterialKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "image", "pdf":
		return true
	default:
		return false
	}
}

func candidateNeedsTextExtraction(f CandidateFile) bool {
	status := strings.TrimSpace(f.ExtractionStatus)
	return status == "needs_text_extraction" || status == "related_text_available" || isNonTextMaterialKind(f.Kind)
}

func attachRelatedTextEvidence(files []CandidateFile) {
	textByStem := map[string][]string{}
	for _, f := range files {
		if !isRunnerInputKind(f.Kind) {
			continue
		}
		stem := materialStem(f.Path)
		if stem == "" {
			continue
		}
		textByStem[stem] = append(textByStem[stem], f.Path)
	}
	for i := range files {
		if !isNonTextMaterialKind(files[i].Kind) {
			continue
		}
		stem := materialStem(files[i].Path)
		if stem == "" {
			continue
		}
		related := append([]string(nil), textByStem[stem]...)
		sort.Strings(related)
		if len(related) > 8 {
			related = related[:8]
		}
		files[i].TextEvidencePaths = related
		if len(related) > 0 {
			files[i].ExtractionStatus = "related_text_available"
			files[i].InspectError = "non-text material; related text evidence candidate(s) are available"
		}
	}
}

func materialStem(path string) string {
	base := filepath.Base(strings.TrimSpace(path))
	if base == "." || base == "" {
		return ""
	}
	ext := filepath.Ext(base)
	return strings.ToLower(strings.TrimSpace(strings.TrimSuffix(base, ext)))
}

func (r Runner) Run(ctx context.Context, plan TaskPlan) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	repoRoot := strings.TrimSpace(r.RepoRoot)
	if repoRoot == "" {
		repoRoot = "."
	}
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return Result{}, err
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	maxFile := r.MaxFileBytes
	if maxFile <= 0 {
		maxFile = defaultMaxFileBytes
	}
	maxTotal := r.MaxTotalBytes
	if maxTotal <= 0 {
		maxTotal = defaultMaxTotalBytes
	}
	if strings.TrimSpace(plan.Script) == "" {
		return Result{}, errors.New("data task plan has empty script")
	}
	if err := validateScriptSafety(plan.Script); err != nil {
		return Result{}, err
	}
	if len(plan.InputPaths) == 0 {
		return Result{}, errors.New("data task plan has no input paths")
	}
	tempRoot := strings.TrimSpace(r.TempRoot)
	if tempRoot == "" {
		tempRoot = os.TempDir()
	}
	tempRoot, err = filepath.Abs(tempRoot)
	if err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(tempRoot, 0700); err != nil {
		return Result{}, err
	}
	workDir, err := os.MkdirTemp(tempRoot, "codrax-data-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(workDir)

	relPaths, err := copyInputs(absRoot, workDir, plan.InputPaths, maxFile, maxTotal)
	if err != nil {
		return Result{}, err
	}
	if err := validateCoverageInputsDeclared(plan.CoverageContract, relPaths); err != nil {
		return Result{}, err
	}
	helperPath := filepath.Join(workDir, "_runner.py")
	if err := os.WriteFile(helperPath, []byte(renderPythonHelper(plan.Script, relPaths)), 0600); err != nil {
		return Result{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "python3", helperPath)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "PYTHONNOUSERSITE=1")
	out, err := cmd.CombinedOutput()
	if runCtx.Err() == context.DeadlineExceeded {
		return Result{}, fmt.Errorf("data task timed out after %s", timeout)
	}
	if err != nil {
		return Result{}, fmt.Errorf("data task script failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	res, err := parseRunnerResult(out)
	if err != nil {
		return Result{}, err
	}
	res.ResultPatches = append(res.ResultPatches, applyDataResultPatchEngine(&res)...)
	return validateRunnerResult(plan, res)
}

func validateRunnerResult(plan TaskPlan, res Result) (Result, error) {
	res.ConsumedPaths = normalizeMaterialPaths(res.ConsumedPaths)
	if err := validateCoverageConsumed(plan.CoverageContract, res.ConsumedPaths); err != nil {
		return Result{}, err
	}
	if plan.CoverageContract.DecisionRecordsRequired && len(res.Rows) == 0 {
		return Result{}, wrapDataResultValidationError(res, dataValidationError(
			"missing_required_ledger",
			"/rows",
			"non-empty array of decision records",
			"empty",
			RepairabilityNeedsRecompute,
			"data coverage incomplete: coverage_contract.decision_records_required=true but result.rows is empty",
		))
	}
	if plan.CoverageContract.DecisionRecordsRequired && !hasMeaningfulRowDecision(res.Rows) {
		return Result{}, wrapDataResultValidationError(res, dataValidationError(
			"empty_semantic_ledger",
			"/rows",
			"decision records with source locator and include/exclude/transform decision",
			snippetJSON(res.Rows),
			RepairabilityNeedsRecompute,
			"data coverage incomplete: coverage_contract.decision_records_required=true but result.rows contains no meaningful decision records",
		))
	}
	if err := validateRequiredLedgers(plan.CoverageContract, res); err != nil {
		return Result{}, wrapDataResultValidationError(res, err)
	}
	if res.OutputContract.Format == "" {
		res.OutputContract = plan.OutputContract
	}
	res.OutputContract = res.OutputContract.Normalize()
	res.Answer, res.ContractWarnings = normalizeAnswerForContract(res.Answer, res.OutputContract, res.ContractWarnings)
	if err := ValidateAnswer(res.Answer, res.OutputContract); err != nil {
		res.ContractWarnings = append(res.ContractWarnings, err.Error())
	}
	return res, nil
}

func validateRequiredLedgers(contract CoverageContract, res Result) error {
	if contract.RuleCoverageRequired && len(res.RuleCoverage) == 0 {
		return dataValidationError(
			"missing_required_ledger",
			"/rule_coverage",
			"non-empty array of rule coverage records",
			"empty",
			RepairabilityNeedsRecompute,
			"data validation incomplete: coverage_contract.rule_coverage_required=true but result.rule_coverage is empty",
		)
	}
	if contract.RuleCoverageRequired && !hasMeaningfulRuleCoverage(res.RuleCoverage) {
		return dataValidationError(
			"empty_semantic_ledger",
			"/rule_coverage",
			"rule coverage records with rule_id/rule_text/status and support",
			snippetJSON(res.RuleCoverage),
			RepairabilityNeedsRecompute,
			"data validation incomplete: result.rule_coverage contains no meaningful rule records",
		)
	}
	if contract.RuleCoverageRequired {
		if err := validateRuleCoverageRecords(res.RuleCoverage, res.Rows, res.Contributions, res.EntityResolutions); err != nil {
			return err
		}
	}
	if contract.ContributionLedgerRequired && len(res.Contributions) == 0 {
		return dataValidationError(
			"missing_required_ledger",
			"/contributions",
			"non-empty array of contribution records",
			"empty",
			RepairabilityNeedsRecompute,
			"data validation incomplete: coverage_contract.contribution_ledger_required=true but result.contributions is empty",
		)
	}
	if contract.ContributionLedgerRequired && !hasMeaningfulContribution(res.Contributions) {
		return dataValidationError(
			"empty_semantic_ledger",
			"/contributions",
			"contribution records with group_key, metric, value, operation, and source",
			snippetJSON(res.Contributions),
			RepairabilityNeedsRecompute,
			"data validation incomplete: result.contributions contains no canonical contribution records; include item_id/source/source_locator/evidence_refs plus group_key/metric/value/operation/reason as needed for the task",
		)
	}
	if contract.ContributionLedgerRequired {
		if err := validateContributionRecords(res.Contributions); err != nil {
			return err
		}
	}
	if contract.EntityResolutionRequired && len(res.EntityResolutions) == 0 {
		return dataValidationError(
			"missing_required_ledger",
			"/entity_resolutions",
			"non-empty array of entity resolution records",
			"empty",
			RepairabilityNeedsRecompute,
			"data validation incomplete: coverage_contract.entity_resolution_required=true but result.entity_resolutions is empty",
		)
	}
	if contract.EntityResolutionRequired && !hasMeaningfulEntityResolution(res.EntityResolutions) {
		return dataValidationError(
			"empty_semantic_ledger",
			"/entity_resolutions",
			"entity resolution records with source value plus canonical value or unresolved reason",
			snippetJSON(res.EntityResolutions),
			RepairabilityNeedsRecompute,
			"data validation incomplete: result.entity_resolutions contains no meaningful resolution records",
		)
	}
	if contract.EntityResolutionRequired {
		if err := validateEntityResolutionRecords(res.EntityResolutions); err != nil {
			return err
		}
	}
	if contract.ReconcileRequired {
		if res.Reconcile == nil {
			return dataValidationError(
				"missing_required_ledger",
				"/reconcile",
				"reconcile object with status and/or groups",
				"empty",
				RepairabilityNeedsRecompute,
				"data validation incomplete: coverage_contract.reconcile_required=true but result.reconcile is empty",
			)
		}
		if err := validateReconcileReport(*res.Reconcile, res.Contributions, res.Answer); err != nil {
			return err
		}
	}
	return nil
}

func hasMeaningfulRuleCoverage(records []RuleCoverageRecord) bool {
	for _, rec := range records {
		if strings.TrimSpace(rec.RuleID.String()) != "" ||
			strings.TrimSpace(rec.RuleText.String()) != "" ||
			strings.TrimSpace(rec.Status.String()) != "" ||
			strings.TrimSpace(rec.Notes.String()) != "" ||
			len(rec.EvidenceRefs) > 0 {
			return true
		}
	}
	return false
}

func validateRuleCoverageRecords(records []RuleCoverageRecord, rows []RowDecision, contributions []ContributionRecord, resolutions []EntityResolutionRecord) error {
	knownRules := map[string]bool{}
	for i, rec := range records {
		ruleID := strings.TrimSpace(rec.RuleID.String())
		ruleText := strings.TrimSpace(rec.RuleText.String())
		status := strings.TrimSpace(rec.Status.String())
		if ruleID == "" && ruleText == "" {
			return dataValidationError("missing_rule_coverage_identity", fmt.Sprintf("/rule_coverage/%d", i), "rule_id or rule_text", "", RepairabilityNeedsRecompute,
				"data validation incomplete: result.rule_coverage[%d] is missing rule_id or rule_text", i)
		}
		if status == "" {
			return dataValidationError("missing_rule_coverage_status", fmt.Sprintf("/rule_coverage/%d/status", i), "status", "", RepairabilityNeedsRecompute,
				"data validation incomplete: result.rule_coverage[%d] is missing status", i)
		}
		if ruleID != "" {
			knownRules[ruleID] = true
		}
	}
	linked := map[string]bool{}
	if err := collectRuleRefs(linked, knownRules, "rows", rowRuleRefs(rows)); err != nil {
		return err
	}
	if err := collectRuleRefs(linked, knownRules, "contributions", contributionRuleRefs(contributions)); err != nil {
		return err
	}
	if err := collectRuleRefs(linked, knownRules, "entity_resolutions", entityResolutionRuleRefs(resolutions)); err != nil {
		return err
	}
	for i, rec := range records {
		ruleID := strings.TrimSpace(rec.RuleID.String())
		if len(rec.EvidenceRefs) > 0 ||
			strings.TrimSpace(rec.Notes.String()) != "" ||
			(ruleID != "" && linked[ruleID]) {
			continue
		}
		return dataValidationError("unsupported_rule_coverage", fmt.Sprintf("/rule_coverage/%d", i), "notes, evidence_refs, or linked rule_refs", "", RepairabilityNeedsRecompute,
			"data validation incomplete: result.rule_coverage[%d] has no evidence_refs, notes, or linked rule_refs", i)
	}
	return nil
}

func rowRuleRefs(rows []RowDecision) [][]string {
	out := make([][]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.RuleRefs)
	}
	return out
}

func contributionRuleRefs(records []ContributionRecord) [][]string {
	out := make([][]string, 0, len(records))
	for _, rec := range records {
		out = append(out, rec.RuleRefs)
	}
	return out
}

func entityResolutionRuleRefs(records []EntityResolutionRecord) [][]string {
	out := make([][]string, 0, len(records))
	for _, rec := range records {
		out = append(out, rec.RuleRefs)
	}
	return out
}

func collectRuleRefs(linked, known map[string]bool, label string, groups [][]string) error {
	for i, refs := range groups {
		for _, ref := range refs {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				continue
			}
			if len(known) > 0 && !known[ref] {
				return dataValidationError("unknown_rule_ref", fmt.Sprintf("/%s/%d/rule_refs", label, i), "rule_ref matching rule_coverage.rule_id", ref, RepairabilityNeedsRecompute,
					"data validation incomplete: result.%s[%d] references unknown rule_id %q", label, i, ref)
			}
			linked[ref] = true
		}
	}
	return nil
}

func hasMeaningfulContribution(records []ContributionRecord) bool {
	for _, rec := range records {
		if contributionHasAnchor(rec) && contributionHasEffect(rec) {
			return true
		}
	}
	return false
}

func validateContributionRecords(records []ContributionRecord) error {
	for i, rec := range records {
		if _, ok := normalizeContributionOperation(rec.Operation.String()); !ok {
			return dataValidationError("unsupported_contribution_operation", fmt.Sprintf("/contributions/%d/operation", i), "add/sum/include/count/subtract/set/rank", strings.TrimSpace(rec.Operation.String()), RepairabilityNeedsRecompute,
				"data validation incomplete: result.contributions[%d] has unsupported operation %q; use add/sum/include/count/subtract/set/rank", i, strings.TrimSpace(rec.Operation.String()))
		}
		if contributionHasAnchor(rec) && contributionHasEffect(rec) {
			continue
		}
		return dataValidationError("missing_contribution_canonical_fields", fmt.Sprintf("/contributions/%d", i), "anchor plus effect fields", "", RepairabilityNeedsRecompute,
			"data validation incomplete: result.contributions[%d] is missing canonical item/effect fields; use item_id/source/source_locator/evidence_refs plus group_key/metric/value/operation/reason instead of task-specific aliases", i)
	}
	return nil
}

func contributionHasAnchor(rec ContributionRecord) bool {
	return strings.TrimSpace(rec.ItemID.String()) != "" ||
		strings.TrimSpace(rec.Source.String()) != "" ||
		strings.TrimSpace(rec.SourceLocator.String()) != "" ||
		len(rec.EvidenceRefs) > 0
}

func contributionHasEffect(rec ContributionRecord) bool {
	return strings.TrimSpace(rec.GroupKey.String()) != "" ||
		strings.TrimSpace(rec.Metric.String()) != "" ||
		strings.TrimSpace(rec.Value.String()) != "" ||
		strings.TrimSpace(rec.Operation.String()) != "" ||
		strings.TrimSpace(rec.Reason.String()) != ""
}

func hasMeaningfulEntityResolution(records []EntityResolutionRecord) bool {
	for _, rec := range records {
		if strings.TrimSpace(rec.ItemID.String()) != "" ||
			strings.TrimSpace(rec.SourceValue.String()) != "" ||
			strings.TrimSpace(rec.CanonicalID.String()) != "" ||
			strings.TrimSpace(rec.CanonicalLabel.String()) != "" ||
			strings.TrimSpace(rec.Status.String()) != "" ||
			strings.TrimSpace(rec.Reason.String()) != "" ||
			len(rec.Candidates) > 0 ||
			len(rec.EvidenceRefs) > 0 {
			return true
		}
	}
	return false
}

func validateEntityResolutionRecords(records []EntityResolutionRecord) error {
	for i, rec := range records {
		source := strings.TrimSpace(rec.SourceValue.String())
		if source == "" {
			source = strings.TrimSpace(rec.ItemID.String())
		}
		if source == "" {
			return dataValidationError("missing_entity_resolution_source", fmt.Sprintf("/entity_resolutions/%d", i), "source_value or item_id", "", RepairabilityNeedsRecompute,
				"data validation incomplete: result.entity_resolutions[%d] is missing source_value or item_id", i)
		}
		status := strings.TrimSpace(rec.Status.String())
		if status == "" {
			return dataValidationError("missing_entity_resolution_status", fmt.Sprintf("/entity_resolutions/%d/status", i), "status", "", RepairabilityNeedsRecompute,
				"data validation incomplete: result.entity_resolutions[%d] is missing status", i)
		}
		if resolutionStatusIsOpen(status) {
			if strings.TrimSpace(rec.Reason.String()) == "" && len(rec.Candidates) == 0 && len(rec.EvidenceRefs) == 0 {
				return dataValidationError("unsupported_open_entity_resolution", fmt.Sprintf("/entity_resolutions/%d", i), "reason, candidates, or evidence_refs", status, RepairabilityNeedsRecompute,
					"data validation incomplete: result.entity_resolutions[%d] is %q but has no reason, candidates, or evidence_refs", i, status)
			}
			continue
		}
		if strings.TrimSpace(rec.CanonicalID.String()) == "" && strings.TrimSpace(rec.CanonicalLabel.String()) == "" {
			return dataValidationError("missing_entity_resolution_canonical", fmt.Sprintf("/entity_resolutions/%d", i), "canonical_id or canonical_label", status, RepairabilityNeedsRecompute,
				"data validation incomplete: result.entity_resolutions[%d] resolved status %q is missing canonical_id or canonical_label", i, status)
		}
	}
	return nil
}

func resolutionStatusIsOpen(status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	for _, marker := range []string{"unresolved", "ambiguous", "unknown", "not_found", "no_match", "failed", "invalid", "open"} {
		if strings.Contains(status, marker) {
			return true
		}
	}
	return false
}

func validateReconcileReport(report ReconcileReport, contributions []ContributionRecord, answer string) error {
	status := strings.ToLower(strings.TrimSpace(report.Status.String()))
	if status == "" {
		return dataValidationError("missing_reconcile_status", "/reconcile/status", "pass/fail/partial/blocked status", "", RepairabilityNeedsRecompute,
			"data validation incomplete: result.reconcile.status is empty")
	}
	if status != "pass" {
		if len(report.Differences) > 0 {
			return dataValidationError("reconcile_status_failed", "/reconcile/status", "pass after deterministic reconciliation succeeds", status, RepairabilityNeedsRecompute,
				"data reconcile failed: status=%s differences=%s", status, strings.Join(report.Differences, "; "))
		}
		return dataValidationError("reconcile_status_failed", "/reconcile/status", "pass after deterministic reconciliation succeeds", status, RepairabilityNeedsRecompute,
			"data reconcile failed: status=%s", status)
	}
	expectedAnswer := strings.TrimSpace(report.ExpectedAnswer.String())
	actualAnswer := strings.TrimSpace(report.ActualAnswer.String())
	answer = strings.TrimSpace(answer)
	if expectedAnswer != "" && answer != "" && expectedAnswer != answer {
		return fmt.Errorf("data reconcile failed: expected_answer %q does not match result.answer %q", expectedAnswer, answer)
	}
	if actualAnswer != "" && answer != "" && actualAnswer != answer {
		return fmt.Errorf("data reconcile failed: actual_answer %q does not match result.answer %q", actualAnswer, answer)
	}
	if len(contributions) > 0 && len(report.Groups) == 0 {
		return dataValidationError("missing_reconcile_groups", "/reconcile/groups", "group for every contribution group", "empty", RepairabilityNeedsRecompute,
			"data reconcile incomplete: result.reconcile.groups is empty while contributions are present")
	}
	if len(contributions) > 0 && len(report.Groups) > 0 && !hasMeaningfulReconcileGroup(report.Groups) {
		return dataValidationError("empty_semantic_ledger", "/reconcile/groups", "reconcile groups with group_key/metric and expected/actual values", snippetJSON(report.Groups), RepairabilityNeedsRecompute,
			"data reconcile incomplete: result.reconcile.groups contains no canonical group records; include group_key/metric and expected/actual values instead of task-specific aliases")
	}
	if len(report.Groups) == 0 || len(contributions) == 0 {
		return nil
	}
	sums := sumContributionGroups(contributions)
	seenGroups := map[string]bool{}
	for i, group := range report.Groups {
		key := reconcileGroupKey(group.GroupKey.String(), group.Metric.String())
		if strings.TrimSpace(group.Expected.String()) == "" && strings.TrimSpace(group.Actual.String()) == "" {
			return dataValidationError("missing_reconcile_group_value", fmt.Sprintf("/reconcile/groups/%d", i), "expected or actual", "", RepairabilityNeedsRecompute,
				"data reconcile incomplete: group %q is missing expected or actual value", displayReconcileGroupKey(group.GroupKey.String(), group.Metric.String()))
		}
		seenGroups[key] = true
		got, ok := sums[key]
		if !ok {
			if reconcileGroupDeclaresZero(group) {
				continue
			}
			return dataValidationError("reconcile_group_mismatch", fmt.Sprintf("/reconcile/groups/%d", i), "group_key/metric matching contribution records", displayReconcileGroupKey(group.GroupKey.String(), group.Metric.String()), RepairabilityNeedsRecompute,
				"data reconcile failed: group %q has no matching contribution records; available contribution groups: %s",
				displayReconcileGroupKey(group.GroupKey.String(), group.Metric.String()),
				displayContributionGroupKeys(sums))
		}
		for _, candidate := range []struct {
			label string
			value string
		}{
			{label: "expected", value: group.Expected.String()},
			{label: "actual", value: group.Actual.String()},
		} {
			value := strings.TrimSpace(candidate.value)
			if value == "" {
				continue
			}
			want, err := parseDecimalRat(value)
			if err != nil {
				continue
			}
			if got.Cmp(want) != 0 {
				return dataValidationError("reconcile_sum_mismatch", fmt.Sprintf("/reconcile/groups/%d/%s", i, candidate.label), "value equal to contribution sum", value, RepairabilityNeedsRecompute,
					"data reconcile failed: group %q %s=%s but contributions sum to %s",
					displayReconcileGroupKey(group.GroupKey.String(), group.Metric.String()), candidate.label, value, formatRat(got))
			}
		}
	}
	for key := range sums {
		if !seenGroups[key] {
			groupKey, metric := splitReconcileGroupKey(key)
			return dataValidationError("missing_reconcile_group", "/reconcile/groups", "group for every contribution group", displayReconcileGroupKey(groupKey, metric), RepairabilityNeedsRecompute,
				"data reconcile incomplete: contributions include group %q but result.reconcile.groups does not report it", displayReconcileGroupKey(groupKey, metric))
		}
	}
	return nil
}

func reconcileGroupDeclaresZero(group ReconcileGroup) bool {
	seenNumeric := false
	for _, value := range []string{group.Expected.String(), group.Actual.String()} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parsed, err := parseDecimalRat(value)
		if err != nil {
			continue
		}
		seenNumeric = true
		if parsed.Sign() != 0 {
			return false
		}
	}
	return seenNumeric
}

func displayContributionGroupKeys(sums map[string]*big.Rat) string {
	if len(sums) == 0 {
		return "(none)"
	}
	keys := make([]string, 0, len(sums))
	for key := range sums {
		groupKey, metric := splitReconcileGroupKey(key)
		keys = append(keys, displayReconcileGroupKey(groupKey, metric))
	}
	sort.Strings(keys)
	const limit = 12
	if len(keys) > limit {
		keys = append(keys[:limit], fmt.Sprintf("... %d more", len(keys)-limit))
	}
	return strings.Join(keys, ", ")
}

func hasMeaningfulReconcileGroup(groups []ReconcileGroup) bool {
	for _, group := range groups {
		if strings.TrimSpace(group.GroupKey.String()) != "" ||
			strings.TrimSpace(group.Metric.String()) != "" ||
			strings.TrimSpace(group.Expected.String()) != "" ||
			strings.TrimSpace(group.Actual.String()) != "" ||
			strings.TrimSpace(group.Difference.String()) != "" {
			return true
		}
	}
	return false
}

func sumContributionGroups(contributions []ContributionRecord) map[string]*big.Rat {
	out := map[string]*big.Rat{}
	for _, rec := range contributions {
		op, ok := normalizeContributionOperation(rec.Operation.String())
		if !ok {
			continue
		}
		switch op {
		case "", "include", "add", "count", "set", "rank":
		case "subtract":
		}
		valueText := strings.TrimSpace(rec.Value.String())
		if valueText == "" {
			if op == "count" {
				valueText = "1"
			} else {
				continue
			}
		}
		value, err := parseDecimalRat(valueText)
		if err != nil {
			continue
		}
		if op == "subtract" {
			value.Neg(value)
		}
		key := reconcileGroupKey(rec.GroupKey.String(), rec.Metric.String())
		if _, ok := out[key]; !ok {
			out[key] = new(big.Rat)
		}
		out[key].Add(out[key], value)
	}
	return out
}

func reconcileGroupKey(groupKey, metric string) string {
	metric = strings.TrimSpace(metric)
	groupKey = normalizeLedgerGroupKey(groupKey, metric)
	return groupKey + "\x00" + metric
}

func displayReconcileGroupKey(groupKey, metric string) string {
	groupKey = strings.TrimSpace(groupKey)
	metric = strings.TrimSpace(metric)
	switch {
	case groupKey != "" && metric != "":
		return groupKey + "/" + metric
	case groupKey != "":
		return groupKey
	case metric != "":
		return metric
	default:
		return "(default)"
	}
}

func splitReconcileGroupKey(key string) (string, string) {
	parts := strings.SplitN(key, "\x00", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

func applyDataResultPatchEngine(res *Result) []DataResultPatch {
	if res == nil {
		return nil
	}
	var patches []DataResultPatch
	for i := range res.Contributions {
		metric := strings.TrimSpace(res.Contributions[i].Metric.String())
		oldGroupKey := res.Contributions[i].GroupKey.String()
		newGroupKey := normalizeLedgerGroupKey(oldGroupKey, metric)
		if oldGroupKey != newGroupKey {
			res.Contributions[i].GroupKey = LooseText(newGroupKey)
			patches = append(patches, newDataResultPatch("replace", fmt.Sprintf("/contributions/%d/group_key", i), newGroupKey, "removed duplicated metric suffix from contribution group_key"))
		}
		if strings.TrimSpace(res.Contributions[i].Operation.String()) != "" {
			oldOp := res.Contributions[i].Operation.String()
			op, ok := normalizeContributionOperation(res.Contributions[i].Operation.String())
			if ok && oldOp != op {
				res.Contributions[i].Operation = LooseText(op)
				patches = append(patches, newDataResultPatch("replace", fmt.Sprintf("/contributions/%d/operation", i), op, "canonicalized contribution operation alias"))
			}
		}
	}
	for i := range res.EntityResolutions {
		if strings.TrimSpace(res.EntityResolutions[i].Status.String()) != "" {
			continue
		}
		if strings.TrimSpace(res.EntityResolutions[i].CanonicalID.String()) == "" &&
			strings.TrimSpace(res.EntityResolutions[i].CanonicalLabel.String()) == "" {
			continue
		}
		res.EntityResolutions[i].Status = "resolved"
		patches = append(patches, newDataResultPatch("replace", fmt.Sprintf("/entity_resolutions/%d/status", i), "resolved", "filled resolved status for entity resolution with canonical value"))
	}
	if res.Reconcile != nil {
		for i := range res.Reconcile.Groups {
			metric := strings.TrimSpace(res.Reconcile.Groups[i].Metric.String())
			oldGroupKey := res.Reconcile.Groups[i].GroupKey.String()
			newGroupKey := normalizeLedgerGroupKey(oldGroupKey, metric)
			if oldGroupKey != newGroupKey {
				res.Reconcile.Groups[i].GroupKey = LooseText(newGroupKey)
				patches = append(patches, newDataResultPatch("replace", fmt.Sprintf("/reconcile/groups/%d/group_key", i), newGroupKey, "removed duplicated metric suffix from reconcile group_key"))
			}
		}
	}
	return patches
}

func ApplyDataResultPatchPlan(plan TaskPlan, base Result, patchPlan DataResultPatchPlan) (Result, []DataResultPatch, error) {
	if len(patchPlan.Patches) == 0 {
		return Result{}, nil, errors.New("data result patch plan has no patches")
	}
	out := cloneResult(base)
	applied := make([]DataResultPatch, 0, len(patchPlan.Patches))
	const maxPatches = 12
	for i, patch := range patchPlan.Patches {
		if i >= maxPatches {
			return Result{}, applied, fmt.Errorf("data result patch plan exceeds patch budget %d", maxPatches)
		}
		normalized, err := applyDataResultPatch(&out, patch)
		if err != nil {
			return Result{}, applied, err
		}
		applied = append(applied, normalized)
	}
	if more := applyDataResultPatchEngine(&out); len(more) > 0 {
		applied = append(applied, more...)
	}
	out.ResultPatches = append(out.ResultPatches, applied...)
	validated, err := validateRunnerResult(plan, out)
	if err != nil {
		return Result{}, applied, err
	}
	return validated, applied, nil
}

func cloneResult(in Result) Result {
	raw, err := json.Marshal(in)
	if err != nil {
		return in
	}
	var out Result
	if err := json.Unmarshal(raw, &out); err != nil {
		return in
	}
	return out
}

func applyDataResultPatch(res *Result, patch DataResultPatch) (DataResultPatch, error) {
	target := strings.TrimSpace(patch.Target)
	if target == "" {
		target = "result"
	}
	if target != "result" {
		return DataResultPatch{}, fmt.Errorf("data result patch rejected: unsupported target %q", target)
	}
	op := strings.ToLower(strings.TrimSpace(patch.Op))
	if op != "replace" {
		return DataResultPatch{}, fmt.Errorf("data result patch rejected: op %q is not allowed; only replace existing structural scalar fields", patch.Op)
	}
	path := strings.TrimSpace(patch.Path)
	if !strings.HasPrefix(path, "/") {
		return DataResultPatch{}, fmt.Errorf("data result patch rejected: path %q is not a JSON pointer", path)
	}
	value := strings.TrimSpace(rawJSONValueString(patch.Value))
	if value == "" {
		return DataResultPatch{}, fmt.Errorf("data result patch rejected: empty value for %s", path)
	}
	reason := strings.TrimSpace(patch.Reason)
	if reason == "" {
		reason = "model-authored structural result patch"
	}
	if err := setSafeResultPatchValue(res, path, value); err != nil {
		return DataResultPatch{}, err
	}
	return newDataResultPatch("replace", path, value, reason), nil
}

func setSafeResultPatchValue(res *Result, pointer, value string) error {
	parts := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	if len(parts) == 0 {
		return fmt.Errorf("data result patch rejected: empty path")
	}
	switch parts[0] {
	case "contributions":
		if len(parts) != 3 {
			return fmt.Errorf("data result patch rejected: unsupported contribution path %q", pointer)
		}
		idx, ok := parseNonNegativeIndex(parts[1], len(res.Contributions))
		if !ok {
			return fmt.Errorf("data result patch rejected: contribution index out of range in %q", pointer)
		}
		switch parts[2] {
		case "operation":
			op, ok := normalizeContributionOperation(value)
			if !ok || op == "" {
				return fmt.Errorf("data result patch rejected: unsupported contribution operation %q", value)
			}
			res.Contributions[idx].Operation = LooseText(op)
		case "group_key":
			res.Contributions[idx].GroupKey = LooseText(normalizeLedgerGroupKey(value, res.Contributions[idx].Metric.String()))
		case "metric":
			res.Contributions[idx].Metric = LooseText(value)
			res.Contributions[idx].GroupKey = LooseText(normalizeLedgerGroupKey(res.Contributions[idx].GroupKey.String(), value))
		default:
			return fmt.Errorf("data result patch rejected: field %q is not a safe contribution patch field", parts[2])
		}
	case "entity_resolutions":
		if len(parts) != 3 || parts[2] != "status" {
			return fmt.Errorf("data result patch rejected: unsupported entity-resolution path %q", pointer)
		}
		idx, ok := parseNonNegativeIndex(parts[1], len(res.EntityResolutions))
		if !ok {
			return fmt.Errorf("data result patch rejected: entity-resolution index out of range in %q", pointer)
		}
		if strings.TrimSpace(res.EntityResolutions[idx].CanonicalID.String()) == "" &&
			strings.TrimSpace(res.EntityResolutions[idx].CanonicalLabel.String()) == "" &&
			strings.TrimSpace(res.EntityResolutions[idx].Reason.String()) == "" &&
			len(res.EntityResolutions[idx].Candidates) == 0 &&
			len(res.EntityResolutions[idx].EvidenceRefs) == 0 {
			return fmt.Errorf("data result patch rejected: entity-resolution status patch needs existing canonical value, reason, candidates, or evidence")
		}
		res.EntityResolutions[idx].Status = LooseText(value)
	case "reconcile":
		if res.Reconcile == nil {
			return fmt.Errorf("data result patch rejected: reconcile is missing")
		}
		if len(parts) == 2 && parts[1] == "status" {
			if strings.TrimSpace(res.Reconcile.Status.String()) != "" {
				return fmt.Errorf("data result patch rejected: reconcile status patch only fills a missing structural status")
			}
			if len(res.Reconcile.Differences) > 0 {
				return fmt.Errorf("data result patch rejected: reconcile status patch cannot hide existing differences")
			}
			res.Reconcile.Status = LooseText(value)
			return nil
		}
		if len(parts) != 4 || parts[1] != "groups" {
			return fmt.Errorf("data result patch rejected: unsupported reconcile path %q", pointer)
		}
		idx, ok := parseNonNegativeIndex(parts[2], len(res.Reconcile.Groups))
		if !ok {
			return fmt.Errorf("data result patch rejected: reconcile group index out of range in %q", pointer)
		}
		switch parts[3] {
		case "group_key":
			res.Reconcile.Groups[idx].GroupKey = LooseText(normalizeLedgerGroupKey(value, res.Reconcile.Groups[idx].Metric.String()))
		case "metric":
			res.Reconcile.Groups[idx].Metric = LooseText(value)
			res.Reconcile.Groups[idx].GroupKey = LooseText(normalizeLedgerGroupKey(res.Reconcile.Groups[idx].GroupKey.String(), value))
		default:
			return fmt.Errorf("data result patch rejected: field %q is not a safe reconcile patch field", parts[3])
		}
	default:
		return fmt.Errorf("data result patch rejected: path %q is outside the safe structural patch set", pointer)
	}
	return nil
}

func parseNonNegativeIndex(text string, length int) (int, bool) {
	if text == "" {
		return 0, false
	}
	var n int
	for _, ch := range text {
		if ch < '0' || ch > '9' {
			return 0, false
		}
		n = n*10 + int(ch-'0')
		if n >= length {
			return 0, false
		}
	}
	return n, n >= 0 && n < length
}

func newDataResultPatch(op, path string, value any, reason string) DataResultPatch {
	raw, err := json.Marshal(value)
	if err != nil {
		raw = nil
	}
	return DataResultPatch{
		Target: "result",
		Op:     op,
		Path:   path,
		Value:  raw,
		Reason: reason,
	}
}

func normalizeLedgerGroupKey(groupKey, metric string) string {
	groupKey = strings.TrimSpace(groupKey)
	metric = strings.TrimSpace(metric)
	if groupKey == "" || metric == "" {
		return groupKey
	}
	suffix := "/" + metric
	for strings.HasSuffix(groupKey, suffix) {
		groupKey = strings.TrimSpace(strings.TrimSuffix(groupKey, suffix))
	}
	return groupKey
}

func normalizeContributionOperation(op string) (string, bool) {
	op = strings.ToLower(strings.TrimSpace(op))
	switch op {
	case "":
		return "", true
	case "include", "included":
		return "include", true
	case "add", "sum", "plus", "positive", "increment":
		return "add", true
	case "count":
		return "count", true
	case "subtract", "sub", "minus", "deduct", "negative":
		return "subtract", true
	case "set", "assign":
		return "set", true
	case "rank", "order":
		return "rank", true
	default:
		return op, false
	}
}

func parseDecimalRat(value string) (*big.Rat, error) {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, ",", "")
	if value == "" {
		return nil, errors.New("empty decimal")
	}
	r := new(big.Rat)
	if _, ok := r.SetString(value); ok {
		return r, nil
	}
	return nil, fmt.Errorf("invalid decimal %q", value)
}

func formatRat(r *big.Rat) string {
	if r == nil {
		return "0"
	}
	if r.IsInt() {
		return r.Num().String()
	}
	return strings.TrimRight(strings.TrimRight(r.FloatString(6), "0"), ".")
}

func validateScriptSafety(script string) error {
	lower := strings.ToLower(script)
	for _, denied := range []string{
		"__", "eval(", "exec(", "compile(", "globals(", "locals(", "vars(",
	} {
		if strings.Contains(lower, denied) {
			return fmt.Errorf("data task script uses unsupported unsafe construct %q", denied)
		}
	}
	reservedHelpers := []string{
		"csv_rows", "tsv_rows", "json_load", "jsonl_rows", "read_text", "parse_money",
		"add_decision", "add_rule_coverage", "add_contribution", "add_resolution", "add_entity_resolution",
		"emit_result", "emit", "open",
	}
	for _, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		for _, name := range reservedHelpers {
			if strings.HasPrefix(trimmed, "def "+name+"(") ||
				strings.HasPrefix(trimmed, "async def "+name+"(") ||
				strings.HasPrefix(trimmed, name+"=") ||
				strings.HasPrefix(trimmed, name+" =") {
				return fmt.Errorf("data task script redefines reserved helper %q; use the provided helper instead", name)
			}
		}
	}
	return nil
}

func hasMeaningfulRowDecision(rows []RowDecision) bool {
	for _, row := range rows {
		if strings.TrimSpace(row.RowID) != "" ||
			strings.TrimSpace(row.Source) != "" ||
			strings.TrimSpace(row.SourceLocator) != "" ||
			strings.TrimSpace(row.Decision) != "" ||
			strings.TrimSpace(row.Reason) != "" ||
			strings.TrimSpace(row.Value) != "" ||
			strings.TrimSpace(row.Contribution) != "" ||
			len(row.NormalizedFields) > 0 ||
			len(row.EvidenceRef) > 0 {
			return true
		}
	}
	return false
}

func copyInputs(root, workDir string, paths []string, maxFile, maxTotal int64) ([]string, error) {
	seen := map[string]bool{}
	var rels []string
	var total int64
	for _, raw := range paths {
		rel, abs, err := resolveInputPath(root, raw)
		if err != nil {
			return nil, err
		}
		if seen[rel] {
			continue
		}
		seen[rel] = true
		info, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("stat input %s: %w", raw, err)
		}
		if info.IsDir() {
			err = filepath.WalkDir(abs, func(path string, d os.DirEntry, walkErr error) error {
				if walkErr != nil || d.IsDir() {
					return nil
				}
				subRel, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				subRel = filepath.ToSlash(subRel)
				if seen[subRel] {
					return nil
				}
				if !isRunnerInputKind(dataKindForPath(path)) {
					return nil
				}
				seen[subRel] = true
				size, err := copyOneInput(path, filepath.Join(workDir, subRel), maxFile)
				if err != nil {
					return err
				}
				total += size
				if total > maxTotal {
					return fmt.Errorf("data task input total exceeds %d bytes", maxTotal)
				}
				rels = append(rels, subRel)
				return nil
			})
			if err != nil {
				return nil, err
			}
			continue
		}
		kind := dataKindForPath(abs)
		if kind == "" {
			return nil, fmt.Errorf("input %s is not a supported data file", raw)
		}
		if !isRunnerInputKind(kind) {
			return nil, fmt.Errorf("input %s is %s; extract text evidence first before deterministic data processing", raw, kind)
		}
		size, err := copyOneInput(abs, filepath.Join(workDir, rel), maxFile)
		if err != nil {
			return nil, err
		}
		total += size
		if total > maxTotal {
			return nil, fmt.Errorf("data task input total exceeds %d bytes", maxTotal)
		}
		rels = append(rels, filepath.ToSlash(rel))
	}
	if len(rels) == 0 {
		return nil, errors.New("no supported data input files were copied")
	}
	sort.Strings(rels)
	return rels, nil
}

func resolveInputPath(root, raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", errors.New("empty input path")
	}
	abs := raw
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, raw)
	}
	abs, err := filepath.Abs(abs)
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", "", err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("input path %s is outside data root %s", raw, root)
	}
	return filepath.ToSlash(rel), abs, nil
}

func validateCoverageInputsDeclared(contract CoverageContract, copied []string) error {
	copied = normalizeMaterialPaths(copied)
	for _, material := range contract.RequiredMaterials {
		mode := normalizeCoverageMaterialUseMode(material.UsageMode)
		switch mode {
		case MaterialUseScriptConsumed:
			req := normalizeMaterialPath(material.Path)
			if req != "" && !materialPathCovered(req, copied) {
				return fmt.Errorf("data coverage incomplete: required material %q is not declared in input_paths", req)
			}
		case MaterialUseTextEvidenceConsumed:
			source := normalizeMaterialPath(material.Path)
			evidence := normalizeMaterialPath(material.TextEvidencePath)
			if source == "" {
				return errors.New("data coverage incomplete: required text-evidence material is missing source path")
			}
			if evidence == "" {
				return fmt.Errorf("data coverage incomplete: required material %q uses text_evidence_consumed but text_evidence_path is empty", source)
			}
			if !materialPathCovered(evidence, copied) {
				return fmt.Errorf("data coverage incomplete: text evidence %q for required material %q is not declared in input_paths", evidence, source)
			}
		case MaterialUsePlannerDistilled:
			source := normalizeMaterialPath(material.Path)
			if source == "" && strings.TrimSpace(material.ID) == "" {
				return errors.New("data coverage incomplete: planner_distilled required material needs path or id")
			}
			if len(cleanMaterialNotes(material.DistilledNotes)) == 0 {
				return fmt.Errorf("data coverage incomplete: required material %q uses planner_distilled but distilled_notes is empty", materialDisplayName(material))
			}
		case MaterialUseReferenceOnly:
			if material.Required {
				return fmt.Errorf("data coverage incomplete: required material %q cannot use reference_only; put it in optional_materials or choose a verifiable usage_mode", materialDisplayName(material))
			}
		}
	}
	return nil
}

func validateCoverageConsumed(contract CoverageContract, consumed []string) error {
	consumed = normalizeMaterialPaths(consumed)
	for _, material := range contract.RequiredMaterials {
		mode := normalizeCoverageMaterialUseMode(material.UsageMode)
		switch mode {
		case MaterialUseScriptConsumed:
			req := normalizeMaterialPath(material.Path)
			if req != "" && !materialPathCovered(req, consumed) {
				return fmt.Errorf("data coverage incomplete: required material %q was not consumed by the script", req)
			}
		case MaterialUseTextEvidenceConsumed:
			source := normalizeMaterialPath(material.Path)
			evidence := normalizeMaterialPath(material.TextEvidencePath)
			if evidence == "" {
				return fmt.Errorf("data coverage incomplete: required material %q uses text_evidence_consumed but text_evidence_path is empty", source)
			}
			if !materialPathCovered(evidence, consumed) {
				return fmt.Errorf("data coverage incomplete: text evidence %q for required material %q was not consumed by the script", evidence, source)
			}
		}
	}
	return nil
}

func cleanMaterialNotes(notes []string) []string {
	out := make([]string, 0, len(notes))
	for _, note := range notes {
		note = strings.TrimSpace(note)
		if note == "" {
			continue
		}
		out = append(out, note)
	}
	return out
}

func materialDisplayName(material CoverageMaterial) string {
	if path := normalizeMaterialPath(material.Path); path != "" {
		return path
	}
	if id := strings.TrimSpace(material.ID); id != "" {
		return id
	}
	if purpose := strings.TrimSpace(material.Purpose); purpose != "" {
		return purpose
	}
	return "<unnamed material>"
}

func materialPathCovered(required string, paths []string) bool {
	required = normalizeMaterialPath(required)
	if required == "" {
		return true
	}
	for _, path := range paths {
		path = normalizeMaterialPath(path)
		if path == required || strings.HasPrefix(path, required+"/") || strings.HasPrefix(required, path+"/") {
			return true
		}
	}
	return false
}

func normalizeMaterialPaths(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, path := range in {
		path = normalizeMaterialPath(path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func normalizeMaterialPath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	path = strings.TrimPrefix(path, "./")
	path = filepath.Clean(path)
	path = strings.ReplaceAll(path, "\\", "/")
	if path == "." {
		return ""
	}
	return path
}

func copyOneInput(src, dst string, maxFile int64) (int64, error) {
	info, err := os.Stat(src)
	if err != nil {
		return 0, err
	}
	if info.Size() > maxFile {
		return 0, fmt.Errorf("input %s exceeds %d bytes", src, maxFile)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return 0, err
	}
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(out, in)
	closeErr := out.Close()
	if err != nil {
		return 0, err
	}
	if closeErr != nil {
		return 0, closeErr
	}
	if err := os.Chmod(dst, 0400); err != nil {
		return 0, err
	}
	return n, nil
}

func renderPythonHelper(script string, relPaths []string) string {
	scriptJSON, _ := json.Marshal(script)
	pathsJSON, _ := json.Marshal(relPaths)
	return fmt.Sprintf(`# -*- coding: utf-8 -*-
import csv, json, decimal, re, os, math, statistics, collections, datetime, itertools, functools, operator
ALLOWED = set(%s)
RESULT = None
BASE_DIR = os.getcwd()
CONSUMED = set()
DECISIONS = []
RULE_COVERAGE = []
CONTRIBUTIONS = []
ENTITY_RESOLUTIONS = []
PRINT_BYTES = 0
PRINT_BYTE_LIMIT = 20000
PRINT_CALL_LIMIT = 4096
ALLOWED_IMPORTS = {
    "csv", "json", "decimal", "re", "math", "statistics", "collections",
    "datetime", "itertools", "functools", "operator", "string", "textwrap",
    "base64", "binascii", "hashlib", "unicodedata", "fractions", "calendar",
}

def _safe_path(path):
    path = str(path).replace("\\", "/").strip()
    norm = os.path.normpath(path).replace("\\", "/")
    if norm.startswith("../") or norm == ".." or os.path.isabs(norm):
        raise ValueError("path outside data task workspace: " + path)
    if norm not in ALLOWED:
        raise ValueError("path was not declared as an input: " + path)
    return norm

def _mark_consumed(path):
    norm = _safe_path(path)
    CONSUMED.add(norm)
    return norm

def _safe_open(path, mode="r", buffering=-1, encoding=None, errors=None, newline=None):
    mode = str(mode or "r")
    if any(ch in mode for ch in "wax+") or not mode.startswith("r"):
        raise ValueError("data task open is read-only: " + mode)
    norm = _mark_consumed(path)
    full = os.path.abspath(os.path.join(BASE_DIR, norm))
    base = os.path.abspath(BASE_DIR)
    if full != base and not full.startswith(base + os.sep):
        raise ValueError("path outside data task workspace: " + str(path))
    if "b" in mode:
        return open(full, mode, buffering=buffering)
    return open(full, mode, buffering=buffering, encoding=encoding or "utf-8", errors=errors or "replace", newline=newline)

def _safe_import(name, globals=None, locals=None, fromlist=(), level=0):
    root = str(name).split(".", 1)[0]
    if level != 0 or root not in ALLOWED_IMPORTS:
        raise ImportError("data task import is blocked: " + str(name))
    return __import__(name, globals, locals, fromlist, level)

def _safe_print(*args, sep=" ", end="\n", file=None, flush=False):
    global PRINT_BYTES
    if file is not None:
        raise ValueError("data task print does not support custom file targets")
    text = sep.join(str(x) for x in args) + str(end)
    data = text.encode("utf-8", errors="replace")
    if len(data) > PRINT_CALL_LIMIT:
        data = data[:PRINT_CALL_LIMIT] + b"\n[data task print call truncated]\n"
    remaining = PRINT_BYTE_LIMIT - PRINT_BYTES
    if remaining <= 0:
        return
    if len(data) > remaining:
        data = data[:remaining] + b"\n[data task print output truncated]\n"
    PRINT_BYTES += len(data)
    print(data.decode("utf-8", errors="replace"), end="", flush=flush)

def read_text(path, encoding="utf-8"):
    norm = _mark_consumed(path)
    with open(norm, "r", encoding=encoding, errors="replace", newline="") as f:
        return f.read()

def csv_rows(path, encoding="utf-8"):
    norm = _mark_consumed(path)
    with open(norm, "r", encoding=encoding, errors="replace", newline="") as f:
        return list(csv.DictReader(f))

def tsv_rows(path, encoding="utf-8"):
    norm = _mark_consumed(path)
    with open(norm, "r", encoding=encoding, errors="replace", newline="") as f:
        return list(csv.DictReader(f, delimiter="\t"))

def json_load(path, encoding="utf-8"):
    norm = _mark_consumed(path)
    with open(norm, "r", encoding=encoding, errors="replace") as f:
        return json.load(f)

def jsonl_rows(path, encoding="utf-8"):
    rows = []
    norm = _mark_consumed(path)
    with open(norm, "r", encoding=encoding, errors="replace") as f:
        for line in f:
            line = line.strip()
            if line:
                rows.append(json.loads(line))
    return rows

def parse_money(value):
    text = str(value).strip().replace(",", "")
    return decimal.Decimal(text or "0")

def _pop_first(mapping, names, default=""):
    for name in names:
        if name in mapping:
            value = mapping.pop(name)
            if value not in (None, ""):
                return value
    return default

def _listify(value):
    if value is None or value == "":
        return []
    if isinstance(value, (list, tuple, set)):
        return [str(v) for v in value if v not in (None, "")]
    return [str(value)]

def add_decision(item_id="", source="", source_locator="", decision="", reason="", value="", contribution="", normalized_fields=None, evidence_refs=None, rule_refs=None, **extra):
    item_id = item_id or _pop_first(extra, ["row_id", "record_id", "id", "item"])
    source_locator = source_locator or _pop_first(extra, ["locator", "location", "span", "cell", "row"])
    decision = decision or _pop_first(extra, ["status", "outcome", "action"])
    if not rule_refs:
        rule_refs = _pop_first(extra, ["rule_refs", "rule_ref", "rule_id", "rule"])
    normalized = dict(normalized_fields or {})
    for key, value_extra in list(extra.items()):
        if value_extra is None:
            continue
        normalized[str(key)] = str(value_extra)
    rec = {
        "row_id": str(item_id or ""),
        "source": str(source or ""),
        "source_locator": str(source_locator or ""),
        "decision": str(decision or ""),
        "reason": str(reason or ""),
        "value": str(value or ""),
        "contribution": str(contribution or ""),
        "normalized_fields": normalized,
        "evidence_refs": list(evidence_refs or []),
        "rule_refs": _listify(rule_refs),
    }
    DECISIONS.append(rec)
    return rec

def add_rule_coverage(rule_id="", rule_text="", status="", evidence_refs=None, notes="", **extra):
    rule_id = rule_id or _pop_first(extra, ["id", "rule", "rule_ref"])
    rule_text = rule_text or _pop_first(extra, ["text", "description", "condition"])
    status = status or _pop_first(extra, ["outcome", "decision"], "applied")
    if not notes:
        notes = _pop_first(extra, ["reason", "summary", "details"])
    rec = {
        "rule_id": str(rule_id or ""),
        "rule_text": str(rule_text or ""),
        "status": str(status or ""),
        "evidence_refs": list(evidence_refs or []),
        "notes": str(notes or ""),
    }
    rec.update(extra)
    RULE_COVERAGE.append(rec)
    return rec

def _candidate_obj(candidate):
    if isinstance(candidate, dict):
        return candidate
    return {"id": str(candidate)}

def add_resolution(item_id="", source_value="", canonical_id="", canonical_label="", status="", candidates=None, evidence_refs=None, rule_refs=None, reason="", **extra):
    item_id = item_id or _pop_first(extra, ["row_id", "record_id", "id", "item"])
    source_value = source_value or _pop_first(extra, ["source", "raw", "raw_value", "value"])
    canonical_id = canonical_id or _pop_first(extra, ["canonical", "normalized_id", "target_id"])
    canonical_label = canonical_label or _pop_first(extra, ["label", "normalized_label", "target_label"])
    if not rule_refs:
        rule_refs = _pop_first(extra, ["rule_refs", "rule_ref", "rule_id", "rule"])
    if not reason:
        reason = _pop_first(extra, ["notes", "summary", "details"])
    if not status:
        status = "resolved" if canonical_id or canonical_label else "unresolved"
    rec = {
        "item_id": str(item_id or ""),
        "source_value": str(source_value or ""),
        "canonical_id": str(canonical_id or ""),
        "canonical_label": str(canonical_label or ""),
        "status": str(status or ""),
        "candidates": [_candidate_obj(c) for c in list(candidates or [])],
        "evidence_refs": list(evidence_refs or []),
        "rule_refs": _listify(rule_refs),
        "reason": str(reason or ""),
    }
    rec.update(extra)
    ENTITY_RESOLUTIONS.append(rec)
    return rec

def add_entity_resolution(*args, **kwargs):
    return add_resolution(*args, **kwargs)

def add_contribution(item_id="", source="", source_locator="", group_key="", metric="", value="", operation="add", reason="", evidence_refs=None, rule_refs=None, **extra):
    item_id = item_id or _pop_first(extra, ["row_id", "record_id", "id", "item"])
    source_locator = source_locator or _pop_first(extra, ["locator", "location", "span", "cell", "row"])
    group_key = group_key or _pop_first(extra, ["group", "dimensions", "dimension_key", "bucket"])
    metric = metric or _pop_first(extra, ["measure", "field", "metric_name"])
    operation = operation or _pop_first(extra, ["op", "aggregation"], "add")
    if not rule_refs:
        rule_refs = _pop_first(extra, ["rule_refs", "rule_ref", "rule_id", "rule"])
    if not reason:
        reason = _pop_first(extra, ["notes", "summary", "details"])
    rec = {
        "item_id": str(item_id or ""),
        "source": str(source or ""),
        "source_locator": str(source_locator or ""),
        "group_key": str(group_key or ""),
        "metric": str(metric or ""),
        "value": str(value or ""),
        "operation": str(operation or "add"),
        "reason": str(reason or ""),
        "evidence_refs": list(evidence_refs or []),
        "rule_refs": _listify(rule_refs),
    }
    rec.update(extra)
    CONTRIBUTIONS.append(rec)
    return rec

def _merge_helper_ledgers(obj):
    if isinstance(obj, dict):
        if "rows" not in obj and DECISIONS:
            obj["rows"] = list(DECISIONS)
        if "rule_coverage" not in obj and RULE_COVERAGE:
            obj["rule_coverage"] = list(RULE_COVERAGE)
        if "contributions" not in obj and CONTRIBUTIONS:
            obj["contributions"] = list(CONTRIBUTIONS)
        if "entity_resolutions" not in obj and ENTITY_RESOLUTIONS:
            obj["entity_resolutions"] = list(ENTITY_RESOLUTIONS)
    return obj

def emit(obj):
    global RESULT
    RESULT = _merge_helper_ledgers(obj)

def emit_result(answer, output_contract=None, audit_summary="", rows=None, rule_coverage=None, contributions=None, entity_resolutions=None, reconcile=None, metrics=None, **extra):
    obj = {
        "answer": str(answer),
        "output_contract": output_contract or {},
        "audit_summary": str(audit_summary or ""),
        "rows": list(rows if rows is not None else DECISIONS),
        "rule_coverage": list(rule_coverage if rule_coverage is not None else RULE_COVERAGE),
        "contributions": list(contributions if contributions is not None else CONTRIBUTIONS),
        "entity_resolutions": list(entity_resolutions if entity_resolutions is not None else ENTITY_RESOLUTIONS),
        "metrics": list(metrics or []),
    }
    if reconcile is not None:
        obj["reconcile"] = reconcile
    obj.update(extra)
    emit(obj)
    return obj

safe_builtins = {
    "len": len, "sum": sum, "min": min, "max": max, "sorted": sorted,
    "range": range, "enumerate": enumerate, "int": int, "float": float,
    "str": str, "repr": repr, "format": format, "round": round, "abs": abs,
    "bool": bool, "list": list, "dict": dict, "set": set, "tuple": tuple,
    "frozenset": frozenset, "bytes": bytes, "bytearray": bytearray,
    "any": any, "all": all, "zip": zip, "map": map, "filter": filter,
    "reversed": reversed, "slice": slice, "iter": iter, "next": next,
    "ord": ord, "chr": chr, "divmod": divmod, "pow": pow,
    "isinstance": isinstance, "issubclass": issubclass,
    "ValueError": ValueError,
    "TypeError": TypeError, "KeyError": KeyError, "Exception": Exception,
    "IndexError": IndexError, "AttributeError": AttributeError,
    "ArithmeticError": ArithmeticError, "ZeroDivisionError": ZeroDivisionError,
    "AssertionError": AssertionError, "StopIteration": StopIteration,
    "open": _safe_open, "print": _safe_print, "__import__": _safe_import,
}
env = {
    "__builtins__": safe_builtins,
    "csv_rows": csv_rows, "tsv_rows": tsv_rows, "json_load": json_load,
    "jsonl_rows": jsonl_rows, "read_text": read_text,
    "parse_money": parse_money, "Decimal": decimal.Decimal,
    "add_decision": add_decision, "add_rule_coverage": add_rule_coverage,
    "add_contribution": add_contribution, "add_resolution": add_resolution,
    "add_entity_resolution": add_entity_resolution,
    "emit_result": emit_result,
    "csv": csv, "json": json, "decimal": decimal, "re": re,
    "math": math, "statistics": statistics, "collections": collections,
    "datetime": datetime, "itertools": itertools, "functools": functools,
    "operator": operator, "emit": emit,
    "true": True, "false": False, "null": None,
}
code = %s
exec(code, env, env)
if RESULT is None:
    RESULT = env.get("result")
if RESULT is None:
    raise ValueError("data task script did not call emit(obj) or set result")
if isinstance(RESULT, dict):
    RESULT.setdefault("consumed_paths", sorted(CONSUMED))
print(%q + json.dumps(RESULT, ensure_ascii=False, default=str))
`, string(pathsJSON), string(scriptJSON), resultMarker)
}

func parseRunnerResult(out []byte) (Result, error) {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, resultMarker) {
			continue
		}
		var res Result
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, resultMarker)), &res); err != nil {
			return Result{}, fmt.Errorf("parse data task result: %w", err)
		}
		res.Answer = strings.TrimSpace(res.Answer)
		return res, nil
	}
	return Result{}, fmt.Errorf("data task script did not emit a structured result; output=%s", strings.TrimSpace(string(out)))
}

func ValidateAnswer(answer string, contract OutputContract) error {
	contract = contract.Normalize()
	if strings.TrimSpace(answer) == "" {
		return errors.New("data task answer is empty")
	}
	if !contract.ExplanationAllowed {
		if strings.Contains(answer, "\n\n") && contract.Format != OutputMarkdown && contract.Format != OutputMarkdownTable {
			return fmt.Errorf("output contract %s does not allow explanatory paragraphs", contract.Format)
		}
	}
	switch contract.Format {
	case OutputPlainSingleLine, OutputCSVLine:
		if strings.Contains(answer, "\n") {
			return fmt.Errorf("output contract %s requires a single line", contract.Format)
		}
	case OutputJSONOnly:
		var v any
		if err := json.Unmarshal([]byte(answer), &v); err != nil {
			return fmt.Errorf("output contract json_only requires valid JSON: %w", err)
		}
	}
	return nil
}

func normalizeAnswerForContract(answer string, contract OutputContract, warnings []string) (string, []string) {
	contract = contract.Normalize()
	trimmed := strings.TrimSpace(answer)
	switch contract.Format {
	case OutputPlainSingleLine, OutputCSVLine:
		if strings.Contains(trimmed, "\n") {
			trimmed = strings.Join(strings.Fields(trimmed), " ")
			warnings = append(warnings, fmt.Sprintf("normalized multi-line answer to satisfy %s", contract.Format))
		}
	case OutputJSONOnly:
		if strings.TrimSpace(trimmed) != "" && !json.Valid([]byte(trimmed)) {
			if extracted, ok := extractFirstJSONObjectOrArray(trimmed); ok {
				trimmed = extracted
				warnings = append(warnings, "extracted first valid JSON object/array from answer text")
			}
		}
	}
	return trimmed, warnings
}

func extractFirstJSONObjectOrArray(s string) (string, bool) {
	for i, r := range s {
		var close rune
		switch r {
		case '{':
			close = '}'
		case '[':
			close = ']'
		default:
			continue
		}
		if out, ok := balancedJSONCandidate(s[i:], r, close); ok && json.Valid([]byte(out)) {
			return out, true
		}
	}
	return "", false
}

func balancedJSONCandidate(s string, open, close rune) (string, bool) {
	depth := 0
	inString := false
	escaped := false
	for idx, r := range s {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch r {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			inString = true
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return s[:idx+len(string(r))], true
			}
		}
	}
	return "", false
}
