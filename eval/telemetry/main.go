package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type options struct {
	format string
	top    int
}

type report struct {
	Files             int                 `json:"files"`
	Lines             int                 `json:"lines"`
	Analyzer          analyzerSummary     `json:"analyzer"`
	Explorer          explorerSummary     `json:"explorer"`
	Finalizer         finalizerSummary    `json:"finalizer"`
	Richness          richnessSummary     `json:"richness"`
	Render            renderSummary       `json:"render"`
	ToolParamCompat   toolParamCompat     `json:"tool_param_compat"`
	LLM               llmSummary          `json:"llm"`
	FilesByRetryScore []fileRetrySnapshot `json:"files_by_retry_score,omitempty"`
}

type analyzerSummary struct {
	ProvenanceEvents int            `json:"provenance_events"`
	Provenance       provenanceSum  `json:"provenance"`
	BlocklistEvents  int            `json:"blocklist_events"`
	BlocklistShadow  blocklistSum   `json:"blocklist_shadow"`
	RetryRenders     int            `json:"retry_renders"`
	ToolRejects      int            `json:"tool_rejects"`
	HardGateFailures int            `json:"hard_gate_failures"`
	RejectReasons    map[string]int `json:"reject_reasons,omitempty"`
}

type explorerSummary struct {
	MidLoopSignals    int            `json:"midloop_signals"`
	MidLoopInjects    int            `json:"midloop_injects"`
	MidLoopForceStops int            `json:"midloop_force_stops"`
	ByKey             map[string]int `json:"by_key,omitempty"`
	ByAction          map[string]int `json:"by_action,omitempty"`
}

type provenanceSum struct {
	Total           int     `json:"total"`
	Scope           int     `json:"scope"`
	Symbol          int     `json:"symbol"`
	File            int     `json:"file"`
	PrescanAnchor   int     `json:"prescan_anchor"`
	InferredConcept int     `json:"inferred_concept"`
	UseForSearch    int     `json:"use_for_search"`
	UseForShape     int     `json:"use_for_shape"`
	MeanNoise       float64 `json:"mean_noise"`
}

type blocklistSum struct {
	Dropped         int            `json:"dropped"`
	WouldSearch     int            `json:"would_search"`
	WouldShape      int            `json:"would_shape"`
	InferredConcept int            `json:"inferred_concept"`
	Samples         map[string]int `json:"samples,omitempty"`
}

type finalizerSummary struct {
	ToolRejects                int            `json:"tool_rejects"`
	DocumentRejects            int            `json:"document_rejects"`
	PatchRejects               int            `json:"patch_rejects"`
	ContractViolations         int            `json:"contract_violations"`
	ContractViolationBySection map[string]int `json:"contract_violation_by_section,omitempty"`
	RewriteRenders             int            `json:"rewrite_renders"`
	ConsistencyRenders         int            `json:"consistency_renders"`
	RepairPlans                int            `json:"repair_plans"`
	RepairKinds                map[string]int `json:"repair_kinds,omitempty"`
	RepairTargets              map[string]int `json:"repair_targets,omitempty"`
}

type richnessSummary struct {
	Events      int            `json:"events"`
	ByKind      map[string]int `json:"by_kind,omitempty"`
	ByFamily    map[string]int `json:"by_family,omitempty"`
	ByFacetKind map[string]int `json:"by_facet_kind,omitempty"`
}

type renderSummary struct {
	StatusEvents                 int            `json:"status_events"`
	StageCounts                  map[string]int `json:"stage_counts,omitempty"`
	StageRegressions             int            `json:"stage_regressions"`
	BacktracksAfterFinalize      int            `json:"backtracks_after_finalize"`
	FirstDraftPreviews           int            `json:"first_draft_previews"`
	SuspiciousFirstDraftPreviews int            `json:"suspicious_first_draft_previews"`
	Anomalies                    map[string]int `json:"anomalies,omitempty"`
}

type toolParamCompat struct {
	SanitizedInvalidArgs int            `json:"sanitized_invalid_args"`
	AutoRepairedParams   int            `json:"auto_repaired_params"`
	Tools                map[string]int `json:"tools,omitempty"`
}

type llmSummary struct {
	Errors   int            `json:"errors"`
	Timeouts int            `json:"timeouts"`
	ByAgent  map[string]int `json:"by_agent,omitempty"`
	ByModel  map[string]int `json:"by_model,omitempty"`
}

type fileRetrySnapshot struct {
	Path              string `json:"path"`
	Score             int    `json:"score"`
	AnalyzerRetries   int    `json:"analyzer_retries"`
	AnalyzerRejects   int    `json:"analyzer_rejects"`
	ExplorerInjects   int    `json:"explorer_injects"`
	FinalizerRejects  int    `json:"finalizer_rejects"`
	FinalizerRewrites int    `json:"finalizer_rewrites"`
	RepairPlans       int    `json:"repair_plans"`
	BlocklistDropped  int    `json:"blocklist_dropped"`
	RenderAnomalies   int    `json:"render_anomalies"`
}

type fileMetrics struct {
	path               string
	lines              int
	analyzerRetries    int
	analyzerRejects    int
	explorerInjects    int
	finalizerRejects   int
	finalizerRewrites  int
	repairPlans        int
	blocklistDropped   int
	renderAnomalies    int
	firstDraftPreviews int
	lastRenderStage    int
	seenFinalizerStage bool
}

type collector struct {
	report report
	files  map[string]*fileMetrics
}

var (
	provenanceRe    = regexp.MustCompile(`entity provenance summary:\s*(.*)$`)
	blocklistRe     = regexp.MustCompile(`blocklist_shadow:\s*(.*)$`)
	toolRejectRe    = regexp.MustCompile(`TOOLRESULT\s+([a-zA-Z0-9_]+)\s+ok=false`)
	repairPlanRe    = regexp.MustCompile(`repair_plan:\s*(.*)$`)
	contractCheckRe = regexp.MustCompile(`answer_contract_check\s+section=([a-zA-Z0-9_]+).*violations=([0-9]+)`)
	richnessRe      = regexp.MustCompile(`\[richness\]\s+([a-zA-Z0-9_]+)\s+(.*)$`)
	renderStageRe   = regexp.MustCompile(`(?:^|[^0-9])([1-4])/4(?:[^0-9]|$)`)
	midLoopSignalRe = regexp.MustCompile(`phase=midloop_signal.*key="([^"]*)".*→\s*([a-zA-Z_]+)`)
	compatToolRe    = regexp.MustCompile(`tool\s+"([^"]+)"\s+params auto-repaired`)
	llmRequestRe    = regexp.MustCompile(`\[llm\]\s+request:\s+model=([^ ]+)`)
	diagAgentRe     = regexp.MustCompile(`\[diag ([a-zA-Z0-9_]+)\]`)
	keyValueIntRe   = regexp.MustCompile(`([a-zA-Z_]+)=(-?[0-9]+)`)
	keyValueFloatRe = regexp.MustCompile(`([a-zA-Z_]+)=(-?[0-9]+(?:\.[0-9]+)?)`)
	repairKindsRe   = regexp.MustCompile(`kinds=\[([^\]]*)\]`)
	repairTargetRe  = regexp.MustCompile(`target=([a-zA-Z0-9_]+)`)
)

func main() {
	opts := options{}
	flag.StringVar(&opts.format, "format", "markdown", "output format: markdown or json")
	flag.IntVar(&opts.top, "top", 12, "number of highest-retry files to include")
	flag.Parse()

	paths := flag.Args()
	if len(paths) == 0 {
		paths = []string{"eval/results", ".codrax/logs", "../.codrax/logs", "../customlogs"}
	}
	rep, err := collect(paths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry collect failed: %v\n", err)
		os.Exit(1)
	}
	switch strings.ToLower(opts.format) {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintf(os.Stderr, "telemetry json failed: %v\n", err)
			os.Exit(1)
		}
	case "markdown", "md":
		writeMarkdown(os.Stdout, rep, opts.top)
	default:
		fmt.Fprintf(os.Stderr, "unknown -format %q (want markdown or json)\n", opts.format)
		os.Exit(2)
	}
}

func collect(paths []string) (report, error) {
	c := &collector{
		files: make(map[string]*fileMetrics),
	}
	c.report.Analyzer.RejectReasons = map[string]int{}
	c.report.Analyzer.BlocklistShadow.Samples = map[string]int{}
	c.report.Explorer.ByKey = map[string]int{}
	c.report.Explorer.ByAction = map[string]int{}
	c.report.Finalizer.ContractViolationBySection = map[string]int{}
	c.report.Finalizer.RepairKinds = map[string]int{}
	c.report.Finalizer.RepairTargets = map[string]int{}
	c.report.Richness.ByKind = map[string]int{}
	c.report.Richness.ByFamily = map[string]int{}
	c.report.Richness.ByFacetKind = map[string]int{}
	c.report.Render.StageCounts = map[string]int{}
	c.report.Render.Anomalies = map[string]int{}
	c.report.ToolParamCompat.Tools = map[string]int{}
	c.report.LLM.ByAgent = map[string]int{}
	c.report.LLM.ByModel = map[string]int{}

	var scanErrs []string
	for _, p := range paths {
		if err := scanPath(p, c); err != nil {
			scanErrs = append(scanErrs, err.Error())
		}
	}
	c.finalize()
	if len(scanErrs) > 0 && c.report.Files == 0 {
		return c.report, errors.New(strings.Join(scanErrs, "; "))
	}
	return c.report, nil
}

func scanPath(root string, c *collector) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return scanFile(root, c)
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", ".codrax-blob":
				return filepath.SkipDir
			}
			return nil
		}
		if looksLikeLogFile(path) {
			_ = scanFile(path, c)
		}
		return nil
	})
}

func looksLikeLogFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".log", ".txt", ".out":
		return true
	default:
		return false
	}
}

func scanFile(path string, c *collector) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	fm := &fileMetrics{path: path}
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 256*1024)
	scanner.Buffer(buf, 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		fm.lines++
		c.observeLine(line, fm)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	c.report.Files++
	c.report.Lines += fm.lines
	c.files[path] = fm
	return nil
}

func (c *collector) observeLine(line string, fm *fileMetrics) {
	if strings.Contains(line, "entity provenance summary:") {
		c.observeProvenance(line)
	}
	if strings.Contains(line, "blocklist_shadow:") {
		dropped := c.observeBlocklist(line)
		fm.blocklistDropped += dropped
	}
	if strings.Contains(line, "[richness]") {
		c.observeRichness(line)
	}
	if isControlComponentLine(line, "DEBUG", "diag explorer") {
		c.observeExplorerLine(line, fm)
	}
	c.observeRenderLine(line, fm)
	if (isControlComponentLine(line, "DEBUG", "diag analyzer") && strings.Contains(line, "TOOLRESULT emit_analysis ok=false")) ||
		(isControlComponentLine(line, "DEBUG", "diag analyzer") && strings.Contains(line, "emit_analysis rejected:")) ||
		(isControlComponentLine(line, "WARN", "orchestrator") && strings.Contains(line, "analyzer quality gate rejected")) {
		c.report.Analyzer.ToolRejects++
		fm.analyzerRejects++
		if reason := analyzerRejectReason(line); reason != "" {
			c.report.Analyzer.RejectReasons[reason]++
		}
	}
	if isControlComponentLine(line, "ERROR", "analyzer-v3") && strings.Contains(line, "quality gate HARD failure") {
		c.report.Analyzer.HardGateFailures++
	}
	if isRenderInfoLine(line) && (strings.Contains(line, "⟳ 1/4") || strings.Contains(line, "重新理解问题")) {
		c.report.Analyzer.RetryRenders++
		fm.analyzerRetries++
	}
	if m := toolRejectRe.FindStringSubmatch(line); len(m) == 2 && isDiagFinalizerToolResultLine(line) {
		switch m[1] {
		case "emit_answer_document":
			c.report.Finalizer.ToolRejects++
			c.report.Finalizer.DocumentRejects++
			fm.finalizerRejects++
		case "emit_answer_document_patch":
			c.report.Finalizer.ToolRejects++
			c.report.Finalizer.PatchRejects++
			fm.finalizerRejects++
		}
	}
	if isFinalizerRejectRenderLine(line) {
		c.report.Finalizer.ToolRejects++
		fm.finalizerRejects++
	}
	if isFinalizerRewriteRenderLine(line) {
		c.report.Finalizer.RewriteRenders++
		fm.finalizerRewrites++
	}
	if (isRenderInfoLine(line) && strings.Contains(line, "检查答案是否前后一致")) ||
		isControlComponentLine(line, "INFO", "self_consistency_reviewer") {
		c.report.Finalizer.ConsistencyRenders++
	}
	if isControlComponentLine(line, "INFO", "orchestrator") && strings.Contains(line, "repair_plan:") {
		c.observeRepairPlan(line)
		fm.repairPlans++
	}
	if isDiagFinalizerLine(line) && strings.Contains(line, "answer_contract_check") && strings.Contains(line, "violations=") {
		c.observeContractCheck(line, fm)
	}
	if strings.Contains(line, "sanitized invalid tool-call arguments") {
		c.report.ToolParamCompat.SanitizedInvalidArgs++
	}
	if m := compatToolRe.FindStringSubmatch(line); len(m) == 2 {
		c.report.ToolParamCompat.AutoRepairedParams++
		c.report.ToolParamCompat.Tools[m[1]]++
	}
	if isLLMErrorLine(line) {
		c.report.LLM.Errors++
		if isTimeoutLine(line) {
			c.report.LLM.Timeouts++
		}
		if m := diagAgentRe.FindStringSubmatch(line); len(m) == 2 {
			c.report.LLM.ByAgent[m[1]]++
		}
		if m := llmRequestRe.FindStringSubmatch(line); len(m) == 2 {
			c.report.LLM.ByModel[m[1]]++
		}
	}
}

func (c *collector) observeExplorerLine(line string, fm *fileMetrics) {
	if strings.Contains(line, "phase=midloop_signal") {
		c.report.Explorer.MidLoopSignals++
		if m := midLoopSignalRe.FindStringSubmatch(line); len(m) == 3 {
			key := strings.TrimSpace(m[1])
			action := strings.TrimSpace(m[2])
			if key != "" {
				c.report.Explorer.ByKey[key]++
			}
			if action != "" {
				c.report.Explorer.ByAction[action]++
			}
		}
	}
	if strings.Contains(line, "phase=midloop_inject") {
		c.report.Explorer.MidLoopInjects++
		fm.explorerInjects++
	}
	if strings.Contains(line, "phase=midloop_force_stop") {
		c.report.Explorer.MidLoopForceStops++
	}
}

func (c *collector) observeProvenance(line string) {
	m := provenanceRe.FindStringSubmatch(line)
	if len(m) != 2 {
		return
	}
	kv := parseInts(m[1])
	p := &c.report.Analyzer.Provenance
	c.report.Analyzer.ProvenanceEvents++
	p.Total += kv["total"]
	p.Scope += kv["scope"]
	p.Symbol += kv["symbol"]
	p.File += kv["file"]
	p.PrescanAnchor += kv["prescan_anchor"]
	p.InferredConcept += kv["inferred_concept"]
	p.UseForSearch += kv["use_for_search"]
	p.UseForShape += kv["use_for_shape"]
	if noise, ok := parseFloats(m[1])["mean_noise"]; ok {
		oldEvents := c.report.Analyzer.ProvenanceEvents - 1
		p.MeanNoise = ((p.MeanNoise * float64(oldEvents)) + noise) / float64(c.report.Analyzer.ProvenanceEvents)
	}
}

func (c *collector) observeBlocklist(line string) int {
	m := blocklistRe.FindStringSubmatch(line)
	if len(m) != 2 {
		return 0
	}
	kv := parseInts(m[1])
	b := &c.report.Analyzer.BlocklistShadow
	c.report.Analyzer.BlocklistEvents++
	b.Dropped += kv["dropped"]
	b.WouldSearch += kv["would_search"]
	b.WouldShape += kv["would_shape"]
	b.InferredConcept += kv["inferred_concept"]
	for _, s := range parseSample(m[1]) {
		b.Samples[s]++
	}
	return kv["dropped"]
}

func (c *collector) observeRichness(line string) {
	m := richnessRe.FindStringSubmatch(line)
	if len(m) != 3 {
		return
	}
	kind := strings.TrimSpace(m[1])
	if kind == "" {
		return
	}
	c.report.Richness.Events++
	c.report.Richness.ByKind[kind]++
	body := m[2]
	if family := parseTokenValue(body, "family"); family != "" {
		c.report.Richness.ByFamily[family]++
	}
	if facet := parseTokenValue(body, "facet_kind"); facet != "" {
		c.report.Richness.ByFacetKind[facet]++
	}
}

func (c *collector) observeRenderLine(line string, fm *fileMetrics) {
	if !isRenderInfoLine(line) {
		return
	}
	payload := strings.TrimSpace(renderPayload(line))
	if strings.HasPrefix(payload, "• 第一稿答案") || strings.HasPrefix(payload, "• First draft answer") {
		c.report.Render.FirstDraftPreviews++
		fm.firstDraftPreviews++
	}
	if stage := parseRenderStage(payload); stage > 0 {
		c.report.Render.StatusEvents++
		c.report.Render.StageCounts[strconv.Itoa(stage)]++
		if fm.lastRenderStage > 0 && stage < fm.lastRenderStage {
			c.observeRenderAnomaly(fm, "stage_regression")
			c.report.Render.StageRegressions++
		}
		if fm.seenFinalizerStage && stage < 4 {
			c.observeRenderAnomaly(fm, "backtrack_after_finalize")
			c.report.Render.BacktracksAfterFinalize++
		}
		if stage == 4 {
			fm.seenFinalizerStage = true
		}
		fm.lastRenderStage = stage
	}
	if isTurnBoundary(line) {
		fm.lastRenderStage = 0
		fm.seenFinalizerStage = false
	}
}

func (c *collector) observeRenderAnomaly(fm *fileMetrics, name string) {
	c.report.Render.Anomalies[name]++
	fm.renderAnomalies++
}

func (c *collector) observeRepairPlan(line string) {
	m := repairPlanRe.FindStringSubmatch(line)
	if len(m) != 2 {
		return
	}
	c.report.Finalizer.RepairPlans++
	body := m[1]
	if km := repairKindsRe.FindStringSubmatch(body); len(km) == 2 {
		for _, kind := range splitList(km[1]) {
			c.report.Finalizer.RepairKinds[kind]++
		}
	}
	if tm := repairTargetRe.FindStringSubmatch(body); len(tm) == 2 {
		c.report.Finalizer.RepairTargets[tm[1]]++
	}
}

func (c *collector) observeContractCheck(line string, fm *fileMetrics) {
	m := contractCheckRe.FindStringSubmatch(line)
	if len(m) != 3 {
		return
	}
	n, err := strconv.Atoi(m[2])
	if err != nil || n <= 0 {
		return
	}
	c.report.Finalizer.ContractViolations += n
	c.report.Finalizer.ContractViolationBySection[m[1]] += n
	fm.finalizerRejects += n
}

func (c *collector) finalize() {
	out := make([]fileRetrySnapshot, 0, len(c.files))
	for _, fm := range c.files {
		if fm.firstDraftPreviews > 0 && fm.finalizerRejects == 0 && fm.finalizerRewrites == 0 && fm.repairPlans == 0 {
			c.report.Render.SuspiciousFirstDraftPreviews += fm.firstDraftPreviews
			c.report.Render.Anomalies["first_draft_without_retry"] += fm.firstDraftPreviews
			fm.renderAnomalies += fm.firstDraftPreviews
		}
		score := fm.analyzerRetries + fm.analyzerRejects*2 + fm.explorerInjects + fm.finalizerRejects*3 + fm.finalizerRewrites*2 + fm.repairPlans*2 + fm.renderAnomalies*2
		if score == 0 && fm.blocklistDropped == 0 {
			continue
		}
		out = append(out, fileRetrySnapshot{
			Path:              fm.path,
			Score:             score,
			AnalyzerRetries:   fm.analyzerRetries,
			AnalyzerRejects:   fm.analyzerRejects,
			ExplorerInjects:   fm.explorerInjects,
			FinalizerRejects:  fm.finalizerRejects,
			FinalizerRewrites: fm.finalizerRewrites,
			RepairPlans:       fm.repairPlans,
			BlocklistDropped:  fm.blocklistDropped,
			RenderAnomalies:   fm.renderAnomalies,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Path < out[j].Path
	})
	c.report.FilesByRetryScore = out
}

func parseInts(s string) map[string]int {
	out := map[string]int{}
	for _, m := range keyValueIntRe.FindAllStringSubmatch(s, -1) {
		n, err := strconv.Atoi(m[2])
		if err == nil {
			out[m[1]] = n
		}
	}
	return out
}

func parseFloats(s string) map[string]float64 {
	out := map[string]float64{}
	for _, m := range keyValueFloatRe.FindAllStringSubmatch(s, -1) {
		f, err := strconv.ParseFloat(m[2], 64)
		if err == nil {
			out[m[1]] = f
		}
	}
	return out
}

func parseSample(s string) []string {
	idx := strings.Index(s, "sample=")
	if idx < 0 {
		return nil
	}
	raw := strings.TrimSpace(s[idx+len("sample="):])
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || strings.HasPrefix(p, "+") {
			continue
		}
		out = append(out, p)
	}
	return out
}

func splitList(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

func parseTokenValue(s, key string) string {
	needle := key + "="
	idx := strings.Index(s, needle)
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(needle):]
	rest = strings.TrimLeft(rest, " \t")
	if rest == "" {
		return ""
	}
	end := len(rest)
	for i, r := range rest {
		if r == ' ' || r == '\t' {
			end = i
			break
		}
	}
	return strings.TrimSpace(rest[:end])
}

func parseRenderStage(line string) int {
	if !looksLikeRenderStatusPayload(line) {
		return 0
	}
	m := renderStageRe.FindStringSubmatch(line)
	if len(m) != 2 {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}

func looksLikeRenderStatusPayload(line string) bool {
	s := strings.TrimSpace(line)
	for _, prefix := range []string{"⟳", "›", "✓", "▪", "·", "◆", "⠇", "⠏", "⠸", "⠹", "⠧", "⠼"} {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func isTurnBoundary(line string) bool {
	return strings.Contains(line, "◆ 已结束") ||
		strings.Contains(line, "◆ Done") ||
		strings.Contains(line, "─────────────────────────────────────") ||
		strings.Contains(line, "] > ")
}

func analyzerRejectReason(line string) string {
	for _, marker := range []string{
		"emit_analysis rejected:",
		"analyzer quality gate rejected:",
		"quality gate HARD failure:",
	} {
		if idx := strings.Index(line, marker); idx >= 0 {
			reason := strings.TrimSpace(line[idx+len(marker):])
			return compactReason(reason)
		}
	}
	return ""
}

func compactReason(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) > 120 {
		s = s[:120]
	}
	return strings.Join(strings.Fields(s), " ")
}

func isLLMErrorLine(line string) bool {
	if strings.Contains(line, "模型响应出错") {
		return isRenderInfoLine(line)
	}
	if !isLLMControlLine(line) {
		return false
	}
	lower := strings.ToLower(line)
	if strings.Contains(lower, "[llm] request:") || strings.Contains(lower, "[llm] response:") {
		return false
	}
	return strings.Contains(lower, "error") ||
		strings.Contains(lower, "fail") ||
		strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "deadline") ||
		strings.Contains(lower, "stall") ||
		strings.Contains(lower, "eof")
}

func isTimeoutLine(line string) bool {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "[llm] request:") || strings.Contains(lower, "[llm] response:") {
		return false
	}
	return strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "deadline") ||
		strings.Contains(lower, "first_byte") ||
		strings.Contains(lower, "stall")
}

func isDiagFinalizerLine(line string) bool {
	return isControlComponentLine(line, "DEBUG", "diag finalizer")
}

func isDiagFinalizerToolResultLine(line string) bool {
	return isDiagFinalizerLine(line) && strings.Contains(line, "phase=toolresult")
}

func isRenderInfoLine(line string) bool {
	return isControlComponentLine(line, "INFO", "render")
}

func renderPayload(line string) string {
	body := controlLineBody(line)
	prefix := "INFO [render]"
	if !strings.HasPrefix(body, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(body, prefix))
}

func isTimestampedCodraxLine(line string) bool {
	return len(line) >= len("2006-01-02T") &&
		line[0] == '2' &&
		line[1] == '0' &&
		line[4] == '-' &&
		line[7] == '-' &&
		line[10] == 'T'
}

func controlLineBody(line string) string {
	if !isTimestampedCodraxLine(line) {
		return ""
	}
	idx := strings.IndexByte(line, ' ')
	if idx < 0 || idx+1 >= len(line) {
		return ""
	}
	return line[idx+1:]
}

func isControlComponentLine(line, level, component string) bool {
	body := controlLineBody(line)
	if body == "" {
		return false
	}
	return strings.HasPrefix(body, level+" ["+component+"]")
}

func isLLMControlLine(line string) bool {
	body := controlLineBody(line)
	if body == "" {
		return false
	}
	return strings.HasPrefix(body, "DEBUG [llm]") ||
		strings.HasPrefix(body, "INFO [llm]") ||
		strings.HasPrefix(body, "WARN [llm]") ||
		strings.HasPrefix(body, "ERROR [llm]")
}

func isFinalizerRejectRenderLine(line string) bool {
	if !isRenderInfoLine(line) {
		return false
	}
	payload := strings.TrimSpace(renderPayload(line))
	return strings.HasPrefix(payload, "• 成文校验未通过") ||
		strings.HasPrefix(payload, "• 成文交验未通过")
}

func isFinalizerRewriteRenderLine(line string) bool {
	if !isRenderInfoLine(line) {
		return false
	}
	payload := strings.TrimSpace(renderPayload(line))
	if !strings.HasPrefix(payload, "⟳ 4/4") {
		return false
	}
	return strings.Contains(payload, "答案待完善") ||
		strings.Contains(payload, "正在重写答案") ||
		(strings.Contains(payload, "检测到 ") && strings.Contains(payload, "前后不一致"))
}

func writeMarkdown(w io.Writer, rep report, top int) {
	fmt.Fprintln(w, "# Codrax Telemetry Report")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- files: %d\n", rep.Files)
	fmt.Fprintf(w, "- lines: %d\n", rep.Lines)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Analyzer")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- provenance events: %d\n", rep.Analyzer.ProvenanceEvents)
	fmt.Fprintf(w, "- provenance totals: total=%d scope=%d symbol=%d file=%d prescan_anchor=%d inferred_concept=%d use_for_search=%d use_for_shape=%d mean_noise=%.2f\n",
		rep.Analyzer.Provenance.Total,
		rep.Analyzer.Provenance.Scope,
		rep.Analyzer.Provenance.Symbol,
		rep.Analyzer.Provenance.File,
		rep.Analyzer.Provenance.PrescanAnchor,
		rep.Analyzer.Provenance.InferredConcept,
		rep.Analyzer.Provenance.UseForSearch,
		rep.Analyzer.Provenance.UseForShape,
		rep.Analyzer.Provenance.MeanNoise,
	)
	fmt.Fprintf(w, "- blocklist shadow: events=%d dropped=%d would_search=%d would_shape=%d inferred_concept=%d\n",
		rep.Analyzer.BlocklistEvents,
		rep.Analyzer.BlocklistShadow.Dropped,
		rep.Analyzer.BlocklistShadow.WouldSearch,
		rep.Analyzer.BlocklistShadow.WouldShape,
		rep.Analyzer.BlocklistShadow.InferredConcept,
	)
	fmt.Fprintf(w, "- analyzer retries: renders=%d tool_rejects=%d hard_gate_failures=%d\n",
		rep.Analyzer.RetryRenders,
		rep.Analyzer.ToolRejects,
		rep.Analyzer.HardGateFailures,
	)
	writeTopMap(w, "Analyzer reject reasons", rep.Analyzer.RejectReasons, 8)
	writeTopMap(w, "Blocklist samples", rep.Analyzer.BlocklistShadow.Samples, 8)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Explorer")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- mid-loop: signals=%d injects=%d force_stops=%d\n",
		rep.Explorer.MidLoopSignals,
		rep.Explorer.MidLoopInjects,
		rep.Explorer.MidLoopForceStops,
	)
	writeTopMap(w, "Explorer mid-loop keys", rep.Explorer.ByKey, 12)
	writeTopMap(w, "Explorer mid-loop actions", rep.Explorer.ByAction, 8)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Finalizer")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- rejects: total=%d document=%d patch=%d\n",
		rep.Finalizer.ToolRejects,
		rep.Finalizer.DocumentRejects,
		rep.Finalizer.PatchRejects,
	)
	fmt.Fprintf(w, "- contract violations: total=%d\n", rep.Finalizer.ContractViolations)
	fmt.Fprintf(w, "- rewrites: render_lines=%d consistency_lines=%d repair_plans=%d\n",
		rep.Finalizer.RewriteRenders,
		rep.Finalizer.ConsistencyRenders,
		rep.Finalizer.RepairPlans,
	)
	writeTopMap(w, "Repair kinds", rep.Finalizer.RepairKinds, 10)
	writeTopMap(w, "Repair targets", rep.Finalizer.RepairTargets, 10)
	writeTopMap(w, "Contract violation sections", rep.Finalizer.ContractViolationBySection, 10)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Richness")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- events: %d\n", rep.Richness.Events)
	writeTopMap(w, "Richness kinds", rep.Richness.ByKind, 8)
	writeTopMap(w, "Richness families", rep.Richness.ByFamily, 8)
	writeTopMap(w, "Richness facet kinds", rep.Richness.ByFacetKind, 8)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Render")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- status events: %d stage_regressions=%d backtracks_after_finalize=%d\n",
		rep.Render.StatusEvents,
		rep.Render.StageRegressions,
		rep.Render.BacktracksAfterFinalize,
	)
	fmt.Fprintf(w, "- first draft previews: total=%d suspicious_without_retry=%d\n",
		rep.Render.FirstDraftPreviews,
		rep.Render.SuspiciousFirstDraftPreviews,
	)
	writeTopMap(w, "Render stage counts", rep.Render.StageCounts, 4)
	writeTopMap(w, "Render anomalies", rep.Render.Anomalies, 8)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Compatibility And Transport")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- tool param compat: sanitized_invalid_args=%d auto_repaired_params=%d\n",
		rep.ToolParamCompat.SanitizedInvalidArgs,
		rep.ToolParamCompat.AutoRepairedParams,
	)
	writeTopMap(w, "Auto-repaired tools", rep.ToolParamCompat.Tools, 8)
	fmt.Fprintf(w, "- LLM: errors=%d timeouts=%d\n", rep.LLM.Errors, rep.LLM.Timeouts)
	writeTopMap(w, "LLM errors by agent", rep.LLM.ByAgent, 8)
	writeTopMap(w, "LLM errors by model", rep.LLM.ByModel, 8)

	if top > 0 && len(rep.FilesByRetryScore) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## Hot Logs")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "| file | score | ana_retry | ana_reject | exp_inject | fin_reject | fin_rewrite | repair | blocklist_drop | render_anom |")
		fmt.Fprintln(w, "|------|------:|----------:|-----------:|-----------:|-----------:|------------:|-------:|---------------:|------------:|")
		n := len(rep.FilesByRetryScore)
		if n > top {
			n = top
		}
		for _, f := range rep.FilesByRetryScore[:n] {
			fmt.Fprintf(w, "| `%s` | %d | %d | %d | %d | %d | %d | %d | %d | %d |\n",
				escapePipe(f.Path),
				f.Score,
				f.AnalyzerRetries,
				f.AnalyzerRejects,
				f.ExplorerInjects,
				f.FinalizerRejects,
				f.FinalizerRewrites,
				f.RepairPlans,
				f.BlocklistDropped,
				f.RenderAnomalies,
			)
		}
	}
}

func writeTopMap(w io.Writer, title string, values map[string]int, limit int) {
	if len(values) == 0 || limit <= 0 {
		return
	}
	type kv struct {
		key string
		val int
	}
	items := make([]kv, 0, len(values))
	for k, v := range values {
		if k != "" && v > 0 {
			items = append(items, kv{k, v})
		}
	}
	if len(items) == 0 {
		return
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].val != items[j].val {
			return items[i].val > items[j].val
		}
		return items[i].key < items[j].key
	})
	if len(items) > limit {
		items = items[:limit]
	}
	fmt.Fprintf(w, "\n### %s\n\n", title)
	fmt.Fprintln(w, "| item | count |")
	fmt.Fprintln(w, "|------|------:|")
	for _, item := range items {
		fmt.Fprintf(w, "| `%s` | %d |\n", escapePipe(item.key), item.val)
	}
}

func escapePipe(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}
