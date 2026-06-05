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

type CoverageMaterial struct {
	ID       string `json:"id,omitempty"`
	Path     string `json:"path,omitempty"`
	Purpose  string `json:"purpose,omitempty"`
	Required bool   `json:"required,omitempty"`
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
	Rows              []RowDecision            `json:"rows,omitempty"`
	RuleCoverage      []RuleCoverageRecord     `json:"rule_coverage,omitempty"`
	Contributions     []ContributionRecord     `json:"contributions,omitempty"`
	EntityResolutions []EntityResolutionRecord `json:"entity_resolutions,omitempty"`
	Reconcile         *ReconcileReport         `json:"reconcile,omitempty"`
	Metrics           []Metric                 `json:"metrics,omitempty"`
	ConsumedPaths     []string                 `json:"consumed_paths,omitempty"`
	ContractWarnings  []string                 `json:"contract_warnings,omitempty"`
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
}

type EntityResolutionRecord struct {
	ItemID         LooseText         `json:"item_id,omitempty"`
	SourceValue    LooseText         `json:"source_value,omitempty"`
	CanonicalID    LooseText         `json:"canonical_id,omitempty"`
	CanonicalLabel LooseText         `json:"canonical_label,omitempty"`
	Status         LooseText         `json:"status,omitempty"`
	Candidates     []EntityCandidate `json:"candidates,omitempty"`
	EvidenceRefs   []string          `json:"evidence_refs,omitempty"`
	Reason         LooseText         `json:"reason,omitempty"`
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
		"evidence_refs": true,
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
}

func NormalizeEvaluationStatus(status string) EvaluationStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case string(EvalComplete):
		return EvalComplete
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
	res.ConsumedPaths = normalizeMaterialPaths(res.ConsumedPaths)
	if err := validateCoverageConsumed(plan.CoverageContract, res.ConsumedPaths); err != nil {
		return Result{}, err
	}
	if plan.CoverageContract.DecisionRecordsRequired && len(res.Rows) == 0 {
		return Result{}, errors.New("data coverage incomplete: coverage_contract.decision_records_required=true but result.rows is empty")
	}
	if plan.CoverageContract.DecisionRecordsRequired && !hasMeaningfulRowDecision(res.Rows) {
		return Result{}, errors.New("data coverage incomplete: coverage_contract.decision_records_required=true but result.rows contains no meaningful decision records")
	}
	if err := validateRequiredLedgers(plan.CoverageContract, res); err != nil {
		return Result{}, err
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
		return errors.New("data validation incomplete: coverage_contract.rule_coverage_required=true but result.rule_coverage is empty")
	}
	if contract.RuleCoverageRequired && !hasMeaningfulRuleCoverage(res.RuleCoverage) {
		return errors.New("data validation incomplete: result.rule_coverage contains no meaningful rule records")
	}
	if contract.ContributionLedgerRequired && len(res.Contributions) == 0 {
		return errors.New("data validation incomplete: coverage_contract.contribution_ledger_required=true but result.contributions is empty")
	}
	if contract.ContributionLedgerRequired && !hasMeaningfulContribution(res.Contributions) {
		return errors.New("data validation incomplete: result.contributions contains no meaningful contribution records")
	}
	if contract.EntityResolutionRequired && len(res.EntityResolutions) == 0 {
		return errors.New("data validation incomplete: coverage_contract.entity_resolution_required=true but result.entity_resolutions is empty")
	}
	if contract.EntityResolutionRequired && !hasMeaningfulEntityResolution(res.EntityResolutions) {
		return errors.New("data validation incomplete: result.entity_resolutions contains no meaningful resolution records")
	}
	if contract.ReconcileRequired {
		if res.Reconcile == nil {
			return errors.New("data validation incomplete: coverage_contract.reconcile_required=true but result.reconcile is empty")
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

func hasMeaningfulContribution(records []ContributionRecord) bool {
	for _, rec := range records {
		if strings.TrimSpace(rec.ItemID.String()) != "" ||
			strings.TrimSpace(rec.Source.String()) != "" ||
			strings.TrimSpace(rec.SourceLocator.String()) != "" ||
			strings.TrimSpace(rec.GroupKey.String()) != "" ||
			strings.TrimSpace(rec.Metric.String()) != "" ||
			strings.TrimSpace(rec.Value.String()) != "" ||
			strings.TrimSpace(rec.Operation.String()) != "" ||
			strings.TrimSpace(rec.Reason.String()) != "" ||
			len(rec.EvidenceRefs) > 0 {
			return true
		}
	}
	return false
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

func validateReconcileReport(report ReconcileReport, contributions []ContributionRecord, answer string) error {
	status := strings.ToLower(strings.TrimSpace(report.Status.String()))
	if status == "" {
		return errors.New("data validation incomplete: result.reconcile.status is empty")
	}
	if status != "pass" {
		if len(report.Differences) > 0 {
			return fmt.Errorf("data reconcile failed: status=%s differences=%s", status, strings.Join(report.Differences, "; "))
		}
		return fmt.Errorf("data reconcile failed: status=%s", status)
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
		return errors.New("data reconcile incomplete: result.reconcile.groups is empty while contributions are present")
	}
	if len(report.Groups) == 0 || len(contributions) == 0 {
		return nil
	}
	sums := sumContributionGroups(contributions)
	for _, group := range report.Groups {
		key := reconcileGroupKey(group.GroupKey.String(), group.Metric.String())
		if strings.TrimSpace(key) == "\x00" {
			continue
		}
		got, ok := sums[key]
		if !ok {
			return fmt.Errorf("data reconcile failed: group %q has no matching contribution records", displayReconcileGroupKey(group.GroupKey.String(), group.Metric.String()))
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
				return fmt.Errorf("data reconcile failed: group %q %s=%s but contributions sum to %s",
					displayReconcileGroupKey(group.GroupKey.String(), group.Metric.String()), candidate.label, value, formatRat(got))
			}
		}
	}
	return nil
}

func sumContributionGroups(contributions []ContributionRecord) map[string]*big.Rat {
	out := map[string]*big.Rat{}
	for _, rec := range contributions {
		op := strings.ToLower(strings.TrimSpace(rec.Operation.String()))
		switch op {
		case "", "include", "add", "count", "set", "rank":
		case "subtract":
		default:
			continue
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
	return strings.TrimSpace(groupKey) + "\x00" + strings.TrimSpace(metric)
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
	reservedHelpers := []string{"csv_rows", "tsv_rows", "json_load", "jsonl_rows", "read_text", "parse_money", "emit", "open"}
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
	for _, req := range contract.RequiredPaths() {
		if !materialPathCovered(req, copied) {
			return fmt.Errorf("data coverage incomplete: required material %q is not declared in input_paths", req)
		}
	}
	return nil
}

func validateCoverageConsumed(contract CoverageContract, consumed []string) error {
	consumed = normalizeMaterialPaths(consumed)
	for _, req := range contract.RequiredPaths() {
		if !materialPathCovered(req, consumed) {
			return fmt.Errorf("data coverage incomplete: required material %q was not consumed by the script", req)
		}
	}
	return nil
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
	return fmt.Sprintf(`import csv, json, decimal, re, os, math, statistics, collections, datetime, itertools, functools, operator
ALLOWED = set(%s)
RESULT = None
BASE_DIR = os.getcwd()
CONSUMED = set()
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

def emit(obj):
    global RESULT
    RESULT = obj

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
    "csv": csv, "json": json, "decimal": decimal, "re": re,
    "math": math, "statistics": statistics, "collections": collections,
    "datetime": datetime, "itertools": itertools, "functools": functools,
    "operator": operator, "emit": emit,
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
