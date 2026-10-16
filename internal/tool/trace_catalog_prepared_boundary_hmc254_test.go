package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc254PreparedMember(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Artifacts []struct{ Path string }
	}
	if err := json.Unmarshal(data, &manifest); err != nil || len(manifest.Artifacts) == 0 {
		t.Fatalf("fixture manifest: %v %s", err, data)
	}
	member := manifest.Artifacts[0].Path
	if !filepath.IsAbs(member) {
		member = filepath.Join(filepath.Dir(path), member)
	}
	return member
}

func hmc254ReplaceSameBytes(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".replacement", data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".replacement", path); err != nil {
		t.Fatal(err)
	}
}

func TestHMC254PreparedQueryOriginCannotBeReconstructedOrPromoted(t *testing.T) {
	for _, mode := range []string{"copied_manifest", "bundle_child", "json_material", "fresh_coordinator", "source_replaced", "manifest_replaced", "member_replaced", "original_not_authorized", "attachment_preview_changed"} {
		t.Run(mode, func(t *testing.T) {
			bus, c := exportCatalogFixture250(t)
			m := bus.TraceInputPreparer.PreparedMaterials()[0]
			queryPath := m.QueryPath()
			switch mode {
			case "copied_manifest":
				body, err := os.ReadFile(queryPath)
				if err != nil {
					t.Fatal(err)
				}
				queryPath = filepath.Join(filepath.Dir(queryPath), "copy.tracebundle.json")
				if err := os.WriteFile(queryPath, body, 0600); err != nil {
					t.Fatal(err)
				}
			case "bundle_child":
				queryPath = hmc254PreparedMember(t, queryPath)
			case "json_material":
				encoded, _ := json.Marshal(m)
				var forged attachment.TraceMaterial
				if err := json.Unmarshal(encoded, &forged); err != nil {
					t.Fatal(err)
				}
				bus.AttachedTraceMaterial, bus.AttachedHitrace, bus.TraceInputPreparer = &forged, m.Preview(), nil
			case "fresh_coordinator":
				bus.TraceInputPreparer = traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: t.TempDir()})
			case "source_replaced":
				hmc254ReplaceSameBytes(t, m.SourcePath())
			case "manifest_replaced":
				hmc254ReplaceSameBytes(t, queryPath)
			case "member_replaced":
				hmc254ReplaceSameBytes(t, hmc254PreparedMember(t, queryPath))
			case "original_not_authorized":
				bus.RepoRoot, bus.UserPinnedFiles = t.TempDir(), nil
			case "attachment_preview_changed":
				bus.AttachedTraceMaterial, bus.AttachedHitrace, bus.TraceInputPreparer = m, "unrelated preview", nil
			}
			before := c.CurrentExecution()
			for range 2 {
				_ = catalogPublicQuery(t, bus, queryPath, 101)
			}
			after := c.CurrentExecution()
			if after.Coverage.Attempts != before.Coverage.Attempts || len(after.Snapshot.Queries) != len(before.Snapshot.Queries) {
				t.Fatalf("%s minted original execution authority: before=%+v after=%+v", mode, before.Coverage, after.Coverage)
			}
		})
	}
}

func TestHMC254PreparedAttachmentKeepsExplicitPathRegistrationAfterProjection(t *testing.T) {
	bus, c := exportCatalogFixture250(t)
	m := bus.TraceInputPreparer.PreparedMaterials()[0]
	bus.AttachedTraceMaterial, bus.AttachedHitrace, bus.TraceInputPreparer = m, m.Preview(), nil
	projected := types.ToolBusContext(promptctx.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore), types.AgentExplorer)
	if out := catalogPublicQuery(t, projected, m.QueryPath(), 101); !out.Success {
		t.Fatalf("attached prepared path failed: %s", out.Summary)
	}
	if c.CurrentExecution().Coverage.Attempts != 5 {
		t.Fatal("host-projected private attachment lost original execution identity")
	}
}

func TestHMC254PreparedQueryKeepsIndependentlyDeclaredManifestCatalog(t *testing.T) {
	bus, original := exportCatalogFixture250(t)
	m := bus.TraceInputPreparer.PreparedMaterials()[0]
	bus.UserPinnedFiles = []string{m.QueryPath()}
	args, _ := json.Marshal(map[string]any{"root": m.QueryPath(), "queries": []map[string]any{{"view": "resource_stack", "pid": 101, "time_start": 10, "time_end": 10.05}}})
	out, err := (&TraceCatalog{}).Execute(bus, args)
	if err != nil || !out.Success {
		t.Fatalf("explicit manifest discovery failed: %v %s", err, out.Summary)
	}
	if out := catalogPublicQuery(t, bus, m.QueryPath(), 101); !out.Success {
		t.Fatalf("manifest query failed: %s", out.Summary)
	}
	if original.CurrentExecution().Coverage.Attempts != 5 {
		t.Fatal("original query missing")
	}
	found := false
	for _, c := range bus.Mutable.TraceCatalogs() {
		if c.ID() == original.ID() {
			continue
		}
		view := c.CurrentExecution()
		if view.Coverage.Attempts != 1 || view.Coverage.DeclaredPlans != 1 || view.Coverage.ObservedQueries != 0 {
			t.Fatalf("private alias erased the separately admitted manifest catalog: %+v", view.Coverage)
		}
		found = true
	}
	if !found {
		t.Fatal("explicit manifest catalog absent")
	}
}
