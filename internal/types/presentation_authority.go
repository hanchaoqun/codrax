package types

import "strings"

// presentation_authority.go — the single typed current-turn presentation
// carrier (colleague_merge_audit §40.54, V7-4).
//
// Two current-turn presentation values travel BusContext → AgentContext →
// tool projection: the byte-anchored directive span (free-form display
// guidance — it may ask for a table, JSON, or prose) and the precise
// hard-visual boolean minted by the turn classifier. Before V7-4 the
// analyzer prompt rendered only the directive prose while the emit_analysis
// hard gate keyed on the boolean it never showed, so the model was taught to
// judge "does the directive ask for a diagram" from words and then silently
// normalized on a success path. Both sides now read ONE value object through
// ONE accessor (`PresentationAuthority()`): the prompt renders
// `HardVisual()` as a typed line from the label table below, and the gate
// reads `DiagramRequired` from the same struct. A census
// (presentation_authority_census_test.go) pins that no other reader of the
// boolean exists and that no LLM-facing literal names the wire field.
//
// Nothing here infers the boolean from directive words (context.go rule).

// PresentationDirectiveSectionTitle is the prompt section title under which
// the typed presentation authority is rendered. The context builder and the
// analyzer teaching both name the section through this constant.
const PresentationDirectiveSectionTitle = "Presentation Directive"

// PresentationAuthority is the typed current-turn presentation carrier.
type PresentationAuthority struct {
	// Directive is the verbatim current-request span selected as display
	// guidance (already trimmed). It chooses presentation FORM only.
	Directive string
	// DiagramRequired is the precise hard-visual authority. It is never
	// derived from Directive text.
	DiagramRequired bool
}

// PresentationAuthority is the single accessor through which prompt
// construction reads the current-turn presentation carrier. The census
// pins it as the only reader of the raw boolean outside the field mirrors.
func (c *BusContext) PresentationAuthority() PresentationAuthority {
	if c == nil {
		return PresentationAuthority{}
	}
	return PresentationAuthority{
		Directive:       strings.TrimSpace(c.PresentationDirective),
		DiagramRequired: c.PresentationDiagramRequired,
	}
}

// PresentationAuthority is the AgentContext twin of the BusContext accessor;
// the hard gate and the section renderer both read through it.
func (a *AgentContext) PresentationAuthority() PresentationAuthority {
	if a == nil {
		return PresentationAuthority{}
	}
	return PresentationAuthority{
		Directive:       strings.TrimSpace(a.PresentationDirective),
		DiagramRequired: a.PresentationDiagramRequired,
	}
}

// Present reports whether either carrier is set; an absent authority renders
// no prompt section so non-diagram prompts stay byte-stable.
func (a PresentationAuthority) Present() bool {
	return a.Directive != "" || a.DiagramRequired
}

// HardVisual is the closed two-value label of the boolean, used by every
// rendering surface.
func (a PresentationAuthority) HardVisual() PresentationHardVisualState {
	if a.DiagramRequired {
		return PresentationHardVisualRequired
	}
	return PresentationHardVisualNotRequired
}

// PresentationHardVisualState is the closed set of hard-visual states.
type PresentationHardVisualState string

const (
	PresentationHardVisualRequired    PresentationHardVisualState = "required"
	PresentationHardVisualNotRequired PresentationHardVisualState = "not_required"
)

// PresentationHardVisualStates lists the closed set in declaration order.
func PresentationHardVisualStates() []PresentationHardVisualState {
	return []PresentationHardVisualState{PresentationHardVisualRequired, PresentationHardVisualNotRequired}
}

// IsValid reports membership in the closed set.
func (s PresentationHardVisualState) IsValid() bool {
	for _, known := range PresentationHardVisualStates() {
		if s == known {
			return true
		}
	}
	return false
}

// presentationLabel is one bilingual prompt label.
type presentationLabel struct {
	EN string
	ZH string
}

func (l presentationLabel) forLanguage(lang string) string {
	if PromptLanguageIsEnglish(lang) {
		return l.EN
	}
	return l.ZH
}

// presentationHardVisualLineLabel and presentationHardVisualStateLabels are
// the ONLY source of the typed line's wording. No other file spells these
// strings; teaching, section rendering, reject text, and warning text all
// derive from them.
var presentationHardVisualLineLabel = presentationLabel{EN: "Hard visual requirement", ZH: "硬性图示要求"}

var presentationHardVisualStateLabels = map[PresentationHardVisualState]presentationLabel{
	PresentationHardVisualRequired:    {EN: "required", ZH: "需要"},
	PresentationHardVisualNotRequired: {EN: "not required", ZH: "不需要"},
}

// PromptLanguageIsEnglish is the one language rule for bilingual prompt
// sections: only an explicit "en" renders English; everything else renders
// Chinese.
func PromptLanguageIsEnglish(lang string) bool {
	return strings.EqualFold(strings.TrimSpace(lang), "en")
}

// PresentationHardVisualStatement renders the typed line for one state and
// prompt language, e.g. "Hard visual requirement: required" /
// "硬性图示要求：需要". An invalid state renders the not-required line so a
// corrupt value can never mint a hard requirement.
func PresentationHardVisualStatement(state PresentationHardVisualState, lang string) string {
	if !state.IsValid() {
		state = PresentationHardVisualNotRequired
	}
	if PromptLanguageIsEnglish(lang) {
		return presentationHardVisualLineLabel.EN + ": " + presentationHardVisualStateLabels[state].EN
	}
	return presentationHardVisualLineLabel.ZH + "：" + presentationHardVisualStateLabels[state].ZH
}

// PresentationHardVisualTeaching is the single R2' teaching sentence for
// `diagram_hint.required`. It is embedded verbatim in the analysis skill
// prompt, the emit_analysis schema description, the missing-field reject,
// and the true→false normalization warning, so every surface names the same
// two carriers: the typed line rendered in the Presentation Directive
// section, and a required verbatim diagram dimension authored by the model.
func PresentationHardVisualTeaching() string {
	required := PresentationHardVisualRequired
	return "Set diagram_hint.required=true only when the \"" + PresentationDirectiveSectionTitle +
		"\" section carries the line `" + PresentationHardVisualStatement(required, "en") +
		"` (`" + PresentationHardVisualStatement(required, "zh") + "`), or when you emit a required " +
		"requested_answer_dimensions row with role=diagram whose source_quote is verbatim from the CURRENT request; " +
		"otherwise set required=false. A true value without either carrier is normalized to false and the " +
		"diagram family remains optional guidance. The directive text chooses only the diagram family or " +
		"presentation form; it never decides required."
}
