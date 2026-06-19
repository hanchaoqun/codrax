package types

import (
	"sort"
	"strings"
)

type TruthLedgerState string

const (
	TruthLedgerUnknown    TruthLedgerState = "unknown"
	TruthLedgerCovered    TruthLedgerState = "covered"
	TruthLedgerWeak       TruthLedgerState = "weak"
	TruthLedgerUnverified TruthLedgerState = "unverified"
	TruthLedgerFailed     TruthLedgerState = "failed"
)

type TruthRecommendedAction string

const (
	TruthActionContinue TruthRecommendedAction = "continue"
	TruthActionVerify   TruthRecommendedAction = "verify"
	TruthActionAddProof TruthRecommendedAction = "add_proof"
	TruthActionRepair   TruthRecommendedAction = "repair"
	TruthActionLocalize TruthRecommendedAction = "localize"
	TruthActionBlock    TruthRecommendedAction = "block"
)

type TruthSignalKind string

const (
	TruthSignalRuntimeObservation TruthSignalKind = "runtime_observation"
	TruthSignalProofCoverage      TruthSignalKind = "proof_coverage"
	TruthSignalPatchReview        TruthSignalKind = "patch_review"
	TruthSignalImpact             TruthSignalKind = "impact"
	TruthSignalLocalization       TruthSignalKind = "localization"
	TruthSignalNavigation         TruthSignalKind = "navigation"
)

type TruthSignal struct {
	Kind              TruthSignalKind        `json:"kind,omitempty"`
	State             TruthLedgerState       `json:"state,omitempty"`
	ReasonCode        string                 `json:"reason_code,omitempty"`
	RecommendedAction TruthRecommendedAction `json:"recommended_action,omitempty"`
	Source            string                 `json:"source,omitempty"`
	Paths             []string               `json:"paths,omitempty"`
	EvidenceRefs      []string               `json:"evidence_refs,omitempty"`
	HardBlock         bool                   `json:"hard_block,omitempty"`
	RequiresAction    bool                   `json:"requires_action,omitempty"`
}

// TruthLedger is the write-loop terminal authority projection. It merges runtime
// observation, patch review, impact, localization, navigation, and proof into a
// single precedence view. Producers may still keep their native artifact shape;
// controller/final-report/eval consumers can read this compact ledger instead
// of reassembling meaning from scattered fields or rendered prose.
type TruthLedger struct {
	State             TruthLedgerState               `json:"state,omitempty"`
	ReasonCode        string                         `json:"reason_code,omitempty"`
	RecommendedAction TruthRecommendedAction         `json:"recommended_action,omitempty"`
	CompletionVerdict WriteWorkflowCompletionVerdict `json:"completion_verdict,omitempty"`
	Signals           []TruthSignal                  `json:"signals,omitempty"`
	Failed            bool                           `json:"failed,omitempty"`
	Weak              bool                           `json:"weak,omitempty"`
	Unverified        bool                           `json:"unverified,omitempty"`
	Covered           bool                           `json:"covered,omitempty"`
}

func NormalizeTruthLedger(in TruthLedger) TruthLedger {
	in.State = NormalizeTruthLedgerState(in.State)
	in.ReasonCode = strings.TrimSpace(in.ReasonCode)
	in.RecommendedAction = NormalizeTruthRecommendedAction(in.RecommendedAction)
	in.Signals = NormalizeTruthSignals(in.Signals)
	if in.State == TruthLedgerUnknown {
		in.State = inferTruthLedgerState(in.Signals)
	}
	if in.RecommendedAction == "" {
		in.RecommendedAction = truthActionForState(in.State)
	}
	if in.ReasonCode == "" {
		in.ReasonCode = truthReasonForState(in.State)
	}
	switch in.State {
	case TruthLedgerFailed:
		in.Failed = true
		in.Weak = false
		in.Unverified = false
		in.Covered = false
	case TruthLedgerWeak:
		in.Weak = true
		in.Failed = false
		in.Unverified = false
		in.Covered = false
	case TruthLedgerUnverified:
		in.Unverified = true
		in.Failed = false
		in.Weak = false
		in.Covered = false
	case TruthLedgerCovered:
		in.Covered = true
		in.Failed = false
		in.Weak = false
		in.Unverified = false
	default:
		in.Failed = false
		in.Weak = false
		in.Unverified = false
		in.Covered = false
	}
	if in.CompletionVerdict == "" {
		switch in.State {
		case TruthLedgerCovered:
			in.CompletionVerdict = WriteWorkflowCompletionVerified
		case TruthLedgerUnverified, TruthLedgerWeak:
			in.CompletionVerdict = WriteWorkflowCompletionUnverified
		}
	}
	return in
}

func NormalizeTruthLedgerState(state TruthLedgerState) TruthLedgerState {
	switch TruthLedgerState(strings.ToLower(strings.TrimSpace(string(state)))) {
	case TruthLedgerCovered:
		return TruthLedgerCovered
	case TruthLedgerWeak:
		return TruthLedgerWeak
	case TruthLedgerUnverified:
		return TruthLedgerUnverified
	case TruthLedgerFailed:
		return TruthLedgerFailed
	default:
		return TruthLedgerUnknown
	}
}

func NormalizeTruthRecommendedAction(action TruthRecommendedAction) TruthRecommendedAction {
	switch TruthRecommendedAction(strings.ToLower(strings.TrimSpace(string(action)))) {
	case TruthActionContinue:
		return TruthActionContinue
	case TruthActionVerify:
		return TruthActionVerify
	case TruthActionAddProof:
		return TruthActionAddProof
	case TruthActionRepair:
		return TruthActionRepair
	case TruthActionLocalize:
		return TruthActionLocalize
	case TruthActionBlock:
		return TruthActionBlock
	default:
		return ""
	}
}

func NormalizeTruthSignals(in []TruthSignal) []TruthSignal {
	out := make([]TruthSignal, 0, len(in))
	seen := map[string]bool{}
	for _, signal := range in {
		signal.Kind = normalizeTruthSignalKind(signal.Kind)
		signal.State = NormalizeTruthLedgerState(signal.State)
		signal.ReasonCode = strings.TrimSpace(signal.ReasonCode)
		signal.RecommendedAction = NormalizeTruthRecommendedAction(signal.RecommendedAction)
		signal.Source = strings.TrimSpace(signal.Source)
		signal.Paths = dedupTruthStrings(signal.Paths)
		signal.EvidenceRefs = dedupTruthStrings(signal.EvidenceRefs)
		if signal.Kind == "" && signal.State == TruthLedgerUnknown && signal.ReasonCode == "" {
			continue
		}
		key := strings.Join([]string{
			string(signal.Kind),
			string(signal.State),
			signal.ReasonCode,
			signal.Source,
			strings.Join(signal.Paths, ","),
			strings.Join(signal.EvidenceRefs, ","),
		}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, signal)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if truthStateRank(out[i].State) != truthStateRank(out[j].State) {
			return truthStateRank(out[i].State) < truthStateRank(out[j].State)
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ReasonCode < out[j].ReasonCode
	})
	return out
}

func normalizeTruthSignalKind(kind TruthSignalKind) TruthSignalKind {
	switch TruthSignalKind(strings.ToLower(strings.TrimSpace(string(kind)))) {
	case TruthSignalRuntimeObservation,
		TruthSignalProofCoverage,
		TruthSignalPatchReview,
		TruthSignalImpact,
		TruthSignalLocalization,
		TruthSignalNavigation:
		return TruthSignalKind(strings.ToLower(strings.TrimSpace(string(kind))))
	default:
		return ""
	}
}

func inferTruthLedgerState(signals []TruthSignal) TruthLedgerState {
	state := TruthLedgerUnknown
	for _, signal := range signals {
		if truthStateRank(signal.State) < truthStateRank(state) {
			state = signal.State
		}
	}
	return state
}

func truthActionForState(state TruthLedgerState) TruthRecommendedAction {
	switch state {
	case TruthLedgerFailed:
		return TruthActionRepair
	case TruthLedgerWeak:
		return TruthActionAddProof
	case TruthLedgerUnverified, TruthLedgerCovered:
		return TruthActionContinue
	default:
		return TruthActionVerify
	}
}

func truthReasonForState(state TruthLedgerState) string {
	switch state {
	case TruthLedgerFailed:
		return "truth_failed"
	case TruthLedgerWeak:
		return "truth_weak"
	case TruthLedgerUnverified:
		return "truth_unverified"
	case TruthLedgerCovered:
		return "truth_covered"
	default:
		return "truth_unknown"
	}
}

func truthStateRank(state TruthLedgerState) int {
	switch state {
	case TruthLedgerFailed:
		return 0
	case TruthLedgerWeak:
		return 1
	case TruthLedgerUnverified:
		return 2
	case TruthLedgerUnknown:
		return 3
	case TruthLedgerCovered:
		return 4
	default:
		return 3
	}
}

func dedupTruthStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
