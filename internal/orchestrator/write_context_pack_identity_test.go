package orchestrator

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/repl"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestContextPack234RestartUpsert(t *testing.T) {
	for _, logicalID := range []string{"merged", "change-plan"} {
		t.Run(logicalID, func(t *testing.T) {
			store := repl.NewWriteWorkflowRunStore(t.TempDir())
			pack := types.WriteContextPack{PackID: logicalID, BatchID: "batch", Items: []types.WriteContextItem{{ID: "first", Text: "first", Priority: types.WriteContextP1}}}
			run := types.WriteWorkflowRun{RunID: "run", Batches: []types.WriteWorkflowBatch{{ID: "batch"}}}
			run = upsertWorkflowRunContextPack(run, pack)
			if _, err := store.Save(&run); err != nil {
				t.Fatal(err)
			}
			loaded, err := store.Load("run")
			if err != nil {
				t.Fatal(err)
			}
			oldArtifact := loaded.ContextPacks[0].ArtifactID
			pack.Items = []types.WriteContextItem{{ID: "second", Text: "second", Priority: types.WriteContextP1}}
			run = upsertWorkflowRunContextPack(*loaded, pack)
			if len(run.ContextPacks) != 1 || run.ContextPacks[0].PackID != logicalID || len(run.ContextPacks[0].Items) != 2 || run.ContextPacks[0].ArtifactID != oldArtifact {
				t.Fatalf("upsert lost logical/storage identity: %+v", run.ContextPacks)
			}
			if _, err := store.Save(&run); err != nil {
				t.Fatal(err)
			}
			loaded, err = store.Load("run")
			if err != nil {
				t.Fatal(err)
			}
			if len(loaded.Batches[0].ContextPackIDs) != 1 || loaded.Batches[0].ContextPackIDs[0] == oldArtifact {
				t.Fatal("upsert accumulated obsolete storage refs")
			}
		})
	}
}

func TestContextPack234LegacyAmbiguousUpsert(t *testing.T) {
	store := repl.NewWriteWorkflowRunStore(t.TempDir())
	run := types.WriteWorkflowRun{RunID: "run", Batches: []types.WriteWorkflowBatch{{ID: "batch", ContextPackIDs: []string{"old-truncated"}}}}
	for _, text := range []string{"first origin", "second origin"} {
		run.ContextPacks = append(run.ContextPacks, types.WriteContextPack{PackID: "old-truncated", BatchID: "batch", Items: []types.WriteContextItem{{ID: "fact", Text: text, Priority: types.WriteContextP1}}})
	}
	if _, err := store.Save(&run); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load("run")
	if err != nil {
		t.Fatal(err)
	}
	before := append([]types.WriteContextPack(nil), loaded.ContextPacks...)
	incoming := types.WriteContextPack{PackID: "old-truncated", BatchID: "batch", Items: []types.WriteContextItem{{ID: "fact", Text: "third origin", Priority: types.WriteContextP1}}}
	run = upsertWorkflowRunContextPack(*loaded, incoming)
	if len(run.ContextPacks) != 3 || !reflect.DeepEqual(run.ContextPacks[:2], before) {
		t.Fatal("ambiguous update selected/replaced an old owner")
	}
	run = upsertWorkflowRunContextPack(run, incoming)
	if len(run.ContextPacks) != 3 {
		t.Fatal("identical ambiguous retry accumulated")
	}
	if _, err := store.Save(&run); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.Load("run")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.ContextPacks) != 3 || len(loaded.Batches[0].ContextPackIDs) != 3 {
		t.Fatal("independent legacy contents lost")
	}
}

func TestContextPack234LogicalTupleNamespaces(t *testing.T) {
	for name, pair := range map[string][2]types.WriteContextPack{
		"identity delimiter":   {{PackID: "a|b", BatchID: "c"}, {PackID: "a", BatchID: "b|c"}},
		"fallback delimiter":   {{SourceStage: "a|b", Goal: "c"}, {SourceStage: "a", Goal: "b|c"}},
		"identity vs fallback": {{PackID: "a", BatchID: "b"}, {SourceStage: "a", Goal: "b"}},
	} {
		t.Run(name, func(t *testing.T) {
			var run types.WriteWorkflowRun
			for i := range pair {
				pair[i].Items = []types.WriteContextItem{{ID: "same-local-id", Text: "fact", Priority: types.WriteContextP1}}
				run = upsertWorkflowRunContextPack(run, pair[i])
			}
			if len(run.ContextPacks) != 2 {
				t.Fatal("different logical tuples shared a slot")
			}
			for _, pack := range pair {
				run = upsertWorkflowRunContextPack(run, pack)
			}
			if len(run.ContextPacks) != 2 {
				t.Fatal("exact tuple retries not idempotent")
			}
		})
	}
}
