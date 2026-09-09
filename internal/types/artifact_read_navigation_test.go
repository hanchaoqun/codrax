package types

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const artifactNavigationRootA = "/work/.codrax/blob/session/query-a.json"
const artifactNavigationRootB = "/work/.codrax/blob/session/query-b.json"
const artifactNavigationDerived = "/work/.codrax/blob/session/grep-output.txt"

func artifactNavigationPublishRoot(m *MutableState, ref string) {
	m.AppendDispatchToolResult(ToolResult{ToolName: "trace_query", Success: true, RawRef: ref})
}

func artifactNavigationResult(t *testing.T, m *MutableState, input, output string) ToolResult {
	t.Helper()
	ticket, ok := m.PrepareArtifactReadNavigation(input)
	if !ok {
		t.Fatalf("valid source ticket absent for %s", input)
	}
	ticket.OutputRef = output
	return ToolResult{ToolName: "grep", Success: true, RawRef: output, ArtifactReadNavigation: ticket}
}

func TestB1624bDispatchPreservesDerivedNavigationWithoutReadPermission(t *testing.T) {
	m := NewMutableState("navigation")
	artifactNavigationPublishRoot(m, artifactNavigationRootA)
	result := artifactNavigationResult(t, m, artifactNavigationRootA, artifactNavigationDerived)
	m.AppendDispatchToolResult(result)
	ticket, ok := m.PrepareArtifactReadNavigation(artifactNavigationDerived)
	if !ok || ticket.InputRef != artifactNavigationDerived || ticket.OriginQueryRef != artifactNavigationRootA || ticket.OutputRef != "" {
		t.Errorf("successful dispatch lost original query navigation: ok=%t ticket=%+v", ok, ticket)
	}
	if _, allowed := m.ResolveTraceQueryBlobRef(artifactNavigationDerived); allowed {
		t.Fatal("navigation must not register a derived read permission")
	}
	if got := m.DispatchToolResults(); len(got) != 2 || got[1].ArtifactReadNavigation != result.ArtifactReadNavigation {
		t.Fatal("producer ticket must survive the ordinary result path")
	}
}

func TestB1624bNavigationOnlyUsesExactCurrentOriginsAndSuccessfulOutput(t *testing.T) {
	for _, change := range []struct {
		name string
		edit func(*ToolResult)
	}{
		{"failed", func(r *ToolResult) { r.Success = false }},
		{"other_tool", func(r *ToolResult) { r.ToolName = "repo_map" }},
		{"output_mismatch", func(r *ToolResult) { r.RawRef += "other" }},
		{"empty_output", func(r *ToolResult) { r.RawRef = ""; r.ArtifactReadNavigation.OutputRef = "" }},
		{"relative_output", func(r *ToolResult) {
			r.RawRef = ".codrax/blob/session/derived.txt"
			r.ArtifactReadNavigation.OutputRef = r.RawRef
		}},
		{"basename_output", func(r *ToolResult) { r.RawRef = "derived.txt"; r.ArtifactReadNavigation.OutputRef = r.RawRef }},
		{"unknown_input", func(r *ToolResult) { r.ArtifactReadNavigation.InputRef = "/unknown/input" }},
		{"wrong_origin", func(r *ToolResult) { r.ArtifactReadNavigation.OriginQueryRef = artifactNavigationRootB }},
		{"origin_basename", func(r *ToolResult) { r.ArtifactReadNavigation.OriginQueryRef = filepath.Base(artifactNavigationRootA) }},
		{"origin_case_only", func(r *ToolResult) {
			r.ArtifactReadNavigation.OriginQueryRef = strings.Replace(artifactNavigationRootA, "query-a", "Query-A", 1)
		}},
		{"legacy_no_metadata", func(r *ToolResult) { r.ArtifactReadNavigation = ToolArtifactReadNavigation{} }},
		{"literal_no_token", func(r *ToolResult) { r.ArtifactReadNavigation.generation = nil }},
	} {
		t.Run(change.name, func(t *testing.T) {
			m := NewMutableState("negative")
			artifactNavigationPublishRoot(m, artifactNavigationRootA)
			r := artifactNavigationResult(t, m, artifactNavigationRootA, artifactNavigationDerived)
			change.edit(&r)
			m.AppendDispatchToolResult(r)
			if _, ok := m.PrepareArtifactReadNavigation(r.RawRef); ok {
				t.Fatalf("invalid producer tuple became a navigation link: %+v", r.ArtifactReadNavigation)
			}
			if len(m.artifactReadNavigation) != 0 {
				t.Fatal("invalid tuple was retained for later resurrection")
			}
		})
	}
	m := NewMutableState("exact")
	artifactNavigationPublishRoot(m, artifactNavigationRootA)
	for _, wrong := range []string{filepath.Base(artifactNavigationRootA), "/elsewhere/" + filepath.Base(artifactNavigationRootA), strings.Replace(artifactNavigationRootA, "query-a", "Query-A", 1)} {
		if _, ok := m.PrepareArtifactReadNavigation(wrong); ok {
			t.Fatalf("basename/case/nonmatching actual path must not borrow a root: %q", wrong)
		}
	}
	if _, ok := m.PrepareArtifactReadNavigation(strings.ReplaceAll(artifactNavigationRootA, "/", `\`)); !ok {
		t.Fatal("existing lexical slash normalization should retain the exact path")
	}
	legacy := &MutableState{}
	artifactNavigationPublishRoot(legacy, artifactNavigationRootA)
	if _, ok := legacy.PrepareArtifactReadNavigation(artifactNavigationRootA); ok {
		t.Fatal("literal/legacy mutable without a lifecycle token is unknown")
	}
}

func TestB1624bNavigationGenerationAndDispatchLifecycle(t *testing.T) {
	m := NewMutableState("same label")
	artifactNavigationPublishRoot(m, artifactNavigationRootA)
	first := artifactNavigationResult(t, m, artifactNavigationRootA, artifactNavigationDerived)
	m.AppendDispatchToolResult(first)
	m.ResetDispatchToolResults()
	if _, ok := m.PrepareArtifactReadNavigation(artifactNavigationDerived); !ok {
		t.Fatal("one task's navigation must survive dispatch reset")
	}
	other := NewMutableState("same label")
	artifactNavigationPublishRoot(other, artifactNavigationRootA)
	other.AppendDispatchToolResult(first)
	if _, ok := other.PrepareArtifactReadNavigation(artifactNavigationDerived); ok || other.artifactReadNavigationGeneration == m.artifactReadNavigationGeneration {
		t.Fatal("unrelated Mutable instances cannot share a generation")
	}
	m.ResetTurnAArtifacts()
	artifactNavigationPublishRoot(m, artifactNavigationRootA) // Identical path in a new cycle.
	m.AppendDispatchToolResult(first)                         // Old I/O finishes late.
	if _, ok := m.PrepareArtifactReadNavigation(artifactNavigationDerived); ok {
		t.Fatal("same origin path re-registration must not revive an old ticket")
	}
	fresh := artifactNavigationResult(t, m, artifactNavigationRootA, artifactNavigationDerived)
	if fresh.ArtifactReadNavigation.generation == first.ArtifactReadNavigation.generation {
		t.Fatal("reset did not advance the unique generation")
	}
	m.AppendDispatchToolResult(fresh)
	if _, ok := m.PrepareArtifactReadNavigation(artifactNavigationDerived); !ok {
		t.Fatal("actual new-cycle production must remain usable")
	}
}

func TestB1624bLateForkCannotCreateFreshNavigationForOldOriginal(t *testing.T) {
	m := NewMutableState("late original")
	artifactNavigationPublishRoot(m, artifactNavigationRootA)
	fork := m.ForkForExploreDispatch()
	m.ResetTurnAArtifacts()
	m.MergeExploreFork(fork)
	// This task must not alter the legacy permission registry's merge rule.
	if _, ok := m.ResolveTraceQueryBlobRef(artifactNavigationRootA); !ok {
		t.Fatal("old read-registry semantics changed")
	}
	if ticket, ok := m.PrepareArtifactReadNavigation(artifactNavigationRootA); ok {
		t.Fatalf("late old original minted a fresh navigation ticket: %+v", ticket)
	}
	artifactNavigationPublishRoot(m, artifactNavigationRootA)
	if _, ok := m.PrepareArtifactReadNavigation(artifactNavigationRootA); !ok {
		t.Fatal("real current-generation re-publication must prepare a new ticket")
	}
}

func TestB1624bNavigationConflictsPropagateToDescendants(t *testing.T) {
	const child = "/work/.codrax/blob/session/page.txt"
	for _, reverse := range []bool{false, true} {
		m := NewMutableState("conflict")
		artifactNavigationPublishRoot(m, artifactNavigationRootA)
		artifactNavigationPublishRoot(m, artifactNavigationRootB)
		left, right := artifactNavigationRootA, artifactNavigationRootB
		if reverse {
			left, right = right, left
		}
		m.AppendDispatchToolResult(artifactNavigationResult(t, m, left, artifactNavigationDerived))
		page := artifactNavigationResult(t, m, artifactNavigationDerived, child)
		page.ToolName = "read_file"
		m.AppendDispatchToolResult(page)
		if ticket, ok := m.PrepareArtifactReadNavigation(child); !ok || ticket.OriginQueryRef != left {
			t.Fatal("successful two-hop chain lost its original source")
		}
		m.AppendDispatchToolResult(artifactNavigationResult(t, m, right, artifactNavigationDerived))
		for _, path := range []string{artifactNavigationDerived, child} {
			if _, ok := m.PrepareArtifactReadNavigation(path); ok {
				t.Fatalf("ancestor conflict must make %q unknown (reverse=%t)", path, reverse)
			}
		}
		if len(m.artifactReadNavigation[artifactNavigationDerived]) != 2 {
			t.Fatal("conflicting tuples must remain inspectable, not first-wins")
		}
	}
	// Same root is insufficient when the actual parents differ.
	m := NewMutableState("different parents")
	artifactNavigationPublishRoot(m, artifactNavigationRootA)
	m.AppendDispatchToolResult(artifactNavigationResult(t, m, artifactNavigationRootA, artifactNavigationDerived))
	m.AppendDispatchToolResult(artifactNavigationResult(t, m, artifactNavigationDerived, child))
	m.AppendDispatchToolResult(artifactNavigationResult(t, m, artifactNavigationRootA, child))
	if _, ok := m.PrepareArtifactReadNavigation(child); ok {
		t.Fatal("different parent inputs cannot collapse just because the root agrees")
	}
}

func TestB1624bNavigationForkMergePreservesConflictsAndRejectsOldGeneration(t *testing.T) {
	const grandchild = "/work/.codrax/blob/session/grandchild.txt"
	for _, reverse := range []bool{false, true} {
		m := NewMutableState("fork")
		artifactNavigationPublishRoot(m, artifactNavigationRootA)
		artifactNavigationPublishRoot(m, artifactNavigationRootB)
		forkA, forkB := m.ForkForExploreDispatch(), m.ForkForExploreDispatch()
		forkA.AppendDispatchToolResult(artifactNavigationResult(t, forkA, artifactNavigationRootA, artifactNavigationDerived))
		grand := forkA.ForkForExploreDispatch()
		grand.AppendDispatchToolResult(artifactNavigationResult(t, grand, artifactNavigationDerived, grandchild))
		forkA.MergeExploreFork(grand)
		forkB.AppendDispatchToolResult(artifactNavigationResult(t, forkB, artifactNavigationRootB, artifactNavigationDerived))
		if _, ok := m.PrepareArtifactReadNavigation(artifactNavigationDerived); ok {
			t.Fatal("fork-local writes leaked before merge")
		}
		if reverse {
			m.MergeExploreFork(forkB)
			m.MergeExploreFork(forkA)
		} else {
			m.MergeExploreFork(forkA)
			m.MergeExploreFork(forkB)
		}
		if len(m.artifactReadNavigation[artifactNavigationDerived]) != 2 {
			t.Fatal("fork merge lost a conflicting source")
		}
		if _, ok := m.PrepareArtifactReadNavigation(grandchild); ok {
			t.Fatal("grandchild must not survive its ancestor's merged conflict")
		}
		if _, ok := forkA.PrepareArtifactReadNavigation(grandchild); !ok {
			t.Fatal("parent merge mutated the fork's independent index")
		}
		m.ResetTurnAArtifacts()
		artifactNavigationPublishRoot(m, artifactNavigationRootA)
		m.MergeExploreFork(forkA)
		if len(m.artifactReadNavigation) != 0 {
			t.Fatal("late fork resurrected navigation from the previous task")
		}
	}
}

func TestB1624bNavigationForkClonesExistingLinksAndMergesNewOriginal(t *testing.T) {
	m := NewMutableState("fresh fork")
	artifactNavigationPublishRoot(m, artifactNavigationRootA)
	m.AppendDispatchToolResult(artifactNavigationResult(t, m, artifactNavigationRootA, artifactNavigationDerived))
	fork := m.ForkForExploreDispatch()
	if _, ok := fork.PrepareArtifactReadNavigation(artifactNavigationDerived); !ok {
		t.Fatal("fork lost the parent's accepted navigation")
	}
	artifactNavigationPublishRoot(fork, artifactNavigationRootB)
	const outputB = "/elsewhere/.codrax/blob/session/grep-output.txt"
	fork.AppendDispatchToolResult(artifactNavigationResult(t, fork, artifactNavigationRootB, outputB))
	if _, ok := m.PrepareArtifactReadNavigation(artifactNavigationRootB); ok {
		t.Fatal("fork-local original registration leaked before merge")
	}
	m.MergeExploreFork(fork)
	for output, want := range map[string]string{artifactNavigationDerived: artifactNavigationRootA, outputB: artifactNavigationRootB} {
		if ticket, ok := m.PrepareArtifactReadNavigation(output); !ok || ticket.OriginQueryRef != want {
			t.Fatalf("same-basename full paths or same-generation merge lost identity: %s %+v", output, ticket)
		}
	}
	// Mutating/resetting the fork cannot mutate the parent's cloned maps.
	fork.ResetTurnAArtifacts()
	if _, ok := m.PrepareArtifactReadNavigation(outputB); !ok {
		t.Fatal("fork reset aliased the merged parent's navigation index")
	}
}

func TestB1624bNavigationValueCopiesMemoAndJSONBoundary(t *testing.T) {
	m := NewMutableState("value copies")
	artifactNavigationPublishRoot(m, artifactNavigationRootA)
	r := artifactNavigationResult(t, m, artifactNavigationRootA, artifactNavigationDerived)
	m.AppendDispatchToolResult(r)
	m.StoreToolResultMemo("grep", "exact-read", r)
	copy, ok := m.ToolResultMemo("grep", "exact-read")
	if !ok || copy.ArtifactReadNavigation != r.ArtifactReadNavigation {
		t.Fatal("in-memory result copy lost the immutable ticket")
	}
	copy.ArtifactReadNavigation.OriginQueryRef = artifactNavigationRootB
	if got, _ := m.ToolResultMemo("grep", "exact-read"); got.ArtifactReadNavigation.OriginQueryRef != artifactNavigationRootA {
		t.Fatal("ticket fields alias a stored result")
	}
	encoded, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "ArtifactReadNavigation") || strings.Contains(string(encoded), artifactNavigationRootA) {
		t.Fatal("run-local lineage leaked into serialized/model-facing result")
	}
	var legacy ToolResult
	if err := json.Unmarshal(encoded, &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.ArtifactReadNavigation != (ToolArtifactReadNavigation{}) {
		t.Fatal("JSON replay recreated a private generation")
	}
	var forged ToolArtifactReadNavigation
	if err := json.Unmarshal([]byte(`{"InputRef":"`+artifactNavigationRootA+`","OutputRef":"`+artifactNavigationDerived+`","OriginQueryRef":"`+artifactNavigationRootA+`","generation":1}`), &forged); err != nil {
		t.Fatal(err)
	}
	if ArtifactReadNavigationAdvisory(forged) != "" {
		t.Fatal("external fields minted a private navigation ticket")
	}
	other := NewMutableState("old snapshot")
	artifactNavigationPublishRoot(other, artifactNavigationRootA)
	other.AppendDispatchToolResult(legacy)
	if _, ok := other.PrepareArtifactReadNavigation(artifactNavigationDerived); ok {
		t.Fatal("legacy snapshot acquired navigation from RawRef or Summary")
	}
	if !reflect.DeepEqual(m.TraceQueryBlobRefs(), []string{artifactNavigationRootA}) {
		t.Fatal("result copies changed read permissions")
	}
}

func TestB1624bNavigationAdvisoryIsSingleSourceAndJSONQuoted(t *testing.T) {
	m := NewMutableState("advice")
	ref := "/work/.codrax/blob/session/query-\"odd\".json"
	artifactNavigationPublishRoot(m, ref)
	ticket, ok := m.PrepareArtifactReadNavigation(ref)
	if !ok {
		t.Fatal("exact quoted path was not prepared")
	}
	quoted, _ := json.Marshal(ref)
	want := "query_result_return_navigation: use grep or read_file on the original published query result at " + string(quoted) + ". This is navigation only: derived line numbers are not original trace lines and this link grants no new read or evidence authority.\n"
	if got := ArtifactReadNavigationAdvisory(ticket); got != want {
		t.Fatalf("shared navigation wording/escaping changed: %q", got)
	}
}
