package repl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func contextIdentityRun234() *types.WriteWorkflowRun {
	const batch = "source-proof-repair-proof-probe-plan"
	run := &types.WriteWorkflowRun{RunID: "native-live-source-terminal", ActiveBatchID: batch,
		Batches: []types.WriteWorkflowBatch{{ID: batch}}}
	for i, n := range []int{3, 11} {
		name, stage := "planner-observations-planner-observation", "planner_observation"
		if i == 1 {
			name, stage = "merged", ""
		}
		pack := types.WriteContextPack{PackID: "native-live-source-terminal-" + batch + "-" + name, BatchID: batch, SourceStage: stage}
		for j := 0; j < n; j++ {
			pack.Items = append(pack.Items, types.WriteContextItem{ID: fmt.Sprintf("%d-%d", i, j), Priority: types.WriteContextP1, Kind: "evidence_ref", Text: fmt.Sprintf("source-%d.go:%d", i, j+1)})
		}
		run.ContextPacks = append(run.ContextPacks, pack)
	}
	return run
}

func TestContextPack234SaveLoadIdentity(t *testing.T) {
	store := NewWriteWorkflowRunStore(t.TempDir())
	run := contextIdentityRun234()
	wantIDs := []string{run.ContextPacks[0].PackID, run.ContextPacks[1].PackID}
	var refs []string
	for round := 0; round < 5; round++ {
		if _, err := store.Save(run); err != nil {
			t.Fatal(err)
		}
		var err error
		run, err = store.Load(run.RunID)
		if err != nil {
			t.Fatal(err)
		}
		if got := []string{run.ContextPacks[0].PackID, run.ContextPacks[1].PackID}; !reflect.DeepEqual(got, wantIDs) {
			t.Errorf("round %d mutated logical IDs: got %q want %q", round, got, wantIDs)
		}
		if len(run.Batches[0].ContextPackIDs) != 2 {
			t.Errorf("round %d refs=%v", round, run.Batches[0].ContextPackIDs)
		}
		if round == 0 {
			refs = append([]string(nil), run.Batches[0].ContextPackIDs...)
		} else if !reflect.DeepEqual(refs, run.Batches[0].ContextPackIDs) {
			t.Errorf("round %d refs changed", round)
		}
		files, err := os.ReadDir(filepath.Join(store.WorkflowDir(), "contexts", run.RunID))
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 2 {
			t.Errorf("round %d file count=%d", round, len(files))
		}
		for i, n := range []int{3, 11} {
			if len(run.ContextPacks[i].Items) != n {
				t.Fatalf("round %d pack %d lost items", round, i)
			}
			artifact, err := types.LoadWriteContextPackFromFile(filepath.Join(store.WorkflowDir(), "contexts", run.RunID, run.ContextPacks[i].ArtifactID+".json"))
			if err != nil || artifact == nil || !reflect.DeepEqual(*artifact, run.ContextPacks[i]) {
				t.Fatalf("round %d pack %d external body differs: %v", round, i, err)
			}
		}
	}
}

func contextSnapshot234(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func saveLoadContext234(t *testing.T, store *WriteWorkflowRunStore, run *types.WriteWorkflowRun) *types.WriteWorkflowRun {
	t.Helper()
	before := string(contextSnapshot234(t, run))
	if _, err := store.Save(run); err != nil {
		t.Fatal(err)
	}
	if string(contextSnapshot234(t, run)) != before {
		t.Fatal("Save mutated caller")
	}
	loaded, err := store.Load(run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func TestContextPack234OrderContentAndCaller(t *testing.T) {
	store := NewWriteWorkflowRunStore(t.TempDir())
	run := contextIdentityRun234()
	a := saveLoadContext234(t, store, run)
	b := saveLoadContext234(t, store, run) // saving the original memory again is also idempotent
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same input changed persistence")
	}
	b.ContextPacks[0], b.ContextPacks[1] = b.ContextPacks[1], b.ContextPacks[0]
	b = saveLoadContext234(t, store, b)
	if b.ContextPacks[1].ArtifactID != a.ContextPacks[0].ArtifactID || b.ContextPacks[0].ArtifactID != a.ContextPacks[1].ArtifactID {
		t.Fatal("pack order changed identity")
	}
	old := b.ContextPacks[0].ArtifactID
	b.Batches[0].Slices = []types.WriteWorkflowSlice{{ID: "slice", ContextPackIDs: []string{old, "unknown-legacy"}}}
	b.ContextPacks[0].Items = append(b.ContextPacks[0].Items, types.WriteContextItem{ID: "extra", Text: "new evidence", Kind: "evidence_ref", Priority: types.WriteContextP1})
	c := saveLoadContext234(t, store, b)
	updated := c.ContextPacks[0].ArtifactID
	if updated == old || len(c.ContextPacks[0].Items) != 12 {
		t.Fatal("content revision not distinguished")
	}
	if len(c.Batches[0].ContextPackIDs) != 2 || !reflect.DeepEqual(c.Batches[0].Slices[0].ContextPackIDs, []string{updated, "unknown-legacy"}) {
		t.Fatalf("refs accumulated or unknown ref removed: %+v", c.Batches[0])
	}
	oldPack, err := types.LoadWriteContextPackFromFile(filepath.Join(store.WorkflowDir(), "contexts", c.RunID, old+".json"))
	if err != nil || oldPack == nil || len(oldPack.Items) != 11 {
		t.Fatal("old revision overwritten")
	}
}

func TestContextPack234LegacyAliasesAndAmbiguity(t *testing.T) {
	for _, legacy := range []string{"old-short", "native-live-source-terminal-source-proof-repair-proof-probe-plan-native-live-source-terminal-source-proof-repair-proof-p", "Case", "../not-a-filename"} {
		t.Run(legacy, func(t *testing.T) {
			run := contextIdentityRun234()
			for i := range run.ContextPacks {
				run.ContextPacks[i].PackID = legacy
			}
			run.Batches[0].ContextPackIDs = []string{legacy, "unknown-legacy"}
			run.Batches[0].Slices = []types.WriteWorkflowSlice{{ID: "slice", ContextPackIDs: []string{legacy}}}
			planned := types.NormalizeWriteWorkflowRun(*run)
			artifacts, diagnostics, err := planWorkflowContextArtifacts(&planned)
			if err != nil || len(artifacts) != 2 || len(diagnostics) != 1 || diagnostics[0].Ref != legacy || len(diagnostics[0].Artifacts) != 2 {
				t.Fatalf("ambiguous alias not diagnosed: %+v %v", diagnostics, err)
			}
			store := NewWriteWorkflowRunStore(t.TempDir())
			// Seed an actual old workflow with no ArtifactID, not a v2 save
			// pretending to be a legacy load. Embedded facts remain authoritative
			// when the old shared filename could only retain one of the two packs.
			if err := types.WriteWorkflowRunToFile(run, filepath.Join(store.WorkflowDir(), run.RunID+".json")); err != nil {
				t.Fatal(err)
			}
			var oldPath string
			var oldBytes []byte
			if !strings.Contains(legacy, "/") {
				oldPath = filepath.Join(store.WorkflowDir(), "contexts", run.RunID, legacy+".json")
				if err := types.WriteContextPackToFile(&run.ContextPacks[1], oldPath); err != nil {
					t.Fatal(err)
				}
				oldBytes, err = os.ReadFile(oldPath)
				if err != nil {
					t.Fatal(err)
				}
			}
			run, err = store.Load(run.RunID)
			if err != nil {
				t.Fatal(err)
			}
			for round := 0; round < 5; round++ {
				run = saveLoadContext234(t, store, run)
				if len(run.Batches[0].ContextPackIDs) != 3 || len(run.Batches[0].Slices[0].ContextPackIDs) != 2 {
					t.Fatalf("refs lost or grew: %+v", run.Batches[0])
				}
				if run.ContextPacks[0].PackID != legacy || run.ContextPacks[1].PackID != legacy || run.ContextPacks[0].ArtifactID == run.ContextPacks[1].ArtifactID {
					t.Fatal("legacy logical identity guessed or artifacts collided")
				}
			}
			if oldPath != "" {
				after, err := os.ReadFile(oldPath)
				if err != nil || string(after) != string(oldBytes) {
					t.Fatal("legacy external artifact was overwritten")
				}
			}
		})
	}
}

func TestContextPack234LongBatchStoreReachability(t *testing.T) {
	for name, scope := range map[string][2]string{
		"long":                {strings.Repeat("proof-followup-", 30) + "target", strings.Repeat("slice-", 50) + "target"},
		"workflow whitespace": {"a  b", "slice \t target"},
	} {
		t.Run(name, func(t *testing.T) {
			run := contextIdentityRun234()
			batch, slice := scope[0], scope[1]
			wantBatch, wantSlice := strings.Join(strings.Fields(batch), " "), strings.Join(strings.Fields(slice), " ")
			run.ActiveBatchID, run.Batches[0].ID = batch, batch
			for i := range run.ContextPacks {
				run.ContextPacks[i] = run.ContextPacks[i].WithScope(batch, slice)
			}
			run.Batches[0].ActiveSliceID = slice
			run.Batches[0].Slices = []types.WriteWorkflowSlice{{ID: slice, ContextPackIDs: []string{run.ContextPacks[0].PackID}}}
			run = saveLoadContext234(t, NewWriteWorkflowRunStore(t.TempDir()), run)
			if run.Batches[0].ID != wantBatch || run.Batches[0].Slices[0].ID != wantSlice || len(run.Batches[0].ContextPackIDs) != 2 {
				t.Fatal("workflow/store scope differs")
			}
			packs := writeWorkflowPacksForBatch(*run, wantBatch)
			if len(packs) != 2 {
				t.Fatal("batch packs not reachable")
			}
			for i, pack := range packs {
				if got := pack.ViewForScope(types.WriteConsumerPlanner, 20, batch, slice); len(got.Items) != []int{3, 11}[i] {
					t.Fatal("scope consumer lost facts")
				}
			}
		})
	}
}

func TestContextPack234ExactIdentityCoordinates(t *testing.T) {
	ids := []string{"Case", "case", "a_b", "a-b", "a  b", "a b", strings.Repeat("同", 250) + "甲", strings.Repeat("同", 250) + "乙"}
	run := contextIdentityRun234()
	run.ContextPacks = nil
	for _, id := range ids {
		run.ContextPacks = append(run.ContextPacks, types.WriteContextPack{PackID: id, BatchID: run.ActiveBatchID, Items: []types.WriteContextItem{{ID: id, SourceID: id, Text: "fact", Priority: types.WriteContextP1}}})
	}
	run.Batches[0].ContextPackIDs = append([]string(nil), ids...)
	store := NewWriteWorkflowRunStore(t.TempDir())
	run = saveLoadContext234(t, store, run)
	seen := map[string]bool{}
	for i, pack := range run.ContextPacks {
		if pack.PackID != ids[i] || pack.Items[0].ID != ids[i] || pack.Items[0].SourceID != ids[i] {
			t.Fatal("identity was display-normalized")
		}
		if seen[pack.ArtifactID] || len(pack.ArtifactID) != 71 {
			t.Fatal("identity collided or filename not bounded")
		}
		seen[pack.ArtifactID] = true
	}
	if len(run.Batches[0].ContextPackIDs) != len(ids) {
		t.Fatal("case-sensitive legacy aliases were lost")
	}
}

func TestContextPack234FailurePreservesPublishedRun(t *testing.T) {
	for _, failure := range []string{"conflicting_artifact", "unwritable_context_path", "workflow_publish"} {
		t.Run(failure, func(t *testing.T) {
			store := NewWriteWorkflowRunStore(t.TempDir())
			run := saveLoadContext234(t, store, contextIdentityRun234())
			path := filepath.Join(store.WorkflowDir(), run.RunID+".json")
			published, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			run.ContextPacks[0].Items[0].Text = "updated evidence"
			before := string(contextSnapshot234(t, run))
			pack := types.NormalizeWriteContextPack(run.ContextPacks[0])
			id, err := workflowContextPackArtifactID(run.RunID, pack)
			if err != nil {
				t.Fatal(err)
			}
			artifactPath := filepath.Join(store.WorkflowDir(), "contexts", run.RunID, id+".json")
			switch failure {
			case "conflicting_artifact":
				if err := os.WriteFile(artifactPath, []byte("conflicting bytes"), 0600); err != nil {
					t.Fatal(err)
				}
			case "unwritable_context_path":
				if err := os.Mkdir(artifactPath, 0700); err != nil {
					t.Fatal(err)
				}
			case "workflow_publish":
				// Existing AtomicWriteFileSync uses an adjacent .tmp. A directory
				// at that path fails before replacing the published run.
				if err := os.Mkdir(path+".tmp", 0700); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.Save(run); err == nil {
				t.Fatal("expected storage failure")
			}
			if before != string(contextSnapshot234(t, run)) {
				t.Fatal("failure mutated caller")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != string(published) {
				t.Fatal("failure changed published workflow")
			}
			if failure == "conflicting_artifact" {
				body, err := os.ReadFile(artifactPath)
				if err != nil || string(body) != "conflicting bytes" {
					t.Fatal("conflict was overwritten")
				}
			}
		})
	}
}
