package types

// EffectiveDiagramContract applies the current grounded-structure
// support to a static DiagramContract. The analyzer may correctly
// infer that a question is structural, but by finalization time the
// grounded evidence surface can still be too incomplete to support a
// hard-required diagram honestly. In that case we keep the diagram as
// a preference, but drop the hard requirement instead of forcing the
// finalizer into impossible retries.
func EffectiveDiagramContract(contract *DiagramContract, supportedKinds []DiagramKind) *DiagramContract {
	if contract == nil {
		return nil
	}
	out := *contract
	supported := normalizeSupportedDiagramKinds(supportedKinds)
	if out.Required && len(supported) == 0 {
		out.Required = false
		return &out
	}
	if len(out.PreferredKinds) == 0 {
		if len(supported) > 0 {
			out.PreferredKinds = supported
		}
		return &out
	}
	supportedSet := make(map[DiagramKind]bool, len(supported))
	for _, kind := range supported {
		supportedSet[kind] = true
	}
	filtered := make([]DiagramKind, 0, len(out.PreferredKinds))
	for _, kind := range out.PreferredKinds {
		if supportedSet[kind] {
			filtered = append(filtered, kind)
		}
	}
	if len(filtered) > 0 {
		out.PreferredKinds = filtered
	} else if len(supported) > 0 {
		out.PreferredKinds = supported
	}
	return &out
}

// SupportedDiagramKindsForAnswer derives which diagram kinds the
// current grounded evidence surface can support without inventing
// structure. This is deliberately structural rather than lexical:
// it reads validated logs, flow findings, answer chains, and
// validated config-trace diagram roles instead of matching
// repo-specific keywords.
func SupportedDiagramKindsForAnswer(
	scenario Scenario,
	stableAbsent bool,
	contract *ExactResolutionContract,
	requiredFiles []string,
	logBundle *LogBundle,
	flowFindings []FlowFindingDigest,
	answerChains []AnswerChain,
	evidence []EvidenceItem,
) []DiagramKind {
	var out []DiagramKind
	add := func(kind DiagramKind) {
		if kind == DiagramNone || !kind.IsValid() {
			return
		}
		for _, existing := range out {
			if existing == kind {
				return
			}
		}
		out = append(out, kind)
	}
	if resolvedLogDiagramSupported(logBundle) {
		add(DiagramCallDAG)
		add(DiagramSequence)
	}
	if flowDiagramSupported(flowFindings) {
		add(DiagramFlow)
	}
	if architectureDiagramSupported(answerChains) {
		add(DiagramArchitecture)
	}
	if configTraceDiagramSupported(scenario, stableAbsent, contract, requiredFiles, evidence) {
		add(DiagramArchitecture)
		add(DiagramFlow)
	}
	return out
}

func normalizeSupportedDiagramKinds(kinds []DiagramKind) []DiagramKind {
	var out []DiagramKind
	for _, kind := range kinds {
		if kind == DiagramNone || !kind.IsValid() {
			continue
		}
		duplicate := false
		for _, existing := range out {
			if existing == kind {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, kind)
		}
	}
	return out
}

func resolvedLogDiagramSupported(bundle *LogBundle) bool {
	if bundle == nil {
		return false
	}
	count := 0
	var walk func(err *LogError)
	walk = func(err *LogError) {
		if err == nil {
			return
		}
		for _, frame := range err.Frames {
			if frame.File != "" && frame.Line > 0 {
				count++
				if count >= 2 {
					return
				}
			}
		}
		walk(err.Cause)
	}
	for i := range bundle.Errors {
		walk(&bundle.Errors[i])
		if count >= 2 {
			return true
		}
	}
	return false
}

func flowDiagramSupported(findings []FlowFindingDigest) bool {
	for _, ff := range findings {
		if len(ff.Path) > 0 || len(ff.Hops) > 0 || len(ff.Conditions) > 0 {
			return true
		}
	}
	return false
}

func architectureDiagramSupported(chains []AnswerChain) bool {
	for _, chain := range chains {
		if DiagramEvidenceEligible(chain.Item) {
			return true
		}
	}
	return false
}

func DiagramEvidenceEligible(ev EvidenceItem) bool {
	if ev.Source == "" {
		return false
	}
	if ev.ContextRole == EvidenceContextRoleIllustrativeOnly ||
		ev.ContextRole == EvidenceContextRoleAbsenceSupport {
		return false
	}
	if ev.Kind == EvidenceUnresolved || ev.Kind == EvidenceTruncated {
		return false
	}
	if ev.GroundingStatus == GroundingUngrounded {
		return false
	}
	if ev.LineStart <= 0 && ev.GroundingStatus == GroundingRecovered {
		return false
	}
	return true
}

func configTraceDiagramSupported(
	scenario Scenario,
	stableAbsent bool,
	contract *ExactResolutionContract,
	requiredFiles []string,
	evidence []EvidenceItem,
) bool {
	if scenario != ScenarioConfigTrace {
		return false
	}
	if stableAbsent && contract == nil {
		return false
	}
	roles := make(map[EvidenceDiagramRole]bool, 4)
	for _, item := range evidence {
		role := ConfigTraceValidatedDiagramRoleInFiles(contract, item, requiredFiles)
		if role == EvidenceDiagramRoleUnknown {
			continue
		}
		roles[role] = true
		if len(roles) >= 2 {
			return true
		}
	}
	return false
}
