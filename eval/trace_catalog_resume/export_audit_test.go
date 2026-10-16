package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func prepareExportAudit(t *testing.T) (exportAuditInputs, tracecatalog.ExportManifest) {
	t.Helper()
	out := filepath.Join(t.TempDir(), "case")
	seed, err := prepareScenario(context.Background(), out, "../fixtures/hmosperf_catalog_objects", "capture-review")
	if err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{Ctx: context.Background(), RepoRoot: seed.Workspace, WorkDir: filepath.Join(out, "current"), DeliveryRoot: tracecatalog.NewManagedExportRoot(out), Mutable: types.NewMutableState(question),
		TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(out, "current", "prepared")})}
	call := func(args map[string]any) types.ToolResult {
		raw, _ := json.Marshal(args)
		result, err := (&tool.TraceCatalog{}).Execute(bus, raw)
		if err != nil || !result.Success {
			t.Fatalf("public catalog: %v %s", err, result.Summary)
		}
		return result
	}
	call(map[string]any{"action": "resume", "root": "captures", "snapshot_path": "checkpoint.json"})
	catalog := bus.Mutable.TraceCatalogs()[0]
	for _, q := range catalog.Snapshot().Queries {
		raw, _ := json.Marshal(map[string]any{"catalog_query_ref": map[string]any{"catalog_id": catalog.ID(), "query_id": q.ID}})
		result, err := (&tool.TraceQuery{}).Execute(bus, raw)
		if err != nil || !result.Success {
			t.Fatalf("public query: %v %s", err, result.Summary)
		}
	}
	status := call(map[string]any{"action": "status", "catalog_id": catalog.ID()})
	var page struct {
		Index string `json:"saved_index"`
	}
	if err := json.Unmarshal([]byte(status.Summary), &page); err != nil {
		t.Fatal(err)
	}
	result := call(map[string]any{"action": "export", "catalog_id": catalog.ID(), "export": map[string]any{"target": "capture-review"}})
	var receipt tracecatalog.ExportReceipt
	if err := json.Unmarshal([]byte(result.Summary), &receipt); err != nil {
		t.Fatal(err)
	}
	in := exportAuditInputs{Out: out, Index: page.Index, Receipt: filepath.Join(out, "actual-tool-result.json"), Process: filepath.Join(out, "test-process.json"), Answer: filepath.Join(out, "default-answer.md"), PriorReport: filepath.Join(out, "prior-report.md"), Report: filepath.Join(out, "explicit-report.md")}
	if err := writeJSON(in.Receipt, result); err != nil {
		t.Fatal(err)
	}
	prior := []byte("# Prior report\n")
	answer := []byte(fmt.Sprintf("# Test-only final answer fixture\n\n[Execution](<%s>) [Manifest](<%s>)\n", receipt.ReportPath, receipt.ManifestPath))
	for path, body := range map[string][]byte{in.PriorReport: prior, in.Answer: answer, in.Report: answer} {
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	zero := 0
	process := auditProcess{Mode: "live", Started: time.Now().Add(-time.Second).Format(time.RFC3339Nano), Finished: time.Now().Format(time.RFC3339Nano), ExitCode: &zero,
		Args: []string{"test-fixture-not-a-live-model", "--repo", seed.Workspace, "--request", seed.Question, "--report-md", in.Report}, PriorReportSHA256: hash(prior)}
	if err := writeJSON(in.Process, process); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(receipt.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest tracecatalog.ExportManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	return in, manifest
}

func TestExportAuditPublicPackageAndExplicitReportBytes(t *testing.T) {
	in, _ := prepareExportAudit(t)
	a, err := inspectExport(context.Background(), in)
	if err != nil || a.Status != "physical_delivery_smoke_pass_human_pending" {
		t.Fatalf("audit: %v %+v", err, a)
	}
	if a.Checks["explicit_report_bytes"].Status != "verified" || len(a.OutputFiles) < 2 || a.InputSHA[in.Report] != a.InputSHA[in.Answer] {
		t.Fatalf("missing physical checks: %+v", a)
	}
	if err := auditExport(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if err := auditExport(context.Background(), in); err == nil {
		t.Fatal("overwrote prior audit")
	}
	// No-report natural lane remains independently useful.
	in.PriorReport, in.Report = "", ""
	a, err = inspectExport(context.Background(), in)
	if err != nil || a.Status != "physical_delivery_smoke_pass_human_pending" || a.Checks["explicit_report_bytes"].Status != "not_applicable" {
		t.Fatalf("ordinary delivery: %v %+v", err, a)
	}
}

func TestExportAuditDoesNotSignIncompleteProcessLinksOrReport(t *testing.T) {
	in, _ := prepareExportAudit(t)
	tests := []struct {
		name, path, contents, axis string
	}{
		{"interrupted", in.Process, `{"mode":"live","exit_code":null}`, "formal_process"},
		{"echo_only", in.Answer, "User requested capture-review, completed.", "post_return_delivery_links"},
		{"old_report", in.Report, "# Prior report\n", "explicit_report_bytes"},
		{"wrong_prior", in.PriorReport, "different old bytes", "explicit_report_bytes"},
		{"wrong_index", in.Index, `{}`, "current_fixture_execution"},
		{"failed_tool", in.Receipt, `{"ToolName":"trace_catalog","Success":false,"Summary":"not exported"}`, "package_integrity"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original, err := os.ReadFile(tt.path)
			if err != nil {
				t.Fatal(err)
			}
			defer os.WriteFile(tt.path, original, 0600)
			if err := os.WriteFile(tt.path, []byte(tt.contents), 0600); err != nil {
				t.Fatal(err)
			}
			a, err := inspectExport(context.Background(), in)
			if err != nil || a.Status != "physical_delivery_smoke_incomplete_human_pending" || a.Checks[tt.axis].Status != "not_verified" {
				t.Fatalf("negative was signed complete: %v %+v", err, a)
			}
		})
	}
	answer, err := os.ReadFile(in.Answer)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(in.Answer, append(append([]byte("```markdown\n"), answer...), []byte("\n```\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := inspectExport(context.Background(), in)
	if err != nil || a.Checks["post_return_delivery_links"].Status != "not_verified" {
		t.Fatalf("fenced example counted as delivery link: %v %+v", err, a)
	}
}

func TestExportAuditRejectsChangedOrRedirectedPackageMembers(t *testing.T) {
	in, m := prepareExportAudit(t)
	body, _ := os.ReadFile(in.Receipt)
	r, err := decodeAuditExportReceipt(body)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(filepath.Dir(r.ManifestPath), m.Files[0].Path)
	original, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := inspectExportPackage(r, "capture-review"); err == nil {
		t.Fatal("tampered member passed")
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(in.Out, "same-bytes-outside")
	if err := os.WriteFile(outside, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, file); err != nil {
		t.Fatal(err)
	}
	if _, _, err := inspectExportPackage(r, "capture-review"); err == nil {
		t.Fatal("same-byte redirected member passed")
	}
	for _, name := range []string{"../escape", "/absolute", "results/../report.md", "results\\foreign"} {
		if err := plainPackageFile(in.Out, name); err == nil {
			t.Fatalf("unsafe path accepted: %s", name)
		}
	}
}

func TestExportAuditReferenceIdentityAndHonestIncompleteMaterial(t *testing.T) {
	in, m := prepareExportAudit(t)
	files := map[string]tracecatalog.ExportFile{}
	for _, f := range m.Files {
		files[f.Path] = f
	}
	original, _ := json.Marshal(m)
	t.Run("wrong_attempt", func(t *testing.T) {
		var changed tracecatalog.ExportManifest
		_ = json.Unmarshal(original, &changed)
		changed.References[0].AttemptID = "not-that-attempt"
		if err := inspectExportReferences(changed, files); err == nil {
			t.Fatal("cross-attempt reference accepted")
		}
	})
	t.Run("honest_partial", func(t *testing.T) {
		var changed tracecatalog.ExportManifest
		_ = json.Unmarshal(original, &changed)
		changed.MaterialComplete = false
		changed.MaterialIssues = append(changed.MaterialIssues, tracecatalog.Issue{Code: "test_missing"})
		missing := changed.References[0].File
		for i := range changed.References {
			if changed.References[i].File == missing {
				changed.References[i].Status, changed.References[i].File, changed.References[i].Reason = "missing", "", "test missing member"
			}
		}
		partial := map[string]tracecatalog.ExportFile{}
		for name, file := range files {
			if name != missing {
				partial[name] = file
			}
		}
		if err := inspectExportReferences(changed, partial); err != nil {
			t.Fatalf("honest partial package mislabeled structurally invalid: %v", err)
		}
	})
	current, err := tracecatalog.Load(context.Background(), in.Index)
	if err != nil {
		t.Fatal(err)
	}
	current.Queries[0], current.Queries[1] = current.Queries[1], current.Queries[0]
	if !sameCurrentNavigation(m.Catalog, current) {
		t.Fatal("slice order treated as query identity")
	}
	current.Queries[0].Attempts[0].SourceRevision += "changed"
	if sameCurrentNavigation(m.Catalog, current) {
		t.Fatal("changed source revision passed current match")
	}
}

func TestExportAuditDoesNotAcceptSwappedProducerCopies(t *testing.T) {
	in, m := prepareExportAudit(t)
	body, _ := os.ReadFile(in.Receipt)
	r, err := decodeAuditExportReceipt(body)
	if err != nil {
		t.Fatal(err)
	}
	first := m.References[0].File
	second := ""
	for _, ref := range m.References {
		if ref.File != first {
			second = ref.File
			break
		}
	}
	if second == "" {
		t.Fatal("fixture needs independent producer results")
	}
	for i := range m.References {
		switch m.References[i].File {
		case first:
			m.References[i].File = second
		case second:
			m.References[i].File = first
		}
	}
	changed, _ := json.Marshal(m)
	if err := os.WriteFile(r.ManifestPath, changed, 0600); err != nil {
		t.Fatal(err)
	}
	r.ManifestSHA256 = hash(changed) // Test a structurally self-consistent swap.
	if _, _, err := inspectExportPackage(r, "capture-review"); err == nil {
		t.Fatal("different producer's correctly hashed copy was accepted")
	}
}
