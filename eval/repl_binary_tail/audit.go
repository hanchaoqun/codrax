package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/outputdump"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type runReceipt struct {
	Status         string            `json:"status"`
	HumanAudit     string            `json:"human_audit"`
	Checks         map[string]bool   `json:"checks"`
	ExecutionError string            `json:"execution_error,omitempty"`
	ElapsedSeconds float64           `json:"elapsed_seconds"`
	Evidence       map[string]string `json:"evidence"`
	Artifacts      []auditArtifact   `json:"artifacts"`
	Errors         []string          `json:"errors,omitempty"`
}

type auditArtifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type preparation struct {
	Version      string `json:"version"`
	SourcePath   string `json:"source_path"`
	SourceSHA256 string `json:"source_sha256"`
	QueryPath    string `json:"query_path"`
}

type snapshot struct {
	Trace struct {
		SchemaVersion int    `json:"schema_version"`
		Kind          string `json:"kind"`
		Path          string `json:"path"`
		Bytes         int    `json:"bytes"`
		SHA256        string `json:"sha256"`
		Original      string `json:"trace_original_path"`
		Reattach      bool   `json:"trace_requires_reattach"`
	} `json:"trace"`
}

func (r *runReceipt) finish() {
	r.Status = "machine_checks_pass_human_pending"
	r.HumanAudit = "pending_full_answer_and_process_review"
	for _, ok := range r.Checks {
		if !ok {
			r.Status = "machine_checks_failed"
		}
	}
	if len(r.Errors) > 0 {
		r.Status = "machine_checks_failed"
	}
}

// Audit only artifacts authored by the actual child in workspace/logs.
// The operator oracle directory is deliberately outside this traversal.
func auditRun(out string, oracle fixtureOracle) runReceipt {
	r := runReceipt{Checks: map[string]bool{
		"cli_exit_zero": false, "input_unchanged": false, "repl_snapshot_preview_excludes_tail": false,
		"child_preparation_bound_to_original": false, "native_query_contains_exact_tail": false,
		"model_and_tool_telemetry": false, "answer_surface_bound_to_markdown": false,
		"answer_mentions_expected_tuple": false,
	}, Evidence: map[string]string{}}
	input := filepath.Join(out, "input", "capture.sys")
	sha, err := fileSHA(input)
	r.Checks["input_unchanged"] = err == nil && sha == oracle.InputSHA256
	workspace := filepath.Join(out, "workspace")
	var files []string
	for _, root := range []string{workspace, filepath.Join(out, "logs")} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.Type().IsRegular() {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			r.Errors = append(r.Errors, err.Error())
		}
	}
	sort.Strings(files)
	prepared := map[string]string{}
	for _, path := range files {
		sha, err := fileSHA(path)
		if err != nil {
			r.Errors = append(r.Errors, err.Error())
			continue
		}
		rel, _ := filepath.Rel(out, path)
		r.Artifacts = append(r.Artifacts, auditArtifact{rel, sha})
		if filepath.Base(path) != "preparation.json" {
			continue
		}
		var p preparation
		if readJSON(path, &p) && p.Version == "traceinput-v1" && p.SourcePath == input && p.SourceSHA256 == oracle.InputSHA256 && within(workspace, p.QueryPath) {
			if info, err := os.Stat(p.QueryPath); err == nil && info.Mode().IsRegular() {
				prepared[p.QueryPath] = path
				r.Checks["child_preparation_bound_to_original"] = true
				r.Evidence["preparation"] = rel
			}
		}
	}
	var model, queryCall bool
	modelLine := regexp.MustCompile(`(?m)^\d{4}-\d\d-\d\dT[^\n]+ (?:DEBUG|INFO) \[diag [^\]]+\] [^\n]*phase=llm_request [^\n]*done elapsed=`)
	queryLine := regexp.MustCompile(`(?m)^\d{4}-\d\d-\d\dT[^\n]+ (?:DEBUG|INFO) \[diag [^\]]+\] [^\n]*phase=toolcall [^\n]*tool=trace_query params=`)
	for _, path := range files {
		name := filepath.Base(path)
		rel, _ := filepath.Rel(out, path)
		switch {
		case name == "latest.json":
			var s snapshot
			if !readJSON(path, &s) || s.Trace.SchemaVersion != 2 || s.Trace.Kind != "trace" || !s.Trace.Reattach || s.Trace.Original != input || !within(workspace, s.Trace.Path) {
				continue
			}
			preview, err := os.ReadFile(s.Trace.Path)
			if err != nil || len(preview) == 0 || len(preview) > previewBytes || len(preview) != s.Trace.Bytes || bytesSHA(preview) != s.Trace.SHA256 || strings.Contains(string(preview), oracle.ExpectedName) || strings.Contains(string(preview), "424242") || !strings.Contains(string(preview), "# codrax-preview: truncated; full query material retained") {
				continue
			}
			for query, receipt := range prepared {
				if strings.Contains(string(preview), "# codrax-source: "+query+"\n") && strings.Contains(string(preview), "# codrax-conversion-receipt: "+receipt+"\n") {
					r.Checks["repl_snapshot_preview_excludes_tail"] = true
					r.Evidence["repl_snapshot"] = rel
				}
			}
		case strings.HasPrefix(name, "trace-query-result-") && strings.HasSuffix(name, ".json"):
			var result tracequery.Result
			if !readJSON(path, &result) || prepared[result.SourcePath] == "" {
				continue
			}
			for _, event := range result.Events {
				if event.Type == tracequery.EventSchedWakeup && event.Ts == oracle.ExpectedTime && event.WakeePID == oracle.ExpectedPID && event.WakeeComm == oracle.ExpectedName {
					r.Checks["native_query_contains_exact_tail"] = true
					r.Evidence["tail_query"] = rel
				}
			}
		case strings.HasSuffix(name, outputdump.AnswerSurfacesExt):
			var surface outputdump.AnswerSurfaceArtifact
			if !readJSON(path, &surface) || surface.SchemaVersion != 1 || surface.Status != "available" || surface.Primary == "" || surface.Principal == "" {
				continue
			}
			md := strings.TrimSuffix(path, outputdump.AnswerSurfacesExt) + ".md"
			sha, err := fileSHA(md)
			if err != nil || sha != surface.MarkdownSHA256 {
				continue
			}
			r.Checks["answer_surface_bound_to_markdown"] = true
			r.Evidence["answer_surfaces"] = rel
			// Only a smoke oracle. Wrong relationships, extra rows and causal
			// overclaims remain mandatory human checks, never a claimed PASS.
			if strings.Contains(surface.Primary, oracle.ExpectedName) && strings.Contains(surface.Primary, "424242") && strings.Contains(surface.Primary, "1.011") {
				r.Checks["answer_mentions_expected_tuple"] = true
			}
		case strings.HasSuffix(name, ".log"):
			data, err := os.ReadFile(path)
			if err == nil {
				model = model || modelLine.Match(data)
				queryCall = queryCall || queryLine.Match(data)
			}
		}
	}
	r.Checks["model_and_tool_telemetry"] = model && queryCall
	return r
}

func within(root, path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, canonical)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func readJSON(path string, value any) bool {
	data, err := os.ReadFile(path)
	return err == nil && json.Unmarshal(data, value) == nil
}
