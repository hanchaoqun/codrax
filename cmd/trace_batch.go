package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/hanchaoqun/codrax/internal/analysis/tracecluster"
	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

var (
	traceClusterInputDir string
	traceClusterOutput   string
	traceClusterBatchID  string
	traceClusterFormat   string

	traceBatchInputDir    string
	traceBatchPatterns    []string
	traceBatchOutputDir   string
	traceBatchID          string
	traceBatchConcurrency int
	traceBatchRecursive   bool
)

var traceClusterCmd = &cobra.Command{
	Use:   "cluster --input-dir <finding-directory>",
	Short: "Cluster validated TraceFindingV1 sidecars without rereading raw traces",
	Example: `  codrax trace cluster --input-dir .codrax/trace-batches/run-1/findings
  codrax trace cluster --input-dir findings --format json --output clusters.json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		inputs, failures, err := loadTraceFindingDirectory(traceClusterInputDir)
		if err != nil {
			return err
		}
		if len(inputs) == 0 && len(failures) == 0 {
			return fmt.Errorf("no *.trace-finding.json files found in %s", traceClusterInputDir)
		}
		batchID := strings.TrimSpace(traceClusterBatchID)
		if batchID == "" {
			batchID = "trace-cluster-" + time.Now().UTC().Format("20060102T150405Z")
		}
		set := tracecluster.Exact(batchID, inputs, failures)
		var body []byte
		switch strings.ToLower(strings.TrimSpace(traceClusterFormat)) {
		case "", "markdown", "md":
			body = []byte(renderTraceClusterMarkdown(set, flagLang))
		case "json":
			body, err = json.MarshalIndent(set, "", "  ")
			body = append(body, '\n')
		default:
			return fmt.Errorf("unknown --format %q (use markdown or json)", traceClusterFormat)
		}
		if err != nil {
			return err
		}
		if strings.TrimSpace(traceClusterOutput) == "" {
			_, err = cmd.OutOrStdout().Write(body)
			return err
		}
		path, err := writeNewArtifact(traceClusterOutput, body)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "cluster report written: %s\n", path)
		return nil
	},
}

var traceBatchCmd = &cobra.Command{
	Use:   "batch --input-dir <trace-directory>",
	Short: "Analyze trace files independently and build a root-cause cluster report",
	Long: `Each trace file is analyzed in an isolated Codrax child process. Raw
timelines are never merged. Every successful child writes a validated
TraceFindingV1 sidecar; the parent then performs deterministic exact
root-cause clustering and writes clusters.json plus report.md.`,
	Example: `  codrax trace batch --input-dir traces
  codrax trace batch --input-dir traces --pattern "*.systrace" --concurrency 2`,
	Args: cobra.NoArgs,
	RunE: runTraceBatch,
}

type traceBatchUnit struct {
	UnitID string
	Path   string
}

type traceBatchUnitResult struct {
	input   *tracecluster.Input
	failure *types.TraceBatchFailure
}

func runTraceBatch(cmd *cobra.Command, _ []string) error {
	if strings.TrimSpace(flagTraceFindingOut) != "" {
		return fmt.Errorf("--trace-finding-out is for one trace run and cannot be combined with trace batch")
	}
	units, err := discoverTraceBatchUnits(traceBatchInputDir, traceBatchPatterns, traceBatchRecursive)
	if err != nil {
		return err
	}
	if len(units) == 0 {
		return fmt.Errorf("no trace files matched in %s (patterns: %s)", traceBatchInputDir, strings.Join(traceBatchPatterns, ", "))
	}
	batchID := strings.TrimSpace(traceBatchID)
	if batchID == "" {
		batchID = "trace-batch-" + time.Now().UTC().Format("20060102T150405Z")
	}
	outputDir := strings.TrimSpace(traceBatchOutputDir)
	if outputDir == "" {
		outputDir = filepath.Join(".codrax", "trace-batches", batchID)
	}
	outputDir, err = filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("resolve trace batch output directory: %w", err)
	}
	if _, err := os.Stat(outputDir); err == nil {
		return fmt.Errorf("trace batch output directory already exists: %s", outputDir)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect trace batch output directory: %w", err)
	}
	for _, dir := range []string{outputDir, filepath.Join(outputDir, "findings"), filepath.Join(outputDir, "logs")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create trace batch output directory: %w", err)
		}
	}
	request := strings.TrimSpace(flagRequest)
	if request == "" {
		request = "请独立分析这一份 trace，找出最主要的卡顿根因和次要原因；结论要简短，并严格依据 trace 证据。"
	}
	concurrency := traceBatchConcurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > len(units) {
		concurrency = len(units)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "batch %s: %d trace(s), concurrency=%d\n", batchID, len(units), concurrency)

	jobs := make(chan traceBatchUnit)
	results := make(chan traceBatchUnitResult)
	var workers sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for unit := range jobs {
				results <- runTraceBatchUnit(cmd.Context(), unit, outputDir, request)
			}
		}()
	}
	go func() {
		for _, unit := range units {
			jobs <- unit
		}
		close(jobs)
		workers.Wait()
		close(results)
	}()

	inputs := make([]tracecluster.Input, 0, len(units))
	failures := make([]types.TraceBatchFailure, 0)
	completed := 0
	for result := range results {
		completed++
		if result.input != nil {
			inputs = append(inputs, *result.input)
			fmt.Fprintf(cmd.OutOrStdout(), "[%d/%d] %s: finding ready\n", completed, len(units), result.input.UnitID)
		} else if result.failure != nil {
			failures = append(failures, *result.failure)
			fmt.Fprintf(cmd.OutOrStdout(), "[%d/%d] %s: failed (%s)\n", completed, len(units), result.failure.UnitID, result.failure.Code)
		}
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].UnitID < inputs[j].UnitID })
	sort.Slice(failures, func(i, j int) bool { return failures[i].UnitID < failures[j].UnitID })
	set := tracecluster.Exact(batchID, inputs, failures)
	jsonBody, err := json.MarshalIndent(set, "", "  ")
	if err != nil {
		return fmt.Errorf("encode trace cluster set: %w", err)
	}
	jsonBody = append(jsonBody, '\n')
	jsonPath, err := writeNewArtifact(filepath.Join(outputDir, "clusters.json"), jsonBody)
	if err != nil {
		return err
	}
	reportPath, err := writeNewArtifact(filepath.Join(outputDir, "report.md"), []byte(renderTraceClusterMarkdown(set, flagLang)))
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\ncluster result: resolved=%d unresolved=%d failed=%d clusters=%d\n", set.ResolvedCount, set.UnresolvedCount, set.FailedCount, len(set.Clusters))
	fmt.Fprintf(cmd.OutOrStdout(), "report: %s\njson: %s\n", reportPath, jsonPath)
	if !set.Invariants.Valid {
		return fmt.Errorf("cluster invariant check failed: %s", strings.Join(set.Invariants.Errors, "; "))
	}
	return nil
}

func runTraceBatchUnit(ctx context.Context, unit traceBatchUnit, outputDir, request string) traceBatchUnitResult {
	findingPath := filepath.Join(outputDir, "findings", unit.UnitID+".trace-finding.json")
	logPath := filepath.Join(outputDir, "logs", unit.UnitID+".log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if err != nil {
		return failedTraceBatchUnit(unit.UnitID, "batch_log_create_failed", err)
	}
	defer logFile.Close()
	executable, err := os.Executable()
	if err != nil {
		return failedTraceBatchUnit(unit.UnitID, "child_executable_unavailable", err)
	}
	args := []string{
		"--request", request,
		"--htrace", unit.Path,
		"--trace-finding-out", findingPath,
		"--repo", flagRepo,
		"--branch", flagBranch,
		"--lang", flagLang,
		"--color", "never",
		"--log-level", flagLogLevel,
	}
	if strings.TrimSpace(flagProviders) != "" {
		args = append(args, "--providers", flagProviders)
	}
	if flagMaxSteps > 0 {
		args = append(args, "--pipeline-max-steps", fmt.Sprintf("%d", flagMaxSteps))
	}
	child := exec.CommandContext(ctx, executable, args...)
	child.Stdout = logFile
	child.Stderr = logFile
	if err := child.Run(); err != nil {
		return failedTraceBatchUnit(unit.UnitID, "read_run_failed", fmt.Errorf("%v; log=%s", err, logPath))
	}
	finding, err := loadTraceFindingFile(findingPath)
	if err != nil {
		return failedTraceBatchUnit(unit.UnitID, "finding_schema_invalid", err)
	}
	return traceBatchUnitResult{input: &tracecluster.Input{UnitID: unit.UnitID, Finding: *finding}}
}

func failedTraceBatchUnit(unitID, code string, err error) traceBatchUnitResult {
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	return traceBatchUnitResult{failure: &types.TraceBatchFailure{UnitID: unitID, Code: code, Detail: detail}}
}

func discoverTraceBatchUnits(inputDir string, patterns []string, recursive bool) ([]traceBatchUnit, error) {
	dir, err := filepath.Abs(strings.TrimSpace(inputDir))
	if err != nil {
		return nil, fmt.Errorf("resolve trace input directory: %w", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("inspect trace input directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("trace input is not a directory: %s", dir)
	}
	if len(patterns) == 0 {
		patterns = defaultTraceBatchPatterns()
	}
	var paths []string
	err = filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != dir && !recursive {
				return filepath.SkipDir
			}
			return nil
		}
		if traceBatchNameMatches(entry.Name(), patterns) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan trace input directory: %w", err)
	}
	sort.Strings(paths)
	units := make([]traceBatchUnit, 0, len(paths))
	used := map[string]int{}
	for _, path := range paths {
		base := sanitizeTraceBatchUnitID(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
		if base == "" {
			base = "trace"
		}
		used[base]++
		unitID := base
		if used[base] > 1 {
			unitID = fmt.Sprintf("%s-%03d", base, used[base])
		}
		units = append(units, traceBatchUnit{UnitID: unitID, Path: path})
	}
	return units, nil
}

func defaultTraceBatchPatterns() []string {
	return []string{"*.trace", "*.systrace", "*.htrace", "*.ftrace", "*.perfetto-trace"}
}

func traceBatchNameMatches(name string, patterns []string) bool {
	lower := strings.ToLower(name)
	for _, pattern := range patterns {
		matched, err := filepath.Match(strings.ToLower(strings.TrimSpace(pattern)), lower)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func sanitizeTraceBatchUnitID(value string) string {
	var out strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			out.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			out.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(out.String(), "-")
}

func loadTraceFindingDirectory(dir string) ([]tracecluster.Input, []types.TraceBatchFailure, error) {
	dir, err := filepath.Abs(strings.TrimSpace(dir))
	if err != nil {
		return nil, nil, fmt.Errorf("resolve finding input directory: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("read finding input directory: %w", err)
	}
	var inputs []tracecluster.Input
	var failures []types.TraceBatchFailure
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".trace-finding.json") {
			continue
		}
		unitID := strings.TrimSuffix(entry.Name(), ".trace-finding.json")
		finding, err := loadTraceFindingFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			failures = append(failures, types.TraceBatchFailure{UnitID: unitID, Code: "finding_schema_invalid", Detail: err.Error()})
			continue
		}
		inputs = append(inputs, tracecluster.Input{UnitID: unitID, Finding: *finding})
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].UnitID < inputs[j].UnitID })
	sort.Slice(failures, func(i, j int) bool { return failures[i].UnitID < failures[j].UnitID })
	return inputs, failures, nil
}

func loadTraceFindingFile(path string) (*types.TraceFindingV1, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(io.LimitReader(f, 4<<20))
	var finding types.TraceFindingV1
	if err := dec.Decode(&finding); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	if err := tracefinding.ValidateStored(&finding); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	return &finding, nil
}

func renderTraceClusterMarkdown(set types.TraceRootCauseClusterSetV1, lang string) string {
	zh := strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "zh")
	var b bytes.Buffer
	if zh {
		fmt.Fprintf(&b, "# Trace 根因聚类报告\n\n- 批次：`%s`\n- 输入：%d\n- 成功：%d\n- 已确定根因：%d\n- 未确定：%d\n- 失败：%d\n- 根因组：%d\n\n", set.BatchID, set.InputUnitCount, set.SuccessfulCount, set.ResolvedCount, set.UnresolvedCount, set.FailedCount, len(set.Clusters))
		b.WriteString("## 根因组\n\n| 根因 | 主因样本数 | 已确定样本占比 | 样本 |\n|---|---:|---:|---|\n")
	} else {
		fmt.Fprintf(&b, "# Trace Root-Cause Clusters\n\n- Batch: `%s`\n- Inputs: %d\n- Successful: %d\n- Resolved: %d\n- Unresolved: %d\n- Failed: %d\n- Clusters: %d\n\n", set.BatchID, set.InputUnitCount, set.SuccessfulCount, set.ResolvedCount, set.UnresolvedCount, set.FailedCount, len(set.Clusters))
		b.WriteString("## Clusters\n\n| Root cause | Primary samples | Share of resolved | Samples |\n|---|---:|---:|---|\n")
	}
	clusters := append([]types.TraceRootCauseCluster(nil), set.Clusters...)
	sort.SliceStable(clusters, func(i, j int) bool {
		if clusters[i].PrimaryCount != clusters[j].PrimaryCount {
			return clusters[i].PrimaryCount > clusters[j].PrimaryCount
		}
		return clusters[i].ClusterID < clusters[j].ClusterID
	})
	for _, cluster := range clusters {
		label := cluster.CanonicalLabel
		if zh {
			if display := tool.TraceRootCauseTypeZHLabel(cluster.Fingerprint.Token); display != "" {
				label = display + "（`" + cluster.Fingerprint.Token + "`）"
			}
		}
		members := make([]string, 0, len(cluster.PrimaryMembers))
		for _, member := range cluster.PrimaryMembers {
			members = append(members, "`"+member.UnitID+"`")
		}
		fmt.Fprintf(&b, "| %s | %d | %.1f%% | %s |\n", label, cluster.PrimaryCount, cluster.ShareOfResolved*100, strings.Join(members, "、"))
	}
	if len(clusters) == 0 {
		b.WriteString("| - | 0 | 0.0% | - |\n")
	}
	if len(set.Unresolved) > 0 {
		if zh {
			b.WriteString("\n## 未确定根因\n\n")
		} else {
			b.WriteString("\n## Unresolved\n\n")
		}
		for _, item := range set.Unresolved {
			fmt.Fprintf(&b, "- `%s`: %s\n", item.UnitID, item.Reason)
		}
	}
	if len(set.Failures) > 0 {
		if zh {
			b.WriteString("\n## 分析失败\n\n")
		} else {
			b.WriteString("\n## Failures\n\n")
		}
		for _, failure := range set.Failures {
			fmt.Fprintf(&b, "- `%s` [%s]: %s\n", failure.UnitID, failure.Code, failure.Detail)
		}
	}
	return b.String()
}

func writeNewArtifact(path string, body []byte) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	f, err := os.OpenFile(abs, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return abs, nil
}

func traceFindingUtilityHelp(command *cobra.Command, groups []traceHelpFlagGroup) {
	out := command.OutOrStdout()
	if strings.TrimSpace(command.Long) != "" {
		fmt.Fprintln(out, command.Long)
	} else {
		fmt.Fprintln(out, command.Short)
	}
	fmt.Fprintf(out, "\nUsage:\n  %s\n", command.UseLine())
	if strings.TrimSpace(command.Example) != "" {
		fmt.Fprintf(out, "\nExamples:\n%s\n", command.Example)
	}
	renderTraceHelpFlagGroups(command, groups)
}

func traceBatchHelp(command *cobra.Command, _ []string) {
	traceFindingUtilityHelp(command, []traceHelpFlagGroup{
		{title: "Input", names: []string{"input-dir", "pattern", "recursive"}},
		{title: "Batch", names: []string{"output-dir", "batch-id", "concurrency", "request", "pipeline-max-steps"}},
		{title: "Runtime", names: []string{"providers", "repo", "branch", "log-level"}},
		{title: "Common", names: []string{"lang", "cache-dir", "help"}},
	})
}

func traceClusterHelp(command *cobra.Command, _ []string) {
	traceFindingUtilityHelp(command, []traceHelpFlagGroup{
		{title: "Input and output", names: []string{"input-dir", "output", "format", "batch-id"}},
		{title: "Common", names: []string{"lang", "help"}},
	})
}

func init() {
	traceClusterCmd.Flags().StringVar(&traceClusterInputDir, "input-dir", "", "directory containing *.trace-finding.json sidecars")
	traceClusterCmd.Flags().StringVar(&traceClusterOutput, "output", "", "write report to a new file instead of stdout")
	traceClusterCmd.Flags().StringVar(&traceClusterBatchID, "batch-id", "", "stable batch id; default is timestamped")
	traceClusterCmd.Flags().StringVar(&traceClusterFormat, "format", "markdown", "output format: markdown or json")
	_ = traceClusterCmd.MarkFlagRequired("input-dir")

	traceBatchCmd.Flags().StringVar(&traceBatchInputDir, "input-dir", "", "directory containing independent trace files")
	traceBatchCmd.Flags().StringSliceVar(&traceBatchPatterns, "pattern", defaultTraceBatchPatterns(), "filename patterns to include; repeat or comma-separate")
	traceBatchCmd.Flags().StringVar(&traceBatchOutputDir, "output-dir", "", "new batch output directory; default .codrax/trace-batches/<batch-id>")
	traceBatchCmd.Flags().StringVar(&traceBatchID, "batch-id", "", "stable batch id; default is timestamped")
	traceBatchCmd.Flags().IntVar(&traceBatchConcurrency, "concurrency", 2, "maximum independent trace analyses running at once")
	traceBatchCmd.Flags().BoolVar(&traceBatchRecursive, "recursive", false, "scan subdirectories recursively")
	_ = traceBatchCmd.MarkFlagRequired("input-dir")
	traceClusterCmd.SetHelpFunc(traceClusterHelp)
	traceBatchCmd.SetHelpFunc(traceBatchHelp)

	traceCmd.AddCommand(traceClusterCmd, traceBatchCmd)
}
