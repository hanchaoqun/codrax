package orchestrator

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/hanchaoqun/codrax/internal/repl"
	"github.com/hanchaoqun/codrax/internal/types"
)

const registrationContextPackAuditFile = "restart-context-pack-audit.json"

type registrationContextPackInventory struct {
	PackCount         int                 `json:"pack_count"`
	UniqueLogicalIDs  int                 `json:"unique_scoped_logical_ids"`
	UniqueArtifactIDs int                 `json:"unique_artifact_ids"`
	ContentByLogical  map[string]string   `json:"content_sha256_by_scoped_logical_id"`
	ArtifactByLogical map[string]string   `json:"artifact_by_scoped_logical_id"`
	ArtifactFiles     map[string]string   `json:"artifact_file_sha256"`
	BatchReferences   map[string][]string `json:"batch_references"`
	SliceReferences   map[string][]string `json:"slice_references"`
	OpaqueReferences  map[string][]string `json:"opaque_references"`
	Problems          []string            `json:"problems,omitempty"`
}

type registrationContextPackAudit struct {
	Passed    bool                                        `json:"passed"`
	Snapshots map[string]registrationContextPackInventory `json:"snapshots"`
	Problems  []string                                    `json:"problems,omitempty"`
}

// Inspect actual pack data, not model prose or the production storage-key
// function. Logical identity is batch-scoped; "merged" in two batches is legal.
// Child memory may not yet carry the latest ArtifactID. Only durable snapshots
// must resolve each current artifact to the exact corresponding file content.
func inspectRegistrationContextPacks(run *types.WriteWorkflowRun, contextDir string, durable bool) registrationContextPackInventory {
	out := registrationContextPackInventory{
		ContentByLogical: map[string]string{}, ArtifactByLogical: map[string]string{}, ArtifactFiles: map[string]string{},
		BatchReferences: map[string][]string{}, SliceReferences: map[string][]string{}, OpaqueReferences: map[string][]string{},
	}
	problem := func(format string, args ...any) { out.Problems = append(out.Problems, fmt.Sprintf(format, args...)) }
	if run == nil {
		problem("workflow snapshot is missing")
		return out
	}
	out.PackCount = len(run.ContextPacks)
	artifacts := map[string]string{}
	byBatch := map[string][]string{}
	for _, pack := range run.ContextPacks {
		keyBytes, _ := json.Marshal([]string{pack.BatchID, pack.PackID})
		key := string(keyBytes)
		logical := pack
		logical.ArtifactID = ""
		body, err := json.Marshal(logical)
		if err != nil {
			problem("logical pack %s cannot be marshaled: %v", key, err)
			continue
		}
		if _, exists := out.ContentByLogical[key]; exists {
			problem("duplicate scoped logical pack identity: %s", key)
		}
		out.ContentByLogical[key] = fmt.Sprintf("%x", sha256.Sum256(body))
		out.ArtifactByLogical[key] = pack.ArtifactID
		if pack.ArtifactID != "" {
			if previous, exists := artifacts[pack.ArtifactID]; exists && previous != key {
				problem("artifact %s aliases different logical packs: %s / %s", pack.ArtifactID, previous, key)
			}
			artifacts[pack.ArtifactID] = key
		}
		if !durable || len(pack.Items) == 0 {
			continue
		}
		if pack.ArtifactID == "" || filepath.Base(pack.ArtifactID) != pack.ArtifactID || pack.ArtifactID == "." || pack.ArtifactID == ".." {
			problem("durable pack %s lacks a safe artifact reference", key)
			continue
		}
		byBatch[pack.BatchID] = append(byBatch[pack.BatchID], pack.ArtifactID)
		file, err := os.ReadFile(filepath.Join(contextDir, pack.ArtifactID+".json"))
		if err != nil {
			problem("durable artifact %s is unreadable: %v", pack.ArtifactID, err)
			continue
		}
		out.ArtifactFiles[pack.ArtifactID] = fmt.Sprintf("%x", sha256.Sum256(file))
		var stored types.WriteContextPack
		if err := json.Unmarshal(file, &stored); err != nil {
			problem("durable artifact %s is invalid JSON: %v", pack.ArtifactID, err)
		} else if !reflect.DeepEqual(pack, stored) {
			problem("durable artifact %s differs from the workflow's exact pack", pack.ArtifactID)
		}
	}
	out.UniqueLogicalIDs, out.UniqueArtifactIDs = len(out.ContentByLogical), len(artifacts)
	refs := func(scope string, values []string) []string {
		copied := append([]string{}, values...)
		seen := map[string]bool{}
		for _, ref := range copied {
			if seen[ref] {
				problem("duplicate context reference at %s: %s", scope, ref)
			}
			seen[ref] = true
			if _, managed := artifacts[ref]; !managed {
				// Historical/external references are not silently promoted to
				// current managed artifacts, nor rejected just for being opaque.
				out.OpaqueReferences[scope] = append(out.OpaqueReferences[scope], ref)
			}
		}
		sort.Strings(copied)
		sort.Strings(out.OpaqueReferences[scope])
		return copied
	}
	for _, batch := range run.Batches {
		out.BatchReferences[batch.ID] = refs("batch:"+batch.ID, batch.ContextPackIDs)
		if durable {
			for _, artifact := range byBatch[batch.ID] {
				at := sort.SearchStrings(out.BatchReferences[batch.ID], artifact)
				if at == len(out.BatchReferences[batch.ID]) || out.BatchReferences[batch.ID][at] != artifact {
					problem("batch %s cannot reach its current artifact %s", batch.ID, artifact)
				}
			}
		}
		for _, slice := range batch.Slices {
			key := batch.ID + ":" + slice.ID
			out.SliceReferences[key] = refs("slice:"+key, slice.ContextPackIDs)
		}
	}
	return out
}

// This adds no model call and grants no execution authority. It checks the
// real child's output and the default durable store, then repeats persistence
// of an unchanged terminal snapshot. Preserve the audit even when it fails.
func auditNativeRegistrationContextPacks(t *testing.T, store *repl.WriteWorkflowRunStore, workDir string, loaded *types.WriteWorkflowRun) {
	t.Helper()
	audit := registrationContextPackAudit{Snapshots: map[string]registrationContextPackInventory{}}
	defer func() {
		audit.Passed = len(audit.Problems) == 0
		body, err := json.MarshalIndent(audit, "", "  ")
		if err == nil {
			err = os.WriteFile(filepath.Join(workDir, registrationContextPackAuditFile), body, 0600)
		}
		if err != nil {
			t.Errorf("preserve context-pack audit: %v", err)
		}
		if !audit.Passed {
			t.Errorf("context-pack durability audit failed: %v", audit.Problems)
		}
	}()
	contextDir := filepath.Join(store.WorkflowDir(), "contexts", loaded.RunID)
	inspect := func(name string, run *types.WriteWorkflowRun, durable bool) registrationContextPackInventory {
		inv := inspectRegistrationContextPacks(run, contextDir, durable)
		audit.Snapshots[name] = inv
		for _, problem := range inv.Problems {
			audit.Problems = append(audit.Problems, name+": "+problem)
		}
		return inv
	}
	read := func(name, path string) *types.WriteWorkflowRun {
		body, err := os.ReadFile(path)
		var run types.WriteWorkflowRun
		if err == nil {
			err = json.Unmarshal(body, &run)
		}
		if err != nil {
			audit.Problems = append(audit.Problems, name+": "+err.Error())
			return nil
		}
		return &run
	}
	child := inspect("child_memory", read("child", filepath.Join(workDir, "restart-child-workflow.json")), false)
	raw := inspect("durable_raw", read("durable", filepath.Join(store.WorkflowDir(), loaded.RunID+".json")), true)
	initial := inspect("durable_loaded", loaded, true)
	if child.PackCount != initial.PackCount || !reflect.DeepEqual(child.ContentByLogical, initial.ContentByLogical) {
		audit.Problems = append(audit.Problems, "child-to-durable persistence changed pack count, logical identity, or content")
	}
	if !reflect.DeepEqual(raw, initial) {
		audit.Problems = append(audit.Problems, "loading normalized away a durable identity/reference/content discrepancy")
	}
	current := loaded
	for cycle := 1; cycle <= 2; cycle++ {
		before, _ := json.Marshal(current)
		if _, err := store.Save(current); err != nil {
			audit.Problems = append(audit.Problems, fmt.Sprintf("save cycle %d: %v", cycle, err))
			return
		}
		after, _ := json.Marshal(current)
		if string(before) != string(after) {
			audit.Problems = append(audit.Problems, fmt.Sprintf("save cycle %d mutated its caller's workflow", cycle))
		}
		next, err := store.Load(current.RunID)
		if err != nil {
			audit.Problems = append(audit.Problems, fmt.Sprintf("load cycle %d: %v", cycle, err))
			return
		}
		got := inspect(fmt.Sprintf("round_trip_%d", cycle), next, true)
		if !reflect.DeepEqual(initial, got) {
			audit.Problems = append(audit.Problems, fmt.Sprintf("unchanged save/load cycle %d changed logical packs, content, artifact identities, or reference sets", cycle))
		}
		current = next
	}
}

func assertNativeRegistrationContextPackReceipt(t *testing.T, workDir string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(workDir, registrationContextPackAuditFile))
	var audit registrationContextPackAudit
	if err == nil {
		err = json.Unmarshal(body, &audit)
	}
	if err != nil || !audit.Passed || len(audit.Snapshots) != 5 {
		t.Fatalf("context-pack audit must succeed before publishing a success receipt: %v %+v", err, audit)
	}
}

func TestNativeRegistrationContextPackOracle(t *testing.T) {
	a := types.NormalizeWriteContextPack(types.WriteContextPack{PackID: "merged", BatchID: "a", Items: []types.WriteContextItem{{Text: "source fact"}}})
	b := a
	b.BatchID = "b"
	base := &types.WriteWorkflowRun{RunID: "oracle", ContextPacks: []types.WriteContextPack{a, b}, Batches: []types.WriteWorkflowBatch{{ID: "a", ContextPackIDs: []string{"external-reference"}}, {ID: "b"}}}
	got := inspectRegistrationContextPacks(base, "", false)
	if len(got.Problems) != 0 || got.UniqueLogicalIDs != 2 || len(got.OpaqueReferences["batch:a"]) != 1 {
		t.Fatalf("cross-batch names and opaque references must be retained: %+v", got)
	}
	for _, mode := range []string{"logical_collision", "artifact_collision", "duplicate_reference", "missing_artifact"} {
		t.Run(mode, func(t *testing.T) {
			run := *base
			run.ContextPacks = append([]types.WriteContextPack(nil), base.ContextPacks...)
			run.Batches = append([]types.WriteWorkflowBatch(nil), base.Batches...)
			switch mode {
			case "logical_collision":
				run.ContextPacks[1].BatchID = "a"
			case "artifact_collision":
				run.ContextPacks[0].ArtifactID, run.ContextPacks[1].ArtifactID = "same", "same"
			case "duplicate_reference":
				run.Batches[0].ContextPackIDs = []string{"external-reference", "external-reference"}
			case "missing_artifact":
				run.ContextPacks[0].ArtifactID, run.ContextPacks[1].ArtifactID = "first", "second"
			}
			if got := inspectRegistrationContextPacks(&run, t.TempDir(), mode == "missing_artifact"); len(got.Problems) == 0 {
				t.Fatalf("oracle accepted %s: %+v", mode, got)
			}
		})
	}
}
