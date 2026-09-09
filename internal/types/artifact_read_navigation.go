package types

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/canonpath"
)

// ToolArtifactReadNavigation is a run-local navigation receipt, not read or
// evidence authority. Producers prepare it from the actual resolved input
// before reading, then fill OutputRef only after successfully saving output.
// The opaque generation cannot be serialized, reconstructed from prose, or
// substituted by a different MutableState with the same task label.
type ToolArtifactReadNavigation struct {
	InputRef       string
	OutputRef      string
	OriginQueryRef string
	generation     *artifactReadNavigationGeneration
}

// Non-zero size matters: Go may give distinct zero-sized allocations the same
// address. A live receipt retains this immutable token across ordinary copies.
type artifactReadNavigationGeneration struct{ identity byte }

type artifactReadNavigationIndex map[string]map[ToolArtifactReadNavigation]struct{}

// PrepareArtifactReadNavigation resolves only exact published/derived paths.
// Call with the actual resolved input before I/O, and call again when choosing
// later advice. OutputRef is deliberately empty until a successful save.
// This method neither registers nor permits any read.
func (m *MutableState) PrepareArtifactReadNavigation(actualInputRef string) (ToolArtifactReadNavigation, bool) {
	if m == nil {
		return ToolArtifactReadNavigation{}, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.artifactReadNavigationGeneration == nil {
		return ToolArtifactReadNavigation{}, false
	}
	origin, ok := m.artifactReadNavigationOriginLocked(actualInputRef, map[string]bool{})
	if !ok {
		return ToolArtifactReadNavigation{}, false
	}
	return ToolArtifactReadNavigation{InputRef: actualInputRef, OriginQueryRef: origin, generation: m.artifactReadNavigationGeneration}, true
}

// ArtifactReadNavigationAdvisory formats an already rechecked ticket. It does
// not revalidate mutable lifetime: consumers must obtain a fresh Prepare result
// immediately before formatting. No derived coordinates are transplanted.
func ArtifactReadNavigationAdvisory(ticket ToolArtifactReadNavigation) string {
	if ticket.generation == nil || artifactReadNavigationRefKey(ticket.InputRef) == "" || artifactReadNavigationRefKey(ticket.OriginQueryRef) == "" {
		return ""
	}
	quoted, _ := json.Marshal(ticket.OriginQueryRef)
	return "query_result_return_navigation: use grep or read_file on the original published query result at " + string(quoted) + ". This is navigation only: derived line numbers are not original trace lines and this link grants no new read or evidence authority.\n"
}

func artifactReadNavigationRefKey(ref string) string {
	if strings.TrimSpace(ref) == "" {
		return ""
	}
	key := canonpath.CanonicalRepoRelative(ref, "")
	if key == "." {
		return ""
	}
	return key
}

// Exact lookup deliberately does not call ResolveTraceQueryBlobRef: its legacy
// basename compatibility is a read route, not a new lineage identity rule.
func (m *MutableState) artifactReadNavigationPublishedOriginLocked(ref string) (string, bool) {
	key := artifactReadNavigationRefKey(ref)
	if key == "" {
		return "", false
	}
	origin, ok := m.artifactReadNavigationPublished[key]
	if !ok || artifactReadNavigationRefKey(origin) != key {
		return "", false
	}
	registered, ok := m.traceQueryPublishedBlobRefs[key]
	if !ok || artifactReadNavigationRefKey(registered) != key {
		return "", false
	}
	return traceQueryBlobRefVerified(origin)
}

// The legacy permission registry merges independently of this run-local
// generation. Its old fork union must not mint a new navigation origin.
// Only an actual registration in this generation calls this producer hook.
func (m *MutableState) registerArtifactReadNavigationOriginLocked(key, ref string) {
	if m.artifactReadNavigationGeneration == nil || key == "" || artifactReadNavigationRefKey(ref) != key {
		return
	}
	if m.artifactReadNavigationPublished == nil {
		m.artifactReadNavigationPublished = map[string]string{}
	}
	m.artifactReadNavigationPublished[key] = ref
}

func (m *MutableState) artifactReadNavigationOriginLocked(ref string, visited map[string]bool) (string, bool) {
	key := artifactReadNavigationRefKey(ref)
	if key == "" || visited[key] {
		return "", false
	}
	visited[key] = true
	if origin, ok := m.artifactReadNavigationPublishedOriginLocked(ref); ok {
		return origin, true
	}
	links := m.artifactReadNavigation[key]
	if len(links) != 1 {
		return "", false
	}
	for link := range links {
		if link.generation == nil || link.generation != m.artifactReadNavigationGeneration || artifactReadNavigationRefKey(link.OutputRef) != key {
			return "", false
		}
		origin, ok := m.artifactReadNavigationPublishedOriginLocked(link.OriginQueryRef)
		if !ok {
			return "", false
		}
		parentOrigin, ok := m.artifactReadNavigationOriginLocked(link.InputRef, visited)
		if !ok || artifactReadNavigationRefKey(parentOrigin) != artifactReadNavigationRefKey(origin) {
			return "", false
		}
		return origin, true
	}
	return "", false
}

func (m *MutableState) registerArtifactReadNavigationResultLocked(result ToolResult) {
	if !result.Success {
		return
	}
	switch CanonicalToolName(result.ToolName) {
	case "grep", "read_file":
	default:
		return
	}
	link, ok := m.validArtifactReadNavigationLinkLocked(result.ArtifactReadNavigation)
	if !ok || artifactReadNavigationRefKey(result.RawRef) != link.OutputRef {
		return
	}
	origin, ok := m.artifactReadNavigationOriginLocked(link.InputRef, map[string]bool{})
	if !ok || artifactReadNavigationRefKey(origin) != artifactReadNavigationRefKey(link.OriginQueryRef) {
		return
	}
	m.addArtifactReadNavigationLinkLocked(link)
}

func (m *MutableState) validArtifactReadNavigationLinkLocked(link ToolArtifactReadNavigation) (ToolArtifactReadNavigation, bool) {
	if link.generation == nil || link.generation != m.artifactReadNavigationGeneration {
		return ToolArtifactReadNavigation{}, false
	}
	input, output := artifactReadNavigationRefKey(link.InputRef), artifactReadNavigationRefKey(link.OutputRef)
	if input == "" || output == "" || !filepath.IsAbs(output) || input == output {
		return ToolArtifactReadNavigation{}, false
	}
	// A read result never redefines an original query publication.
	if _, direct := m.artifactReadNavigationPublishedOriginLocked(output); direct {
		return ToolArtifactReadNavigation{}, false
	}
	origin, ok := m.artifactReadNavigationPublishedOriginLocked(link.OriginQueryRef)
	if !ok {
		return ToolArtifactReadNavigation{}, false
	}
	// Normalize only the index copy. The producer's actual path spellings
	// remain untouched in its ToolResult; source case is never folded.
	link.InputRef, link.OutputRef, link.OriginQueryRef = input, output, origin
	return link, true
}

func (m *MutableState) addArtifactReadNavigationLinkLocked(link ToolArtifactReadNavigation) {
	if m.artifactReadNavigation == nil {
		m.artifactReadNavigation = artifactReadNavigationIndex{}
	}
	if m.artifactReadNavigation[link.OutputRef] == nil {
		m.artifactReadNavigation[link.OutputRef] = map[ToolArtifactReadNavigation]struct{}{}
	}
	// Retain the whole tuple set. Even two different parents leading to the
	// same original query make this output ambiguous, never first-wins.
	m.artifactReadNavigation[link.OutputRef][link] = struct{}{}
}

func cloneArtifactReadNavigationIndex(in artifactReadNavigationIndex) artifactReadNavigationIndex {
	if len(in) == 0 {
		return nil
	}
	out := make(artifactReadNavigationIndex, len(in))
	for key, links := range in {
		out[key] = make(map[ToolArtifactReadNavigation]struct{}, len(links))
		for link := range links {
			out[key][link] = struct{}{}
		}
	}
	return out
}

func (m *MutableState) mergeArtifactReadNavigationLocked(generation *artifactReadNavigationGeneration, published map[string]string, index artifactReadNavigationIndex) {
	if generation == nil || generation != m.artifactReadNavigationGeneration {
		return
	}
	for key, ref := range published {
		registered, ok := m.traceQueryPublishedBlobRefs[key]
		if ok && artifactReadNavigationRefKey(registered) == key {
			m.registerArtifactReadNavigationOriginLocked(key, ref)
		}
	}
	for _, links := range index {
		for link := range links {
			if valid, ok := m.validArtifactReadNavigationLinkLocked(link); ok {
				m.addArtifactReadNavigationLinkLocked(valid)
			}
		}
	}
	// Do not resolve parent links while inserting an unordered snapshot.
	// Every lookup rechecks the complete chain against the merged index, so
	// new ancestor conflicts invalidate descendants without scanning history.
}
