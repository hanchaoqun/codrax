package dataquery

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ActionRunner executes typed, read-only data actions. It is deliberately
// narrower than Runner: action nodes produce reusable artifacts, while
// custom_transform is the explicit fallback for bounded Python transforms.
type ActionRunner struct {
	RepoRoot      string
	TempRoot      string
	Timeout       int64
	MaxFileBytes  int64
	MaxTotalBytes int64
}

func (r ActionRunner) Run(ctx context.Context, plan TaskPlan) (Result, error) {
	if len(plan.Actions) == 0 {
		return Result{}, errors.New("data action plan has no actions")
	}
	var artifacts []DataArtifact
	var consumed []string
	var summaries []string
	var lastResult *Result
	for i, action := range plan.Actions {
		action.Kind = normalizeDataActionKind(action.Kind)
		if strings.TrimSpace(action.ID) == "" {
			action.ID = fmt.Sprintf("action_%d", i+1)
		}
		switch action.Kind {
		case DataActionMaterialInventory:
			artifact, err := r.runMaterialInventory(action)
			if err != nil {
				return Result{}, err
			}
			artifacts = append(artifacts, artifact)
			summaries = append(summaries, artifact.Summary)
		case DataActionInspectMaterial:
			artifact, err := r.runInspectMaterial(action)
			if err != nil {
				return Result{}, err
			}
			artifacts = append(artifacts, artifact)
			consumed = append(consumed, artifact.SourcePaths...)
			summaries = append(summaries, artifact.Summary)
		case DataActionExtractRecords:
			artifact, err := r.runExtractRecords(action)
			if err != nil {
				return Result{}, err
			}
			artifacts = append(artifacts, artifact)
			consumed = append(consumed, artifact.SourcePaths...)
			summaries = append(summaries, artifact.Summary)
		case DataActionCustomTransform:
			result, err := r.runCustomTransform(ctx, plan, action)
			if err != nil {
				return Result{}, err
			}
			lastResult = &result
			artifacts = append(artifacts, result.Artifacts...)
			consumed = append(consumed, result.ConsumedPaths...)
			if strings.TrimSpace(result.AuditSummary) != "" {
				summaries = append(summaries, result.AuditSummary)
			}
		default:
			return Result{}, fmt.Errorf("unsupported data action kind %q", action.Kind)
		}
	}
	if lastResult != nil {
		out := *lastResult
		out.Artifacts = append(out.Artifacts, artifacts...)
		out.ConsumedPaths = normalizeMaterialPaths(append(out.ConsumedPaths, consumed...))
		if strings.TrimSpace(out.AuditSummary) == "" {
			out.AuditSummary = strings.Join(cleanArtifactSummaries(summaries), "; ")
		}
		return validateRunnerResult(plan, out)
	}
	answer := renderArtifactsAnswer(artifacts, plan.OutputContract)
	out := Result{
		Answer:         answer,
		OutputContract: plan.OutputContract.Normalize(),
		AuditSummary:   strings.Join(cleanArtifactSummaries(summaries), "; "),
		Artifacts:      artifacts,
		ConsumedPaths:  normalizeMaterialPaths(consumed),
	}
	if out.OutputContract.Format == "" {
		out.OutputContract = OutputContract{Format: OutputMarkdown, ExplanationAllowed: true}.Normalize()
	}
	return validateRunnerResult(plan, out)
}

func normalizeDataActionKind(kind DataActionKind) DataActionKind {
	switch DataActionKind(strings.ToLower(strings.TrimSpace(string(kind)))) {
	case DataActionMaterialInventory:
		return DataActionMaterialInventory
	case DataActionInspectMaterial:
		return DataActionInspectMaterial
	case DataActionExtractRecords:
		return DataActionExtractRecords
	case DataActionCustomTransform, "":
		return DataActionCustomTransform
	default:
		return kind
	}
}

func (r ActionRunner) runMaterialInventory(action DataAction) (DataArtifact, error) {
	root := firstNonEmptyString(strings.TrimSpace(r.RepoRoot), ".")
	limit := 240
	if raw := strings.TrimSpace(action.Params["limit"]); raw != "" {
		var parsed int
		if _, err := fmt.Sscanf(raw, "%d", &parsed); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}
	files, err := DiscoverCandidateFiles(root, limit)
	if err != nil {
		return DataArtifact{}, err
	}
	children := make([]DataArtifact, 0, len(files))
	for _, f := range files {
		children = append(children, candidateArtifact(f))
	}
	id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "material_inventory")
	return DataArtifact{
		ID:       id,
		Kind:     string(DataActionMaterialInventory),
		Summary:  fmt.Sprintf("discovered %d candidate material(s)", len(files)),
		Fields:   map[string]string{"count": fmt.Sprintf("%d", len(files))},
		Children: children,
	}, nil
}

func (r ActionRunner) runInspectMaterial(action DataAction) (DataArtifact, error) {
	paths := cleanStringList(action.InputPaths)
	if len(paths) == 0 {
		return DataArtifact{}, errors.New("inspect_material action has no input_paths")
	}
	root := firstNonEmptyString(strings.TrimSpace(r.RepoRoot), ".")
	all, err := DiscoverCandidateFiles(root, 1000)
	if err != nil {
		return DataArtifact{}, err
	}
	byPath := map[string]CandidateFile{}
	for _, f := range all {
		byPath[normalizeMaterialPath(f.Path)] = f
	}
	children := make([]DataArtifact, 0, len(paths))
	for _, p := range paths {
		key := normalizeMaterialPath(p)
		if f, ok := byPath[key]; ok {
			children = append(children, candidateArtifact(f))
			continue
		}
		children = append(children, DataArtifact{
			ID:          key,
			Kind:        "unknown",
			SourcePaths: []string{key},
			Summary:     "material was requested but not found in candidate inventory",
		})
	}
	id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "material_inspection")
	return DataArtifact{
		ID:          id,
		Kind:        string(DataActionInspectMaterial),
		SourcePaths: normalizeMaterialPaths(paths),
		Summary:     fmt.Sprintf("inspected %d material(s)", len(paths)),
		Fields:      map[string]string{"count": fmt.Sprintf("%d", len(paths))},
		Children:    children,
	}, nil
}

func (r ActionRunner) runExtractRecords(action DataAction) (DataArtifact, error) {
	paths := cleanStringList(action.InputPaths)
	if len(paths) == 0 {
		return DataArtifact{}, errors.New("extract_records action has no input_paths")
	}
	limit := actionIntParam(action, "limit", 20, 1, 200)
	children := make([]DataArtifact, 0, len(paths))
	for _, p := range paths {
		child, err := r.extractRecordsFromPath(p, limit)
		if err != nil {
			return DataArtifact{}, err
		}
		children = append(children, child)
	}
	id := firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "record_extract")
	return DataArtifact{
		ID:          id,
		Kind:        string(DataActionExtractRecords),
		SourcePaths: normalizeMaterialPaths(paths),
		Summary:     fmt.Sprintf("extracted record samples from %d material(s)", len(paths)),
		Fields: map[string]string{
			"count": fmt.Sprintf("%d", len(paths)),
			"limit": fmt.Sprintf("%d", limit),
		},
		Children: children,
	}, nil
}

func (r ActionRunner) extractRecordsFromPath(path string, limit int) (DataArtifact, error) {
	abs, rel, err := r.resolveActionInputPath(path)
	if err != nil {
		return DataArtifact{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return DataArtifact{}, err
	}
	kind := dataKindForPath(abs)
	if kind == "" {
		kind = "text"
	}
	f := inspectCandidateFile(abs, CandidateFile{
		Path: rel,
		Size: info.Size(),
		Kind: kind,
	})
	artifact := candidateArtifact(f)
	artifact.ID = rel + "#records"
	artifact.Kind = string(DataActionExtractRecords) + "/" + kind
	artifact.Sample = nil
	switch kind {
	case "csv", "tsv":
		headers, samples, rowCount, err := extractDelimitedRecords(abs, f.Delimiter, limit)
		if err != nil {
			return DataArtifact{}, err
		}
		artifact.Headers = headers
		artifact.Sample = samples
		artifact.RowCount = rowCount
	case "json":
		samples, rowCount, err := extractJSONRecords(abs, limit)
		if err != nil {
			return DataArtifact{}, err
		}
		artifact.Sample = samples
		artifact.RowCount = rowCount
	case "jsonl":
		samples, rowCount, err := extractJSONLRecords(abs, limit)
		if err != nil {
			return DataArtifact{}, err
		}
		artifact.Sample = samples
		artifact.RowCount = rowCount
	default:
		samples, rowCount, err := extractTextRecords(abs, limit)
		if err != nil {
			return DataArtifact{}, err
		}
		artifact.Sample = samples
		artifact.RowCount = rowCount
	}
	if artifact.Fields == nil {
		artifact.Fields = map[string]string{}
	}
	artifact.Fields["sample_count"] = fmt.Sprintf("%d", len(artifact.Sample))
	artifact.Fields["limit"] = fmt.Sprintf("%d", limit)
	artifact.Summary = fmt.Sprintf("%s | extracted %d sample record(s) from %d total record(s)", rel, len(artifact.Sample), artifact.RowCount)
	return artifact, nil
}

func (r ActionRunner) resolveActionInputPath(path string) (abs string, rel string, err error) {
	path = normalizeMaterialPath(path)
	if path == "" {
		return "", "", errors.New("empty action input path")
	}
	root := firstNonEmptyString(strings.TrimSpace(r.RepoRoot), ".")
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	joined := path
	if !filepath.IsAbs(joined) {
		joined = filepath.Join(absRoot, filepath.FromSlash(path))
	}
	abs, err = filepath.Abs(joined)
	if err != nil {
		return "", "", err
	}
	relPath, err := filepath.Rel(absRoot, abs)
	if err != nil {
		return "", "", err
	}
	if relPath == "." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || relPath == ".." || filepath.IsAbs(relPath) {
		return "", "", fmt.Errorf("action input path escapes data workspace: %s", path)
	}
	return abs, filepath.ToSlash(relPath), nil
}

func (r ActionRunner) runCustomTransform(ctx context.Context, plan TaskPlan, action DataAction) (Result, error) {
	script := strings.TrimSpace(action.Script)
	if script == "" {
		script = strings.TrimSpace(plan.Script)
	}
	if script == "" {
		return Result{}, errors.New("custom_transform action has empty script")
	}
	inputs := cleanStringList(action.InputPaths)
	if len(inputs) == 0 {
		inputs = cleanStringList(plan.InputPaths)
	}
	subPlan := plan
	subPlan.Actions = nil
	subPlan.Script = script
	subPlan.InputPaths = inputs
	runner := Runner{
		RepoRoot:      r.RepoRoot,
		TempRoot:      r.TempRoot,
		MaxFileBytes:  r.MaxFileBytes,
		MaxTotalBytes: r.MaxTotalBytes,
	}
	return runner.Run(ctx, subPlan)
}

func extractDelimitedRecords(path, delimiter string, limit int) ([]string, []string, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, 0, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	if delimiter == "\t" {
		reader.Comma = '\t'
	}
	headers, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil, 0, nil
		}
		return nil, nil, 0, err
	}
	headers = cleanStringSlice(headers)
	var samples []string
	rowCount := 0
	for {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return headers, samples, rowCount, err
		}
		rowCount++
		if len(samples) >= limit {
			continue
		}
		obj := map[string]string{}
		for i, value := range row {
			key := fmt.Sprintf("col_%d", i+1)
			if i < len(headers) && headers[i] != "" {
				key = headers[i]
			}
			obj[key] = strings.TrimSpace(value)
		}
		samples = append(samples, compactJSONLine(obj))
	}
	return headers, samples, rowCount, nil
}

func extractJSONRecords(path string, limit int) ([]string, int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, 0, err
	}
	var records []any
	switch v := value.(type) {
	case []any:
		records = v
	case map[string]any:
		for _, item := range v {
			if arr, ok := item.([]any); ok {
				records = arr
				break
			}
		}
		if records == nil {
			records = []any{v}
		}
	default:
		records = []any{v}
	}
	samples := make([]string, 0, minInt(limit, len(records)))
	for i, record := range records {
		if i >= limit {
			break
		}
		samples = append(samples, compactJSONLine(record))
	}
	return samples, len(records), nil
}

func extractJSONLRecords(path string, limit int) ([]string, int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	lines := strings.Split(string(raw), "\n")
	var samples []string
	rowCount := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rowCount++
		if len(samples) >= limit {
			continue
		}
		var obj any
		if err := json.Unmarshal([]byte(line), &obj); err == nil {
			samples = append(samples, compactJSONLine(obj))
		} else {
			samples = append(samples, line)
		}
	}
	return samples, rowCount, nil
}

func extractTextRecords(path string, limit int) ([]string, int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	lines := strings.Split(string(raw), "\n")
	var samples []string
	rowCount := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rowCount++
		if len(samples) < limit {
			samples = append(samples, line)
		}
	}
	return samples, rowCount, nil
}

func compactJSONLine(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(raw)
}

func actionIntParam(action DataAction, key string, fallback, minValue, maxValue int) int {
	value := fallback
	if raw := strings.TrimSpace(action.Params[key]); raw != "" {
		var parsed int
		if _, err := fmt.Sscanf(raw, "%d", &parsed); err == nil {
			value = parsed
		}
	}
	if value < minValue {
		value = minValue
	}
	if value > maxValue {
		value = maxValue
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func candidateArtifact(f CandidateFile) DataArtifact {
	fields := map[string]string{
		"kind":              f.Kind,
		"size":              fmt.Sprintf("%d", f.Size),
		"extraction_status": f.ExtractionStatus,
	}
	if f.Delimiter != "" {
		fields["delimiter"] = f.Delimiter
	}
	if f.InspectError != "" {
		fields["inspect_error"] = f.InspectError
	}
	if len(f.TextEvidencePaths) > 0 {
		fields["text_evidence_paths"] = strings.Join(f.TextEvidencePaths, ", ")
	}
	return DataArtifact{
		ID:          normalizeMaterialPath(f.Path),
		Kind:        f.Kind,
		SourcePaths: []string{normalizeMaterialPath(f.Path)},
		Summary:     candidateSummary(f),
		Fields:      fields,
		Headers:     append([]string(nil), f.Headers...),
		Sample:      candidateSample(f),
		RowCount:    f.Lines,
	}
}

func candidateSummary(f CandidateFile) string {
	parts := []string{f.Path, f.Kind}
	if f.Lines > 0 {
		parts = append(parts, fmt.Sprintf("%d line(s)", f.Lines))
	}
	if len(f.Headers) > 0 {
		parts = append(parts, "headers="+strings.Join(f.Headers, ","))
	}
	if f.ExtractionStatus != "" {
		parts = append(parts, "status="+f.ExtractionStatus)
	}
	return strings.Join(parts, " | ")
}

func candidateSample(f CandidateFile) []string {
	if len(f.Sample) > 0 {
		return append([]string(nil), f.Sample...)
	}
	if len(f.SampleRows) == 0 {
		return nil
	}
	out := make([]string, 0, len(f.SampleRows))
	for _, row := range f.SampleRows {
		out = append(out, strings.Join(row, ","))
	}
	return out
}

func renderArtifactsAnswer(artifacts []DataArtifact, contract OutputContract) string {
	if len(artifacts) == 0 {
		return ""
	}
	contract = contract.Normalize()
	switch contract.Format {
	case OutputJSONOnly:
		raw, _ := json.Marshal(artifacts)
		return string(raw)
	case OutputCSVLine:
		return fmt.Sprintf("artifacts,%d", len(artifacts))
	case OutputPlainSingleLine:
		return fmt.Sprintf("%d artifact(s)", len(artifacts))
	default:
		var b strings.Builder
		for _, artifact := range artifacts {
			fmt.Fprintf(&b, "- %s: %s\n", firstNonEmptyString(artifact.ID, artifact.Kind), artifact.Summary)
		}
		return strings.TrimSpace(b.String())
	}
}

func cleanArtifactSummaries(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func cleanStringList(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(filepath.ToSlash(s))
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
