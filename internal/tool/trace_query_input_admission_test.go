package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryBinaryAdmissionIsViewInvariantAndSideEffectFree(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "customer capture.sys")
	if err := os.WriteFile(path, append([]byte("PERFILE2"), make([]byte, 64)...), 0o644); err != nil {
		t.Fatal(err)
	}
	var firstSummary string
	for _, view := range []string{"event_search", "window_sweep", "root_cause_rank", "frame_root_cause_bundle"} {
		t.Run(view, func(t *testing.T) {
			mutable := types.NewMutableState("analyze the binary trace")
			ctx := &types.BusContext{
				RepoRoot:   dir,
				WorkDir:    dir,
				Mutable:    mutable,
				AnalysisIR: &types.AnalysisIR{},
			}
			params, err := json.Marshal(map[string]any{
				"source":     "path",
				"path":       path,
				"view":       view,
				"pid":        20,
				"time_start": 1.0,
				"time_end":   2.0,
				"limit":      10,
			})
			if err != nil {
				t.Fatal(err)
			}
			result, executeErr := (&TraceQuery{}).Execute(ctx, params)
			if executeErr != nil {
				t.Fatal(executeErr)
			}
			if result.Success || result.RawRef != "" || len(result.Observations) != 0 {
				t.Fatalf("binary input published a result face: %+v", result)
			}
			if result.Repair == nil || result.Repair.Code != tracequery.TraceInputAdmissionCodeConversionRequired {
				t.Fatalf("missing typed conversion verdict: %+v", result.Repair)
			}
			for _, want := range []string{"code=trace_conversion_required", "linux_perf_data", "codrax trace convert --input", "customer capture.sys"} {
				if !strings.Contains(result.Summary, want) {
					t.Fatalf("summary missing %q: %s", want, result.Summary)
				}
			}
			if result.Repair.Metadata["stage"] != "trace_input_admission" ||
				result.Repair.Metadata["status"] != types.ToolRepairStatusActionRequired ||
				!strings.Contains(result.Repair.Metadata["command"], "codrax trace convert --input") {
				t.Fatalf("repair metadata=%v", result.Repair.Metadata)
			}
			if firstSummary == "" {
				firstSummary = result.Summary
			} else if result.Summary != firstSummary {
				t.Fatalf("view changed admission error:\nfirst=%s\nthis=%s", firstSummary, result.Summary)
			}
			if len(ctx.AnalysisIR.RequestModel.RuntimeTargets) != 0 {
				t.Fatalf("rejected input minted analysis target: %+v", ctx.AnalysisIR.RequestModel.RuntimeTargets)
			}
			if requestModel := mutable.RequestModel(); requestModel != nil && len(requestModel.RuntimeTargets) != 0 {
				t.Fatalf("rejected input minted mutable target: %+v", requestModel.RuntimeTargets)
			}
			if windows := mutable.TraceQueryCallWindows(); len(windows) != 0 {
				t.Fatalf("rejected input minted supplement windows: %+v", windows)
			}
		})
	}
}

func TestTraceQueryEmptyAdmissionDoesNotRecommendConversion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.htrace")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search"})
	result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success || result.Repair == nil || result.Repair.Code != tracequery.TraceInputAdmissionCodeEmpty {
		t.Fatalf("empty input verdict=%+v", result)
	}
	if strings.Contains(result.Summary, "trace convert") || result.Repair.Metadata["command"] != "" || !strings.Contains(result.Summary, "collect a non-empty text trace") {
		t.Fatalf("empty input got misleading conversion recovery: %+v", result)
	}
}

func TestTraceQueryRejectsDirectBinaryAttachedPayloadBeforeBlobMaterialization(t *testing.T) {
	dir := t.TempDir()
	mutable := types.NewMutableState("analyze direct binary attachment")
	ctx := &types.BusContext{
		WorkDir:         dir,
		AttachedHitrace: string(append([]byte("PERFILE2"), make([]byte, 64)...)),
		Mutable:         mutable,
		AnalysisIR:      &types.AnalysisIR{},
	}
	params, _ := json.Marshal(map[string]any{
		"source":     "attached_trace",
		"view":       "root_cause_rank",
		"pid":        20,
		"time_start": 1.0,
		"time_end":   2.0,
	})
	result, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success || result.Repair == nil || result.Repair.Code != tracequery.TraceInputAdmissionCodeConversionRequired {
		t.Fatalf("direct binary attachment verdict=%+v", result)
	}
	if !strings.Contains(result.Summary, "<binary-trace-path>") || result.Repair.Metadata["command"] != "codrax trace convert --input <binary-trace-path>" {
		t.Fatalf("pathless attachment recovery=%+v", result)
	}
	if entries, readErr := os.ReadDir(dir); readErr != nil || len(entries) != 0 {
		t.Fatalf("binary attachment materialized filesystem state before admission: entries=%v err=%v", entries, readErr)
	}
	if len(ctx.AnalysisIR.RequestModel.RuntimeTargets) != 0 || len(mutable.TraceQueryCallWindows()) != 0 {
		t.Fatalf("binary attachment mutated run registries: targets=%+v windows=%+v", ctx.AnalysisIR.RequestModel.RuntimeTargets, mutable.TraceQueryCallWindows())
	}
}

func TestTraceQueryRejectsDirectOversizedLineBeforeBlobMaterialization(t *testing.T) {
	dir := t.TempDir()
	ctx := &types.BusContext{
		WorkDir:         dir,
		AttachedHitrace: strings.Repeat("x", attachment.TracePhysicalLineMaxBytes+1),
		Mutable:         types.NewMutableState("analyze attached trace"),
		AnalysisIR:      &types.AnalysisIR{},
	}
	params, _ := json.Marshal(map[string]any{"source": "attached_trace", "view": "event_search"})
	result, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success || result.Repair == nil || result.Repair.Code != tracequery.TraceInputAdmissionCodeLineTooLong {
		t.Fatalf("oversized direct line verdict=%+v", result)
	}
	if entries, readErr := os.ReadDir(dir); readErr != nil || len(entries) != 0 {
		t.Fatalf("oversized direct line materialized blob state: entries=%v err=%v", entries, readErr)
	}
}

func TestTraceQueryConversionRecoveryKeepsHostilePathOutOfShellCommand(t *testing.T) {
	path := "/tmp/customer $(touch pwn) `id` ' quote\ntrace.sys"
	result := traceQueryInputAdmissionFailure(path, &tracequery.TraceInputAdmissionError{
		Code:   tracequery.TraceInputAdmissionCodeConversionRequired,
		Path:   path,
		Reason: "contains NUL bytes",
	})
	if result.Repair == nil {
		t.Fatal("missing repair metadata")
	}
	command := result.Repair.Metadata["command"]
	if command != "codrax trace convert --input <binary-trace-path>" || strings.Contains(command, "touch pwn") || strings.Contains(command, "`id`") {
		t.Fatalf("untrusted path entered copyable shell command: %q", command)
	}
	var argv []string
	if err := json.Unmarshal([]byte(result.Repair.Metadata["argv_json"]), &argv); err != nil {
		t.Fatalf("argv_json is not structured JSON: %v", err)
	}
	if len(argv) != 5 || argv[4] != path {
		t.Fatalf("argv_json lost exact path: %q", argv)
	}
	if !strings.Contains(result.Summary, strconv.Quote(path)) {
		t.Fatalf("diagnostic did not separately disclose escaped path: %s", result.Summary)
	}
}

func TestTraceQueryGenericContainerDoesNotAdvertiseConvertCommand(t *testing.T) {
	result := traceQueryInputAdmissionFailure("archive.zip", &tracequery.TraceInputAdmissionError{
		Code:   tracequery.TraceInputAdmissionCodeTextExportRequired,
		Path:   "archive.zip",
		Reason: "known binary trace format: zip",
	})
	if result.Repair == nil || result.Repair.Code != tracequery.TraceInputAdmissionCodeTextExportRequired {
		t.Fatalf("generic container verdict=%+v", result)
	}
	if result.Repair.Metadata["command"] != "" || result.Repair.Metadata["argv_json"] != "" || strings.Contains(result.Summary, "trace convert --input") {
		t.Fatalf("unsupported container advertised direct conversion: %+v", result)
	}
}

func TestTraceQuerySchemaSaysBinaryConversionPrecedesInvestigation(t *testing.T) {
	schema := string((&TraceQuery{}).Parameters())
	for _, want := range []string{"recognized binary/non-text prefix is rejected before any physical trace parser", "codrax trace convert --input <binary-trace-path>"} {
		if !strings.Contains(schema, want) {
			t.Fatalf("trace_query schema missing %q", want)
		}
	}
}

func TestResolveTraceQuerySourceRejectsWindowsNamedPipeBeforeStatOnEveryHost(t *testing.T) {
	for _, raw := range []string{
		`\\.\pipe\customer-trace`,
		`\\?\GLOBALROOT\Device\NamedPipe\customer-trace`,
		`\\server\pipe\customer-trace`,
	} {
		t.Run(raw, func(t *testing.T) {
			path, source, reject := resolveTraceQuerySource(&types.BusContext{RepoRoot: t.TempDir()}, traceQueryParams{
				Source: "path",
				Path:   raw,
			})
			if reject == nil || reject.Success || path != "" || source != "path" {
				t.Fatalf("named-pipe namespace escaped source resolution: path=%q source=%q reject=%+v", path, source, reject)
			}
			if reject.Repair == nil || reject.Repair.Code != tracequery.TraceInputAdmissionCodeSourceUnavailable ||
				reject.Repair.Metadata["stage"] != types.ToolRepairStageTraceInputAdmission ||
				reject.Repair.Metadata["status"] != types.ToolRepairStatusActionRequired {
				t.Fatalf("named-pipe rejection is not an action-required admission repair: %+v", reject.Repair)
			}
			if !strings.Contains(reject.Summary, "rejected before filesystem probing") || !strings.Contains(reject.Summary, strconv.Quote(raw)) {
				t.Fatalf("named-pipe rejection lost precise stage/path: %s", reject.Summary)
			}
		})
	}
}

func TestTraceQueryTypedCandidateCollectorsRejectWindowsPipeBeforeStat(t *testing.T) {
	pipe := `\\.\pipe\typed-trace`
	ctx := &types.BusContext{
		RepoRoot: t.TempDir(),
		RuntimeArtifactPreflight: types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: pipe, Carrier: "request_path"}},
		}),
	}
	if got := attachedTraceQueryReferencedArtifactCandidates(ctx); len(got) != 0 {
		t.Fatalf("typed referenced candidate collector statted a pipe namespace: %+v", got)
	}
	item := types.RuntimeArtifactSelectionItem{Kind: "trace", Source: pipe, Carriers: []string{"request_path"}}
	if got := traceQueryLogicalArtifactCandidates(ctx, item); len(got) != 0 {
		t.Fatalf("logical artifact candidate collector statted a pipe namespace: %+v", got)
	}
}

func TestResolveTraceQuerySourceNamedPipeGuardsStructurallyPrecedeStats(t *testing.T) {
	source, err := os.ReadFile("trace_query.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	start := strings.Index(text, "func resolveTraceQuerySource(")
	if start < 0 {
		t.Fatal("resolveTraceQuerySource function boundaries not found")
	}
	end := strings.Index(text[start:], "func traceQueryPathIsWindowsNamedPipe(")
	if end <= 0 {
		t.Fatal("resolveTraceQuerySource function boundaries not found")
	}
	body := text[start : start+end]
	firstGuard := strings.Index(body, "traceQueryPathIsWindowsNamedPipe(candidate, resolved)")
	firstStat := strings.Index(body, "os.Stat(resolved)")
	if firstGuard < 0 || firstStat < 0 || firstGuard > firstStat {
		t.Fatalf("attached fallback must guard the Windows pipe namespace before stat: guard=%d stat=%d", firstGuard, firstStat)
	}
	remainder := body[firstStat+len("os.Stat(resolved)"):]
	finalGuard := strings.Index(remainder, "traceQueryPathIsWindowsNamedPipe(strings.TrimSpace(p.Path), resolved)")
	finalStat := strings.Index(remainder, "os.Stat(resolved)")
	if finalGuard < 0 || finalStat < 0 || finalGuard > finalStat {
		t.Fatalf("explicit path lane must guard the Windows pipe namespace before stat: guard=%d stat=%d", finalGuard, finalStat)
	}
}

func TestRecipeMarkerScannerCannotReintroduceRawPathRead(t *testing.T) {
	source, err := os.ReadFile("trace_query.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	start := strings.Index(text, "func scanTraceQueryRecipeMarkers(")
	if start < 0 {
		t.Fatal("recipe marker scanner function boundaries not found")
	}
	end := strings.Index(text[start:], "func firstTraceQueryMarkerToken(")
	if end <= 0 {
		t.Fatal("recipe marker scanner function boundaries not found")
	}
	body := text[start : start+end]
	if !strings.Contains(body, "tracequery.StreamAdmittedTraceTextLines(") {
		t.Fatal("recipe marker scanner no longer uses the admitted frozen-line authority")
	}
	for _, forbidden := range []string{"os.Open(", "os.OpenFile(", "ReadString(", "bufio.NewReader"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("recipe marker scanner reintroduced raw path/line bypass %q", forbidden)
		}
	}
}
