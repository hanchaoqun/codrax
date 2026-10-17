package types

// Display intent is a strict subset of the selected investigation views. Its
// private identity binds the full current publication, source generation and
// exact query; the serialized selectors alone cannot recreate it.
func qualifyNativeAnswerDisplayPublication(p *RuntimeMeasurementPublication, original string, replay TraceQueryWindowReplayRef, scope *RuntimeArtifactScopeProfile) {
	if p == nil || original == "" {
		return
	}
	start, end, known := TraceObservationContinuousQueryWindow(p.Source)
	if !known {
		return
	}
	if scope.HasExplicitTimeWindows() {
		if _, exact := scope.MatchExplicitTimeWindow(start, end); !exact {
			return
		}
	}
	ref := replay.source
	if replay.preparedSource != nil {
		ref = replay.preparedSource.source
	} else if replay.nativeNavigation != nil {
		ref = replay.nativeNavigation.source
	}
	if ref.generation == nil || ref.path != p.Source.Path {
		return
	}
	p.answerDisplayIdentity = runtimeSamplingDigest(struct {
		Original, Generation string
		Publication          RuntimeMeasurementPublication
	}{original, ref.identity.CacheToken(), *p})
}

func nativeAnswerDisplayPublicationIdentities(publications []RuntimeMeasurementPublication) map[string]string {
	out, conflicts := map[string]string{}, map[string]bool{}
	for _, p := range publications {
		if p.answerDisplayIdentity == "" {
			continue
		}
		if old := out[p.ObservationID]; old != "" && old != p.answerDisplayIdentity {
			conflicts[p.ObservationID] = true
		}
		out[p.ObservationID] = p.answerDisplayIdentity
	}
	for id := range conflicts {
		delete(out, id)
	}
	return out
}

func nativeAnswerDisplayKey(r AnswerRuntimeMeasurementReceipt) string {
	return r.ObservationID + "\x00" + string(r.View)
}

// NativeAnswerDisplayChoices offers layout intent only in an existing typed
// requested-fact domain. Supplementary targets, unknown windows and legacy
// JSON publications remain selectable for investigation, not automatic output.
func NativeAnswerDisplayChoices(rm *RequestModel, c *RuntimeMeasurementContract) []AnswerRuntimeMeasurementReceipt {
	if !RuntimeNativeFactDisplayDomain(rm) || c == nil {
		return nil
	}
	var out []AnswerRuntimeMeasurementReceipt
	for _, table := range c.Choices() {
		if table.EventData == nil && c.answerDisplayIdentities[table.ObservationID] != "" {
			out = append(out, AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View})
		}
	}
	return out
}

// SealInvestigationNativeAnswerDisplay runs at the accepted-tool preparation
// boundary. False rejects just the explicit optional layout instruction; it
// does not add an answer requirement when the field was omitted.
func SealInvestigationNativeAnswerDisplay(h *InvestigationNativeFactHandoff, rm *RequestModel, c *RuntimeMeasurementContract) bool {
	if h == nil {
		return false
	}
	h.answerDisplayBindings = nil
	if len(h.AnswerDisplay) == 0 {
		return true
	}
	if len(h.AnswerDisplay) > 16 {
		return false
	}
	choices, selected := map[string]bool{}, map[string]bool{}
	for _, r := range NativeAnswerDisplayChoices(rm, c) {
		choices[nativeAnswerDisplayKey(r)] = true
	}
	for _, r := range h.RuntimeMeasurements {
		selected[nativeAnswerDisplayKey(r)] = true
	}
	bindings := map[string]string{}
	for _, r := range h.AnswerDisplay {
		key := nativeAnswerDisplayKey(r)
		if !choices[key] || !selected[key] || bindings[key] != "" || r.EventSelection != nil || !BindRuntimeMeasurementReceipt(&r, c) {
			return false
		}
		bindings[key] = nativeAnswerDisplayBinding(r, c)
	}
	h.answerDisplayBindings = bindings
	return true
}

func nativeAnswerDisplayBinding(r AnswerRuntimeMeasurementReceipt, c *RuntimeMeasurementContract) string {
	return runtimeSamplingDigest(struct {
		Publication string
		Table       *RuntimeMeasurementTable
	}{c.answerDisplayIdentities[r.ObservationID], r.BoundTable})
}

// ResolveInvestigationNativeAnswerDisplay never restores private acceptance
// from JSON. Current binding is independent of prompt-preview completeness;
// missing views remain disclosed, never filled from old copied values.
func ResolveInvestigationNativeAnswerDisplay(h *InvestigationNativeFactHandoff, rm *RequestModel, c *RuntimeMeasurementContract) (out []AnswerRuntimeMeasurementReceipt, unavailable int) {
	if h == nil || len(h.answerDisplayBindings) == 0 || !RuntimeNativeFactDisplayDomain(rm) {
		return nil, 0
	}
	choices := map[string]bool{}
	for _, r := range NativeAnswerDisplayChoices(rm, c) {
		choices[nativeAnswerDisplayKey(r)] = true
	}
	for _, r := range h.AnswerDisplay {
		key := nativeAnswerDisplayKey(r)
		if !choices[key] || !BindRuntimeMeasurementReceipt(&r, c) || h.answerDisplayBindings[key] != nativeAnswerDisplayBinding(r, c) {
			unavailable++
			continue
		}
		out = append(out, r)
	}
	return out, unavailable
}

const NativeAnswerDisplayTeaching = "Optional answer_display selects only the runtime_measurements views that the user needs in the final answer, not every view inspected during investigation. Copy a subset of the current offered pairs. The system will preserve their complete retained rows and notes after current-source rebinding; prompt previews are not output limits. The finalizer may place the same complete view using its supported table/summary/fact-diagram surface. This is layout intent only: it adds no complete-member, requested-dimension, source-code or causal authority. Independent interpretation remains separate."
