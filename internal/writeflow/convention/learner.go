package convention

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

type GraphProvider interface {
	Imports(path string) []string
	ReverseImports(path string) []string
	RelatedTests(path string) []string
}

type LearnInput struct {
	BatchID       string
	Goal          string
	ActivePaths   []string
	ActiveSymbols []string
	PatchEffect   *types.PatchEffectRecord
	Graph         GraphProvider
	Limit         int
}

func LearnFromGraph(in LearnInput) types.ConventionGraph {
	graph := types.ConventionGraph{
		BatchID: strings.TrimSpace(in.BatchID),
		Goal:    strings.TrimSpace(in.Goal),
		Source:  "repository_convention_learner",
	}
	activePaths := learnActivePaths(in)
	for _, path := range activePaths {
		graph.Nodes = append(graph.Nodes, learnPatchSurfaceNode(path, in.PatchEffect)...)
		if in.Graph == nil {
			continue
		}
		for _, test := range in.Graph.RelatedTests(path) {
			test = normalizeConventionPath(test)
			if test == "" {
				continue
			}
			graph.Nodes = append(graph.Nodes, types.ConventionNode{
				Category:      types.ConventionCategoryRelationship,
				Summary:       fmt.Sprintf("related test surface %s is associated with %s", test, path),
				Source:        test,
				Subject:       path,
				EvidenceRefID: "related_test:" + path + "->" + test,
				SourceStage:   "convention_learner",
				Strength:      "repo_graph",
			})
		}
		for _, dependent := range in.Graph.ReverseImports(path) {
			dependent = normalizeConventionPath(dependent)
			if dependent == "" {
				continue
			}
			graph.Nodes = append(graph.Nodes, types.ConventionNode{
				Category:      types.ConventionCategoryRelationship,
				Summary:       fmt.Sprintf("dependent surface %s imports %s", dependent, path),
				Source:        dependent,
				Subject:       path,
				EvidenceRefID: "reverse_import:" + dependent + "->" + path,
				SourceStage:   "convention_learner",
				Strength:      "repo_graph",
			})
		}
		for _, dep := range in.Graph.Imports(path) {
			dep = normalizeConventionPath(dep)
			if dep == "" {
				continue
			}
			graph.Nodes = append(graph.Nodes, types.ConventionNode{
				Category:      types.ConventionCategoryRelationship,
				Summary:       fmt.Sprintf("changed surface %s imports dependency %s", path, dep),
				Source:        path,
				Subject:       dep,
				EvidenceRefID: "import:" + path + "->" + dep,
				SourceStage:   "convention_learner",
				Strength:      "repo_graph",
			})
		}
	}
	graph = types.NormalizeConventionGraph(graph)
	limit := in.Limit
	if limit <= 0 {
		limit = 16
	}
	return SelectTopN(graph, activePaths, in.ActiveSymbols, limit)
}

func learnActivePaths(in LearnInput) []string {
	var paths []string
	paths = append(paths, in.ActivePaths...)
	if in.PatchEffect != nil {
		for _, file := range in.PatchEffect.Files {
			if file.Path != "" {
				paths = append(paths, file.Path)
			}
			if file.OldPath != "" {
				paths = append(paths, file.OldPath)
			}
		}
	}
	return dedupeConventionPaths(paths)
}

func learnPatchSurfaceNode(path string, effect *types.PatchEffectRecord) []types.ConventionNode {
	path = normalizeConventionPath(path)
	if path == "" || effect == nil {
		return nil
	}
	var out []types.ConventionNode
	for _, file := range effect.Files {
		filePath := normalizeConventionPath(file.Path)
		if filePath == "" {
			filePath = normalizeConventionPath(file.OldPath)
		}
		if filePath != path {
			continue
		}
		role := string(file.PathRole)
		if role == "" {
			role = string(types.ClassifySourcePathRole(filePath))
		}
		summary := fmt.Sprintf("actual patch touched %s path %s", role, filePath)
		if file.Language != "" {
			summary += " language=" + file.Language
		}
		out = append(out, types.ConventionNode{
			Category:      types.ConventionCategoryMechanism,
			Summary:       summary,
			Source:        filePath,
			EvidenceRefID: strings.TrimSpace(effect.RecordID),
			SourceStage:   "convention_learner",
			Strength:      "patch_effect",
		})
	}
	return out
}

func dedupeConventionPaths(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, path := range in {
		path = normalizeConventionPath(path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}
