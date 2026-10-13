package repl

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

type workflowContextArtifact struct {
	pack types.WriteContextPack
	data []byte
}

type workflowContextRefKey struct{ batch, ref string }

type workflowContextRefAmbiguity struct {
	BatchID, Ref string
	Artifacts    []string
}

// A logical pack may be augmented after a restart. Keep that identity stable,
// and put each content revision in an immutable, content-addressed file. The
// address includes the run and exact typed coordinates, never a display slug.
func workflowContextPackArtifactID(runID string, pack types.WriteContextPack) (string, error) {
	pack.ArtifactID = ""
	data, err := json.Marshal(struct {
		RunID string                 `json:"run_id"`
		Pack  types.WriteContextPack `json:"pack"`
	}{runID, pack})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("ctx-v2-%x", sha256.Sum256(data)), nil
}

// Plan the complete reference migration before publishing any artifact. Legacy
// PackID values are opaque aliases: no prefix stripping or filename guessing.
// An ambiguous alias expands to all exact matches and is diagnosed, not silently
// resolved to the first pack. This changes reachability, never evidence authority.
func planWorkflowContextArtifacts(run *types.WriteWorkflowRun) ([]workflowContextArtifact, []workflowContextRefAmbiguity, error) {
	aliases := map[workflowContextRefKey]map[string]bool{}
	byBatch := map[string][]string{}
	artifacts := make([]workflowContextArtifact, 0, len(run.ContextPacks))
	seen := map[string][]byte{}
	for i, original := range run.ContextPacks {
		pack := types.NormalizeWriteContextPack(original)
		if len(pack.Items) == 0 {
			continue
		}
		id, err := workflowContextPackArtifactID(run.RunID, pack)
		if err != nil {
			return nil, nil, err
		}
		for _, alias := range []string{pack.PackID, pack.ArtifactID} {
			if alias == "" {
				continue
			}
			for _, batch := range []string{pack.BatchID, ""} {
				key := workflowContextRefKey{batch, alias}
				if aliases[key] == nil {
					aliases[key] = map[string]bool{}
				}
				aliases[key][id] = true
			}
		}
		pack.ArtifactID = id
		run.ContextPacks[i] = pack
		byBatch[pack.BatchID] = append(byBatch[pack.BatchID], id)
		data, err := json.MarshalIndent(pack, "", "  ")
		if err != nil {
			return nil, nil, err
		}
		if previous, ok := seen[id]; ok {
			if !bytes.Equal(previous, data) {
				return nil, nil, fmt.Errorf("context artifact identity collision: %s", id)
			}
			continue
		}
		seen[id] = data
		artifacts = append(artifacts, workflowContextArtifact{pack: pack, data: data})
	}
	var ambiguities []workflowContextRefAmbiguity
	diagnosed := map[workflowContextRefKey]bool{}
	rewrite := func(batch string, refs []string) []string {
		out, added := make([]string, 0, len(refs)), map[string]bool{}
		for _, ref := range refs {
			key := workflowContextRefKey{batch, ref}
			matches := aliases[key]
			if len(matches) == 0 {
				key = workflowContextRefKey{"", ref}
				matches = aliases[key]
			}
			next := []string{ref}
			if len(matches) > 0 {
				next = make([]string, 0, len(matches))
				for id := range matches {
					next = append(next, id)
				}
				sort.Strings(next)
			}
			if len(next) > 1 && !diagnosed[key] {
				diagnosed[key] = true
				ambiguities = append(ambiguities, workflowContextRefAmbiguity{BatchID: batch, Ref: ref, Artifacts: append([]string(nil), next...)})
			}
			for _, id := range next {
				if id != "" && !added[id] {
					added[id] = true
					out = append(out, id)
				}
			}
		}
		return out
	}
	for i := range run.Batches {
		batch := &run.Batches[i]
		refs := append([]string(nil), batch.ContextPackIDs...)
		refs = append(refs, byBatch[batch.ID]...)
		batch.ContextPackIDs = rewrite(batch.ID, refs)
		for j := range batch.Slices {
			batch.Slices[j].ContextPackIDs = rewrite(batch.ID, batch.Slices[j].ContextPackIDs)
		}
	}
	return artifacts, ambiguities, nil
}

func (s *WriteWorkflowRunStore) saveContextPacks(run *types.WriteWorkflowRun) error {
	if s == nil || run == nil || len(run.ContextPacks) == 0 {
		return nil
	}
	artifacts, ambiguities, err := planWorkflowContextArtifacts(run)
	if err != nil {
		return fmt.Errorf("WriteWorkflowRunStore.Save context plan: %w", err)
	}
	contextDir := filepath.Join(s.workflowDir, "contexts", run.RunID)
	missing := make([]workflowContextArtifact, 0, len(artifacts))
	// Never overwrite a conflicting existing object. Preflight every object so a
	// late conflict cannot corrupt the previously published workflow or its files.
	for _, artifact := range artifacts {
		path := filepath.Join(contextDir, artifact.pack.ArtifactID+".json")
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			missing = append(missing, artifact)
			continue
		}
		if err != nil {
			return fmt.Errorf("context artifact read %s: %w", path, err)
		}
		if !bytes.Equal(data, artifact.data) {
			return fmt.Errorf("context artifact content conflict: %s", artifact.pack.ArtifactID)
		}
	}
	for _, artifact := range missing {
		if err := types.WriteContextPackToFile(&artifact.pack, filepath.Join(contextDir, artifact.pack.ArtifactID+".json")); err != nil {
			return fmt.Errorf("WriteWorkflowRunStore.Save context pack %s: %w", artifact.pack.ArtifactID, err)
		}
	}
	for _, diagnostic := range ambiguities {
		logging.Warning("[repl/workflow] context_pack_ambiguous_reference batch=%q ref=%q retained_artifacts=%v", diagnostic.BatchID, diagnostic.Ref, diagnostic.Artifacts)
	}
	return nil
}
