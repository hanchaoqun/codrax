package orchestrator

import (
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/repl"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise real Run entry twice on one REPL-style instance. The intercepted
// slash request stops before analyzer dispatch: this test isolates path
// ownership, while SourceTerminalPublic covers the complete controller flow.
func TestGeneratedPlanPathOwnershipAcrossRunEntry(t *testing.T) {
	for _, action := range []string{"automatic", "automatic_store", "explicit_same_path", "explicit_other_path", "explicit_clear", "explicit_mirror"} {
		t.Run(action, func(t *testing.T) {
			ar, sr, sar := buildRegistries(nil)
			o := New(types.PipelineSettings{}, ar, sr, sar)
			o.SetMode(types.ModeApply)
			root := t.TempDir()
			first, err := o.Run("/approve", root, "main")
			if err == nil || first == nil {
				t.Fatal("entry probe must stop before dispatch")
			}
			first.WorkDir = t.TempDir()
			if action == "automatic_store" {
				o.SetPlanSaver(repl.NewPlanStore(filepath.Join(t.TempDir(), "plans")))
			}
			first.Mutable.SetChangePlan(&types.ChangePlan{ID: "generated-source", Status: types.PlanStatusApplied, Request: "change a document", Summary: "document update",
				Changes: []types.FileChange{{Path: "README.md", Kind: "add", NewContent: "document\n"}}, TargetPaths: []string{"README.md"},
			})
			o.persistCurrentChangePlanSnapshot()
			generated := o.PlanPath()
			if generated == "" || first.PlanPath != generated {
				t.Fatal("actual persistence did not publish its output path")
			}
			if _, err := types.LoadChangePlanFromFile(generated); err != nil {
				t.Fatalf("first Run output unavailable before second entry: %v", err)
			}
			want := ""
			switch action {
			case "explicit_same_path":
				want = generated
				o.SetPlanPath(want)
			case "explicit_other_path":
				want = filepath.Join(t.TempDir(), "input.json")
				o.SetPlanPath(want)
			case "explicit_clear":
				o.SetPlanPath("")
			case "explicit_mirror":
				want = filepath.Join(t.TempDir(), "input.json")
				o.mirrorActivePlanToImportFile(want)
			}
			second, err := o.Run("/approve", root, "main")
			if err == nil || second == nil {
				t.Fatal("second entry probe must stop before dispatch")
			}
			if second.PlanPath != want || o.PlanPath() != want {
				t.Fatalf("output was reimported or explicit input lost: bus=%q getter=%q want=%q", second.PlanPath, o.PlanPath(), want)
			}
			retained, err := types.LoadChangePlanFromFile(generated)
			if err != nil || retained.ID != "generated-source" {
				t.Fatalf("Run entry changed or removed the previous durable output: %v %+v", err, retained)
			}
		})
	}
}
