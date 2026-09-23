package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

const ToolDocumentationPromptMaxCount = 8
const ToolDocumentationPromptMaxBytes = 128 << 10

// Selection and rendering share the same whole-document budget. Neither path
// may silently retain only the beginning of a contract and lose its conditions.
func ToolDocumentationPromptChunk(c ToolHandoffCarrier) string {
	d := c.Documentation
	if d == nil {
		return ""
	}
	return fmt.Sprintf("Producer: %s; schema: %s; document version: %d; selected view: %q; detail: %t\n```json\n%s\n```\n\n",
		c.ToolName, d.Schema, d.Version, d.Selection.View, d.Selection.Detail, d.Content)
}

func SelectToolDocumentation(carriers []ToolHandoffCarrier) (kept []ToolHandoffCarrier, omitted int) {
	all := ToolHandoffCarriersFromTurnAInputs(nil, nil, carriers)
	used, count := 0, 0
	for i := len(all) - 1; i >= 0; i-- {
		c := all[i]
		if c.Documentation == nil {
			continue
		}
		count++
		n := len(ToolDocumentationPromptChunk(c))
		if len(kept) == ToolDocumentationPromptMaxCount || used+n > ToolDocumentationPromptMaxBytes {
			continue
		}
		kept = append(kept, c)
		used += n
	}
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	return CloneToolHandoffCarriers(kept), count - len(kept)
}

// Nonzero-size, private tokens cannot be reconstructed by model JSON or by
// replaying a serialized ToolResult. Forks share the run token, not mutable maps.
type toolDocumentationGeneration struct{ marker byte }
type toolDocumentationRead struct {
	generation *toolDocumentationGeneration
	key        string
}
type toolDocumentationCompletion struct {
	generation *toolDocumentationGeneration
	completion uint64
	profile    string
	only       bool
	documents  []ToolHandoffCarrier
}
type toolDocumentationState struct {
	generation *toolDocumentationGeneration
	documents  []ToolHandoffCarrier
	accepted   *toolDocumentationCompletion
}

func cloneToolDocumentationState(s toolDocumentationState) toolDocumentationState {
	s.documents = CloneToolHandoffCarriers(s.documents)
	// Accepted receipts are immutable; external callers only receive copies.
	return s
}

func toolDocumentationReadKey(r ToolResult) string {
	if !toolResultHasDocumentation(r) || r.ToolName != "trace_capabilities" || r.Handoff.Documentation.Schema != "trace_capabilities/v1" {
		return ""
	}
	d, ok := NormalizeToolDocumentation(*r.Handoff.Documentation)
	if !ok {
		return ""
	}
	b, _ := json.Marshal([]any{r.ToolName, d.Version, d.Schema, d.Selection, d.ContentHash})
	return string(b)
}

// StampToolDocumentationResult is called by a trusted producer after building
// its complete result. It grants nothing until the dispatcher publishes it.
func (m *MutableState) StampToolDocumentationResult(r ToolResult) ToolResult {
	if m == nil {
		return r
	}
	key := toolDocumentationReadKey(r)
	if key == "" {
		return r
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.toolDocumentation.generation == nil {
		m.toolDocumentation.generation = &toolDocumentationGeneration{}
	}
	r.Handoff = &CloneToolHandoffCarriers([]ToolHandoffCarrier{*r.Handoff})[0]
	r.Handoff.Documentation.read = &toolDocumentationRead{m.toolDocumentation.generation, key}
	return r
}

func (m *MutableState) registerToolDocumentationLocked(r ToolResult) {
	key := toolDocumentationReadKey(r)
	if key == "" {
		return
	}
	read := r.Handoff.Documentation.read
	if read == nil || read.generation != m.toolDocumentation.generation || read.key != key {
		return
	}
	m.toolDocumentation.documents, _ = SelectToolDocumentation(append(m.toolDocumentation.documents, *r.Handoff))
}

func (m *MutableState) ToolDocumentationReady() bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.toolDocumentation.documents) > 0
}

func toolDocumentationProfile(rm *RequestModel) string {
	if rm == nil || rm.ToolDocumentationRequest == nil || ValidateToolDocumentationRequest(rm) != nil {
		return ""
	}
	// Bind against the complete request, not just the new domain flag. A later
	// reanalysis adding source/runtime obligations cannot reuse an old closure.
	b, err := json.Marshal(rm)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(b)
	return hex.EncodeToString(digest[:])
}

// AcceptToolDocumentationCompletion seals only the accepted model-tool tail.
// System force-completion and raw display carriers never call this method.
func (m *MutableState) AcceptToolDocumentationCompletion(rm *RequestModel) {
	if m == nil {
		return
	}
	profile := toolDocumentationProfile(rm)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolDocumentation.accepted = nil
	if profile == "" || !m.investigationComplete || len(m.toolDocumentation.documents) == 0 {
		return
	}
	m.toolDocumentation.accepted = &toolDocumentationCompletion{
		generation: m.toolDocumentation.generation, completion: m.investigationCompleteGeneration,
		profile: profile, only: ToolDocumentationOnlyRequested(rm), documents: CloneToolHandoffCarriers(m.toolDocumentation.documents),
	}
}

func (m *MutableState) acceptedToolDocumentationLocked() *toolDocumentationCompletion {
	a := m.toolDocumentation.accepted
	if a == nil || a.generation != m.toolDocumentation.generation || a.completion != m.investigationCompleteGeneration || !m.investigationComplete ||
		a.profile != toolDocumentationProfile(m.requestModel) {
		return nil
	}
	return a
}

func (m *MutableState) HasAcceptedToolDocumentationCompletion(rm *RequestModel) bool {
	if m == nil {
		return false
	}
	profile := toolDocumentationProfile(rm)
	m.mu.RLock()
	defer m.mu.RUnlock()
	a := m.acceptedToolDocumentationLocked()
	return a != nil && profile != "" && a.profile == profile
}

func (m *MutableState) AcceptedToolDocumentationCarriers() []ToolHandoffCarrier {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if a := m.acceptedToolDocumentationLocked(); a != nil {
		return CloneToolHandoffCarriers(a.documents)
	}
	return nil
}

// AcceptedToolDocumentationCompletion is deliberately not a JSON-settable
// boolean. Only the current MutableState can seal this Turn A handoff.
func AcceptedToolDocumentationCompletion(ta *TurnAArtifacts) bool {
	return ta != nil && ta.toolDocumentationCompletion != nil && ta.toolDocumentationCompletion.only
}

func (m *MutableState) mergeToolDocumentationLocked(fork toolDocumentationState, decided bool) {
	if fork.generation != m.toolDocumentation.generation {
		return
	}
	m.toolDocumentation.documents, _ = SelectToolDocumentation(append(m.toolDocumentation.documents, fork.documents...))
	if decided {
		m.toolDocumentation.accepted = nil
		if a := fork.accepted; a != nil {
			copy := *a
			copy.completion = m.investigationCompleteGeneration
			m.toolDocumentation.accepted = &copy
		}
	}
}
