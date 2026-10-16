package orchestrator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Real Run owns the blob lifecycle; the stub agent calls the public local
// tools, not a mock export. No model invocation or success of a diagnosis is
// claimed by this fixture.
func TestHMC252ManagedExportSurvivesHostCleanup(t *testing.T) {
	for _, mode := range []string{"legacy", "session"} {
		t.Run(mode, func(t *testing.T) {
			repo, anchor := t.TempDir(), t.TempDir()
			writeTraceAdmissionRepoSource(t, repo)
			body, err := os.ReadFile("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(repo, "capture.data"), body, 0600); err != nil {
				t.Fatal(err)
			}
			var receipt tracecatalog.ExportReceipt
			var calls int
			fns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
				types.AgentAnalyzer: func(ac *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					if calls == 0 {
						bus := types.ToolBusContext(ac, types.AgentAnalyzer)
						r, err := (&tool.TraceCatalog{}).Execute(bus, []byte(`{"queries":[{"view":"resource_stack","pid":101,"time_start":10,"time_end":10.05}]}`))
						if err != nil || !r.Success {
							t.Fatalf("discover: %v %s", err, r.Summary)
						}
						c := bus.Mutable.TraceCatalogs()[0]
						for _, q := range c.Snapshot().Queries {
							args, _ := json.Marshal(map[string]any{"catalog_query_ref": map[string]string{"catalog_id": c.ID(), "query_id": q.ID}})
							r, err = (&tool.TraceQuery{}).Execute(bus, args)
							if err != nil || !r.Success {
								t.Fatalf("query: %v %s", err, r.Summary)
							}
						}
						args, _ := json.Marshal(map[string]any{"action": "export", "catalog_id": c.ID(), "export": map[string]string{"target": "capture-review"}})
						r, err = (&tool.TraceCatalog{}).Execute(bus, args)
						if err != nil || !r.Success {
							t.Fatalf("export: %v %s", err, r.Summary)
						}
						if err := json.Unmarshal([]byte(r.Summary), &receipt); err != nil {
							t.Fatal(err)
						}
					}
					calls++
					return &agent.StageOutput{Error: "bounded lifecycle fixture stops after actual export"}, nil
				},
			}
			ar, sr, sar := buildRegistries(fns)
			o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
			o.SetTraceRuntimeAnchor(anchor)
			blobBase := filepath.Join(anchor, "blob")
			if mode == "session" {
				session := filepath.Join(blobBase, "20000101-000000-000-2147483647")
				if err := os.MkdirAll(session, 0700); err != nil {
					t.Fatal(err)
				}
				o.SetBlobSessionDir(session)
			}
			var paths []string
			for run := 0; run < 2; run++ {
				calls = 0
				bus, _ := o.Run("Export the current capture evidence as capture-review", repo, "main")
				if bus == nil || !receipt.PhysicalSuccess || !receipt.MaterialComplete {
					t.Fatalf("missing actual publication: %+v", receipt)
				}
				if mode == "session" && run == 1 {
					if err := os.MkdirAll(filepath.Join(blobBase, "20000102-000000-000-2147483646"), 0700); err != nil {
						t.Fatal(err)
					}
					tool.PruneBlobSessions(blobBase, 1)
				}
				if mode == "legacy" || run == 1 {
					if _, err := os.Stat(bus.WorkDir); !os.IsNotExist(err) {
						t.Fatalf("fixture did not exercise %s cleanup: %v", mode, err)
					}
				}
				paths = append(paths, receipt.ManifestPath)
				assertHMC252PackageReadable(t, receipt)
				if strings.HasPrefix(receipt.ManifestPath, bus.WorkDir+string(os.PathSeparator)) {
					t.Fatal("delivery remains in disposable query storage")
				}
			}
			if paths[0] == paths[1] {
				t.Fatal("same target across Runs adopted the previous delivery")
			}
			for _, path := range paths {
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("previous Run delivery lost: %v", err)
				}
			}
		})
	}
}

func assertHMC252PackageReadable(t *testing.T, r tracecatalog.ExportReceipt) {
	t.Helper()
	data, err := os.ReadFile(r.ManifestPath)
	if err != nil {
		t.Fatalf("published manifest lost after host cleanup: %v", err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != r.ManifestSHA256 {
		t.Fatal("manifest version changed")
	}
	var m tracecatalog.ExportManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	for _, f := range m.Files {
		b, err := os.ReadFile(filepath.Join(filepath.Dir(r.ManifestPath), f.Path))
		if err != nil {
			t.Fatal(err)
		}
		s := sha256.Sum256(b)
		if hex.EncodeToString(s[:]) != f.SHA256 || int64(len(b)) != f.Bytes {
			t.Fatalf("package member changed: %s", f.Path)
		}
	}
	if _, err := os.ReadFile(r.ReportPath); err != nil {
		t.Fatal(err)
	}
}
