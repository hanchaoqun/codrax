package orchestrator

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Public apply and planner/read/emit tools produce both plans. Exercise the
// controller's actual mirror boundary with the canonical source-ID filename,
// as used when /approve supplies a PlanStore path rather than a custom alias.
func TestNativeRegistrationExplicitMirrorRetainsSourceArtifact(t *testing.T) {
	for _, entry := range []string{"mirror", "persist"} {
		for _, shape := range []string{"canonical", "parent_segment", "directory_symlink"} {
			t.Run(entry+"/"+shape, func(t *testing.T) { nativeRegistrationMirrorRetainsSource(t, entry, shape) })
		}
	}
}

func nativeRegistrationMirrorRetainsSource(t *testing.T, entry, shape string) {
	f := newControllerRegistrationFixture(t)
	plan := f.register(t, "emit_change_plan")
	o := f.o
	aliasPath := f.sourceRef
	if shape == "parent_segment" {
		aliasPath = filepath.Dir(f.sourceRef) + "/../plans/" + filepath.Base(f.sourceRef)
	}
	if shape == "directory_symlink" {
		link := filepath.Join(t.TempDir(), "linked-plans")
		if err := os.Symlink(filepath.Dir(f.sourceRef), link); err != nil {
			t.Fatal(err)
		}
		aliasPath = filepath.Join(link, filepath.Base(f.sourceRef))
	}
	o.SetPlanPath(aliasPath)
	o.busCtx.PlanPath = aliasPath
	if entry == "mirror" {
		o.mirrorActivePlanToImportFile(aliasPath)
	} else {
		o.persistCurrentChangePlanSnapshot()
	}
	alias, err := types.LoadChangePlanFromFile(f.sourceRef)
	if err != nil || alias.ID != plan.ID || o.PlanPath() != aliasPath {
		t.Fatalf("explicit mirror lost its live result: %v %+v", err, alias)
	}
	retained := o.loadDurablePlanArtifact(f.source.ID)
	if retained == nil || retained.ID != f.source.ID {
		t.Fatalf("explicit source-ID mirror erased the original source artifact: %s", f.sourceRef)
	}
	want, valid := verificationSourcePlanDelivery(f.source)
	got, ok := verificationSourcePlanDelivery(retained)
	if !valid || !ok || !verificationDeliveriesEqual(want, got) {
		t.Fatal("recovered source changed delivery identity")
	}
	finalSource, err := o.loadWriteFinalReportChangePlan(f.source.ID)
	if err != nil || finalSource == nil || finalSource.ID != f.source.ID {
		t.Fatalf("final proof assembly lost the retained source: %v", err)
	}
	if entry == "mirror" && shape == "canonical" {
		f.restart(t) // Fresh process must use the preserved original, not alias.
	}
}

func TestPlanSnapshotPreservationFailureDoesNotOverwrite(t *testing.T) {
	for _, condition := range []string{"unwritable_retention", "retention_alias", "wrong_retained_identity"} {
		t.Run(condition, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "source.json")
			prior := &types.ChangePlan{ID: "source", Request: "update docs", Summary: "source", Status: types.PlanStatusApplied, TargetPaths: []string{"README.md"}, Changes: []types.FileChange{{Path: "README.md", Kind: "add", NewContent: "source\n"}}}
			if err := types.WritePlanToFile(prior, path); err != nil {
				t.Fatal(err)
			}
			retention := filepath.Join(dir, "retained-plans")
			switch condition {
			case "unwritable_retention":
				if err := os.WriteFile(retention, []byte("not a directory"), 0600); err != nil {
					t.Fatal(err)
				}
			case "retention_alias":
				if err := os.Symlink(dir, retention); err != nil {
					t.Fatal(err)
				}
			case "wrong_retained_identity":
				other := *prior
				other.ID = "unrelated"
				if err := types.WritePlanToFile(&other, filepath.Join(retention, "source.json")); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(path)
			mu := types.NewMutableState("replacement")
			next := *prior
			next.ID = "replacement"
			mu.SetChangePlan(&next)
			o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, PlanPath: path}}
			for _, entry := range []string{"mirror", "persist"} {
				if entry == "mirror" {
					o.mirrorActivePlanToImportFile(path)
				} else {
					o.persistCurrentChangePlanSnapshot()
				}
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("%s overwrote source despite failed retention: %v", entry, err)
				}
			}
		})
	}
}

func TestPlanIdentityArtifactRejectsMismatchedFallback(t *testing.T) {
	dir := t.TempDir()
	other := &types.ChangePlan{ID: "other", Request: "update docs", Summary: "other", TargetPaths: []string{"README.md"}, Changes: []types.FileChange{{Path: "README.md", Kind: "add", NewContent: "other\n"}}}
	for _, path := range []string{filepath.Join(dir, "source.json"), retainedPlanIdentityPath(dir, "source")} {
		if err := types.WritePlanToFile(other, path); err != nil {
			t.Fatal(err)
		}
	}
	o := &Orchestrator{reportDir: dir, busCtx: &types.BusContext{Mutable: types.NewMutableState("source lookup")}}
	if got := o.loadDurablePlanArtifact("source"); got != nil {
		t.Fatalf("source restore accepted another identity: %+v", got)
	}
	if got, err := o.loadWriteFinalReportChangePlan("source"); got != nil || err == nil {
		t.Fatalf("final report accepted another identity: %+v %v", got, err)
	}
}

func TestPlanSnapshotPreservesLegacyOriginalBytes(t *testing.T) {
	for _, entry := range []string{"mirror", "persist"} {
		t.Run(entry, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "legacy-proof.json")
			// This supported legacy sentinel is upgraded by WritePlanToFile.
			// Preservation must retain the original, including unknown fields.
			original := []byte(`{
  "id": "legacy-proof",
  "status": "no_change_required",
  "target_paths": ["src/widget.ts"],
  "verification_probes": [{"id":"behavior","language":"javascript","code":"if (!true) throw new Error('failed')"}],
  "future_metadata": {"keep": "verbatim"}
}
`)
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := types.LoadChangePlanFromFile(path); err != nil {
				t.Fatalf("legacy original must already be a valid input: %v", err)
			}
			next := &types.ChangePlan{ID: "successor", Request: "update docs", Summary: "successor", TargetPaths: []string{"README.md"}, Changes: []types.FileChange{{Path: "README.md", Kind: "add", NewContent: "next\n"}}}
			mu := types.NewMutableState("preserve legacy input")
			mu.SetChangePlan(next)
			o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, PlanPath: path}}
			if entry == "mirror" {
				o.mirrorActivePlanToImportFile(path)
			} else {
				o.persistCurrentChangePlanSnapshot()
			}
			alias, err := types.LoadChangePlanFromFile(path)
			if err != nil || alias.ID != next.ID {
				t.Fatalf("valid legacy input prevented alias update: %v %+v", err, alias)
			}
			retained, err := os.ReadFile(retainedPlanIdentityPath(dir, "legacy-proof"))
			if err != nil || !bytes.Equal(original, retained) {
				t.Fatalf("legacy original bytes changed during preservation: %v\n%s", err, retained)
			}
			loaded, err := loadPlanIdentityArtifact(dir, "legacy-proof")
			if err != nil || loaded.ID != "legacy-proof" || len(loaded.VerificationProbes) != 1 {
				t.Fatalf("preserved legacy input is no longer loadable by exact identity: %v %+v", err, loaded)
			}
		})
	}
}
