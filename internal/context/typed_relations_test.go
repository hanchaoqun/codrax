package context

import (
	"strings"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// buildLooperGraphForTypedRelations mirrors the s5a-style graph used
// in orchestrator's structural-divergence tests: one Looper interface
// + 3 concrete types whose method set satisfies it via P0a/P0b.
func buildLooperGraphForTypedRelations(t *testing.T) *repotypes.Graph {
	t.Helper()
	return buildLooperGraphForTypedRelationsLang(t, "go", "interface")
}

func buildLooperGraphForTypedRelationsLang(t *testing.T, lang, ifaceKind string) *repotypes.Graph {
	t.Helper()
	ifaceFile := &repotypes.FileInfo{RelPath: "iface.go", Language: lang}
	a := &repotypes.FileInfo{RelPath: "impl_alpha.go", Language: lang}
	b := &repotypes.FileInfo{RelPath: "impl_beta.go", Language: lang}
	c := &repotypes.FileInfo{RelPath: "impl_gamma.go", Language: lang}

	ifaceSym := repotypes.Symbol{Name: "Looper", Kind: ifaceKind, File: "iface.go", Line: 7}
	ifaceSym.ID = repotypes.DeriveSymbolID(ifaceFile, &ifaceSym)
	ifaceFile.Symbols = []repotypes.Symbol{ifaceSym}

	alpha := repotypes.Symbol{Name: "alpha", Kind: "struct", File: "impl_alpha.go", Line: 14, Implements: []repotypes.SymbolID{ifaceSym.ID}}
	alpha.ID = repotypes.DeriveSymbolID(a, &alpha)
	a.Symbols = []repotypes.Symbol{alpha}

	beta := repotypes.Symbol{Name: "beta", Kind: "struct", File: "impl_beta.go", Line: 22, Implements: []repotypes.SymbolID{ifaceSym.ID}}
	beta.ID = repotypes.DeriveSymbolID(b, &beta)
	b.Symbols = []repotypes.Symbol{beta}

	gamma := repotypes.Symbol{Name: "gamma", Kind: "struct", File: "impl_gamma.go", Line: 31, Implements: []repotypes.SymbolID{ifaceSym.ID}}
	gamma.ID = repotypes.DeriveSymbolID(c, &gamma)
	c.Symbols = []repotypes.Symbol{gamma}

	return &repotypes.Graph{
		Files:      []*repotypes.FileInfo{ifaceFile, a, b, c},
		FileIndex:  map[string]*repotypes.FileInfo{"iface.go": ifaceFile, "impl_alpha.go": a, "impl_beta.go": b, "impl_gamma.go": c},
		SymbolDefs: map[string][]*repotypes.Symbol{"Looper": {&ifaceFile.Symbols[0]}},
		SymbolByID: map[repotypes.SymbolID]*repotypes.Symbol{
			ifaceSym.ID: &ifaceFile.Symbols[0],
			alpha.ID:    &a.Symbols[0],
			beta.ID:     &b.Symbols[0],
			gamma.ID:    &c.Symbols[0],
		},
	}
}

func enumerationRequestModel() *types.RequestModel {
	rm := &types.RequestModel{}
	rm.Predicates.IsCategoryEnumeration = true
	rm.AnalyzerHints.PrimaryEntities = []string{"Looper"}
	return rm
}

func TestProbeTypedRelations_CategoryEnumerationFires(t *testing.T) {
	g := buildLooperGraphForTypedRelations(t)
	hints := ProbeTypedRelations(g, enumerationRequestModel())
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint; got %d", len(hints))
	}
	h := hints[0]
	if h.Relation != "implements" || h.SourceName != "Looper" || h.SourceKind != "interface" {
		t.Errorf("unexpected hint header: %+v", h)
	}
	if len(h.Members) != 3 {
		t.Fatalf("expected 3 members; got %d (%v)", len(h.Members), h.Members)
	}
	// Stable alphabetic order.
	wantOrder := []string{"alpha", "beta", "gamma"}
	for i, m := range h.Members {
		if m.Name != wantOrder[i] {
			t.Errorf("member[%d] name=%q; want %q", i, m.Name, wantOrder[i])
		}
	}
}

func TestProbeTypedRelations_NotEnumerationSkips(t *testing.T) {
	g := buildLooperGraphForTypedRelations(t)
	rm := &types.RequestModel{} // all predicates false
	rm.AnalyzerHints.PrimaryEntities = []string{"Looper"}
	if hints := ProbeTypedRelations(g, rm); hints != nil {
		t.Errorf("expected no hints when no enumeration/relational/count predicate; got %v", hints)
	}
}

func TestProbeTypedRelations_NoMatchingInterfaceSkips(t *testing.T) {
	g := buildLooperGraphForTypedRelations(t)
	rm := enumerationRequestModel()
	rm.AnalyzerHints.PrimaryEntities = []string{"NotInGraph"}
	if hints := ProbeTypedRelations(g, rm); hints != nil {
		t.Errorf("expected no hints when entity does not name an interface; got %v", hints)
	}
}

func TestProbeTypedRelations_DerivedContextEntityDoesNotSeedHint(t *testing.T) {
	g := buildLooperGraphForTypedRelations(t)
	rm := enumerationRequestModel()
	rm.AnalyzerHints.MentionedEntities = []string{"internal/analysis"}
	rm.AnalyzerHints.PrimaryEntities = []string{"internal/analysis"}
	rm.AnalyzerHints.Entities = []string{"internal/analysis", "Looper"}
	rm.AnalyzerHints.DerivedEntities = []string{"Looper"}
	if hints := ProbeTypedRelations(g, rm); hints != nil {
		t.Errorf("derived/context relation entity must remain a search hint, got %+v", hints)
	}
}

func TestProbeTypedRelations_PromptHintCanUseEntitiesWhenNoProvenanceLane(t *testing.T) {
	g := buildLooperGraphForTypedRelations(t)
	rm := &types.RequestModel{
		PredicateAxis: types.AxisImplement,
		AnalyzerHints: types.AnalyzerHints{
			Entities:        []string{"Looper", "alpha", "beta"},
			DerivedEntities: []string{"alpha", "beta"},
		},
	}
	hints := ProbeTypedRelations(g, rm)
	if len(hints) != 1 {
		t.Fatalf("expected exact graph probe to recover relation source from Entities; got %d", len(hints))
	}
	if hints[0].SourceName != "Looper" || len(hints[0].Members) != 3 {
		t.Fatalf("unexpected relation hint: %+v", hints[0])
	}
}

func TestProbeTypedRelations_NilGraphSkips(t *testing.T) {
	if hints := ProbeTypedRelations(nil, enumerationRequestModel()); hints != nil {
		t.Errorf("expected no hints when graph is nil; got %v", hints)
	}
	if hints := ProbeTypedRelations(buildLooperGraphForTypedRelations(t), nil); hints != nil {
		t.Errorf("expected no hints when rm is nil; got %v", hints)
	}
}

func TestProbeTypedRelations_RelationalLookupAlsoFires(t *testing.T) {
	g := buildLooperGraphForTypedRelations(t)
	rm := &types.RequestModel{}
	rm.Predicates.IsRelationalLookup = true
	rm.AnalyzerHints.PrimaryEntities = []string{"Looper"}
	if hints := ProbeTypedRelations(g, rm); len(hints) != 1 {
		t.Errorf("expected 1 hint on IsRelationalLookup; got %d", len(hints))
	}
}

func TestProbeTypedRelations_CountQuestionAlsoFires(t *testing.T) {
	g := buildLooperGraphForTypedRelations(t)
	rm := &types.RequestModel{}
	rm.Predicates.IsCountQuestion = true
	rm.AnalyzerHints.PrimaryEntities = []string{"Looper"}
	if hints := ProbeTypedRelations(g, rm); len(hints) != 1 {
		t.Errorf("expected 1 hint on IsCountQuestion; got %d", len(hints))
	}
}

func TestProbeTypedRelations_ImplementAxisFiresForDiagramMechanism(t *testing.T) {
	g := buildLooperGraphForTypedRelations(t)
	rm := &types.RequestModel{
		Intent:        types.IntentExplain,
		Scenario:      types.ScenarioArchitectureExplain,
		PredicateAxis: types.AxisImplement,
		DiagramHint:   &types.DiagramHint{Kind: types.DiagramArchitecture},
		AnalyzerHints: types.AnalyzerHints{PrimaryEntities: []string{"Looper"}},
	}
	hints := ProbeTypedRelations(g, rm)
	if len(hints) != 1 {
		t.Fatalf("expected implement-axis relation hint; got %d", len(hints))
	}
	if len(hints[0].Members) != 3 {
		t.Fatalf("expected 3 implementers for diagram/mechanism relation; got %+v", hints[0].Members)
	}
}

func TestProbeTypedRelations_InterfaceDiagramShapeFiresWhenAxisDriftsToDefine(t *testing.T) {
	g := buildLooperGraphForTypedRelations(t)
	rm := &types.RequestModel{
		Intent:        types.IntentExplain,
		Scenario:      types.ScenarioArchitectureExplain,
		PredicateAxis: types.AxisDefine,
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectInterface},
		DiagramHint:   &types.DiagramHint{Kind: types.DiagramArchitecture},
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"Looper", "internal/agent/agent.go"},
		},
	}
	hints := ProbeTypedRelations(g, rm)
	if len(hints) != 1 {
		t.Fatalf("expected interface-diagram relation hint despite axis drift; got %d", len(hints))
	}
	if hints[0].SourceName != "Looper" || len(hints[0].Members) != 3 {
		t.Fatalf("unexpected relation hint: %+v", hints[0])
	}
}

func TestProbeTypedRelations_InterfaceDiagramUsesResolvedGraphSourceWhenAnalyzerSubjectMissing(t *testing.T) {
	g := buildLooperGraphForTypedRelations(t)
	rm := &types.RequestModel{
		Intent:      types.IntentExplain,
		Scenario:    types.ScenarioArchitectureExplain,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture},
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"Looper"},
		},
	}
	hints := ProbeTypedRelations(g, rm)
	if len(hints) != 1 {
		t.Fatalf("expected exact interface source fact to unlock relation hint; got %d", len(hints))
	}
	if hints[0].Relation != types.TypedRelationImplements || hints[0].SourceName != "Looper" {
		t.Fatalf("unexpected relation hint: %+v", hints[0])
	}
	if len(hints[0].Members) != 3 {
		t.Fatalf("expected all implementers from exact graph source, got %+v", hints[0].Members)
	}
}

func TestProbeTypedRelations_ImplementAxisFiresForAllSupportedLanguages(t *testing.T) {
	for _, lang := range repotypes.SupportedReadLanguages() {
		ifaceKind := "interface"
		switch lang {
		case repotypes.LangRust:
			ifaceKind = "trait"
		case repotypes.LangSwift:
			ifaceKind = "protocol"
		}
		t.Run(lang, func(t *testing.T) {
			g := buildLooperGraphForTypedRelationsLang(t, lang, ifaceKind)
			rm := &types.RequestModel{
				PredicateAxis: types.AxisImplement,
				AnalyzerHints: types.AnalyzerHints{PrimaryEntities: []string{"Looper"}},
			}
			hints := ProbeTypedRelations(g, rm)
			if len(hints) != 1 {
				t.Fatalf("expected relation hint for %s; got %d", lang, len(hints))
			}
			if hints[0].SourceKind != ifaceKind {
				t.Fatalf("source kind for %s = %q, want %q", lang, hints[0].SourceKind, ifaceKind)
			}
		})
	}
}

type fakeTypedRelationCandidateSource struct {
	rows  []types.TypedRelationCandidate
	facts []types.TypedRelationSourceFact
}

func (f fakeTypedRelationCandidateSource) TypedRelationSourceFacts(sources []string) []types.TypedRelationSourceFact {
	var out []types.TypedRelationSourceFact
	for _, fact := range f.facts {
		for _, source := range sources {
			if strings.EqualFold(fact.Name, source) {
				out = append(out, fact)
				break
			}
		}
	}
	return out
}

func (f fakeTypedRelationCandidateSource) TypedRelationCandidates(q types.TypedRelationQuery) []types.TypedRelationCandidate {
	var out []types.TypedRelationCandidate
	for _, row := range f.rows {
		if !q.AllowsKind(row.Relation) {
			continue
		}
		for _, source := range q.Sources {
			if strings.EqualFold(row.SourceName, source) {
				out = append(out, row)
				break
			}
		}
	}
	return out
}

func TestProbeTypedRelations_InterfaceDiagramUsesResolvedProviderSourceFacts(t *testing.T) {
	provider := fakeTypedRelationCandidateSource{
		facts: []types.TypedRelationSourceFact{{
			Name:      "Looper",
			Kind:      "interface",
			File:      "iface.go",
			Line:      7,
			Carrier:   types.TypedRelationCarrierMultiGraph,
			Precision: types.TypedRelationPrecisionExactSymbolID,
		}},
		rows: []types.TypedRelationCandidate{{
			Relation:   types.TypedRelationImplements,
			SourceName: "Looper",
			SourceKind: "interface",
			Member:     types.TypedRelationMember{Name: "alpha", File: "impl_alpha.go", Line: 14, Kind: "struct"},
			Carrier:    types.TypedRelationCarrierMultiGraph,
			Precision:  types.TypedRelationPrecisionExactSymbolID,
		}},
	}
	rm := &types.RequestModel{
		Intent:      types.IntentExplain,
		Scenario:    types.ScenarioArchitectureExplain,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture},
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"Looper"},
		},
	}
	hints := ProbeTypedRelations(provider, rm)
	if len(hints) != 1 {
		t.Fatalf("expected provider source fact to unlock relation hint; got %d", len(hints))
	}
	if hints[0].Relation != types.TypedRelationImplements || len(hints[0].Members) != 1 {
		t.Fatalf("unexpected provider relation hint: %+v", hints[0])
	}
}

func TestProbeTypedRelations_UsesGenericCandidateSourceWhenGraphIsOpaque(t *testing.T) {
	provider := fakeTypedRelationCandidateSource{rows: []types.TypedRelationCandidate{
		{
			Relation:   types.TypedRelationImplements,
			SourceName: "Looper",
			SourceKind: "interface",
			Member:     types.TypedRelationMember{Name: "alpha", File: "impl_alpha.go", Line: 14, Kind: "struct"},
			Carrier:    types.TypedRelationCarrierMultiGraph,
			Precision:  types.TypedRelationPrecisionExactSymbolID,
		},
		{
			Relation:   types.TypedRelationImplements,
			SourceName: "Looper",
			SourceKind: "interface",
			Member:     types.TypedRelationMember{Name: "beta", File: "impl_beta.go", Line: 22, Kind: "struct"},
			Carrier:    types.TypedRelationCarrierMultiGraph,
			Precision:  types.TypedRelationPrecisionExactSymbolID,
		},
	}}
	rm := &types.RequestModel{
		PredicateAxis: types.AxisImplement,
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"Looper"},
		},
	}
	hints := ProbeTypedRelations(provider, rm)
	if len(hints) != 1 {
		t.Fatalf("expected provider-backed relation hint; got %d", len(hints))
	}
	if hints[0].Relation != types.TypedRelationImplements || hints[0].SourceName != "Looper" {
		t.Fatalf("unexpected hint header: %+v", hints[0])
	}
	if got := len(hints[0].Members); got != 2 {
		t.Fatalf("members=%d, want 2: %+v", got, hints[0].Members)
	}
}

func TestProbeTypedRelations_UsesCentralQueryForGenericRelationKinds(t *testing.T) {
	provider := fakeTypedRelationCandidateSource{rows: []types.TypedRelationCandidate{
		{
			Relation:   types.TypedRelationCalledBy,
			SourceName: "Run",
			SourceKind: "function",
			Member:     types.TypedRelationMember{Name: "main", File: "cmd/root.go", Line: 42, Kind: "function"},
			Carrier:    types.TypedRelationCarrierGraph,
			Precision:  types.TypedRelationPrecisionExactSymbolID,
		},
		{
			Relation:   types.TypedRelationImplements,
			SourceName: "Run",
			SourceKind: "interface",
			Member:     types.TypedRelationMember{Name: "runner", File: "runner.go", Line: 7, Kind: "struct"},
			Carrier:    types.TypedRelationCarrierGraph,
			Precision:  types.TypedRelationPrecisionExactSymbolID,
		},
	}}
	rm := &types.RequestModel{
		PredicateAxis: types.AxisCall,
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"Run"},
		},
	}
	hints := ProbeTypedRelations(provider, rm)
	if len(hints) != 1 {
		t.Fatalf("expected called-by relation hint from central query; got %d", len(hints))
	}
	if hints[0].Relation != types.TypedRelationCalledBy || hints[0].SourceName != "Run" {
		t.Fatalf("unexpected relation hint: %+v", hints[0])
	}
	if got := len(hints[0].Members); got != 1 || hints[0].Members[0].Name != "main" {
		t.Fatalf("unexpected called-by members: %+v", hints[0].Members)
	}
}

func TestTypedRelationCarrierFromBusPrefersGenericProviderOverLegacySearchGraph(t *testing.T) {
	legacy := buildLooperGraphForTypedRelations(t)
	provider := fakeTypedRelationCandidateSource{rows: []types.TypedRelationCandidate{{
		Relation:   types.TypedRelationImplements,
		SourceName: "Looper",
		SourceKind: "interface",
		Member:     types.TypedRelationMember{Name: "providerImpl", File: "provider_impl.go", Line: 9},
		Carrier:    types.TypedRelationCarrierMultiGraph,
		Precision:  types.TypedRelationPrecisionExactSymbolID,
	}}}
	bus := &types.BusContext{
		Mutable:    types.NewMutableState("test"),
		MultiGraph: provider,
	}
	bus.Mutable.SetSearchGraph(legacy)
	if _, ok := typedRelationCarrierFromBus(bus).(fakeTypedRelationCandidateSource); !ok {
		t.Fatalf("typed relation carrier should prefer generic MultiGraph provider over legacy SearchGraph")
	}
	carriers := typedRelationCarriersFromBus(bus)
	if len(carriers) != 2 {
		t.Fatalf("expected provider plus legacy graph carriers, got %d", len(carriers))
	}
	hints := ProbeTypedRelations(typedRelationCarrierFromBus(bus), enumerationRequestModel())
	if len(hints) != 1 || len(hints[0].Members) != 1 || hints[0].Members[0].Name != "providerImpl" {
		t.Fatalf("expected provider-backed hint, got %+v", hints)
	}
}

func TestTypedRelationCarrierFromBusFallsBackToLegacySearchGraph(t *testing.T) {
	legacy := buildLooperGraphForTypedRelations(t)
	bus := &types.BusContext{Mutable: types.NewMutableState("test")}
	bus.Mutable.SetSearchGraph(legacy)
	if got := typedRelationCarrierFromBus(bus); got != legacy {
		t.Fatalf("typed relation carrier should fall back to legacy SearchGraph")
	}
	hints := ProbeTypedRelations(typedRelationCarrierFromBus(bus), enumerationRequestModel())
	if len(hints) != 1 || len(hints[0].Members) != 3 {
		t.Fatalf("expected legacy graph-backed hint, got %+v", hints)
	}
}

func TestAppendTypedRelationHintsDedupsAcrossCarriers(t *testing.T) {
	a := types.TypedRelationHint{
		Relation:   types.TypedRelationImplements,
		SourceName: "Looper",
		Members: []types.TypedRelationMember{
			{Name: "alpha", File: "impl_alpha.go", Line: 14},
		},
	}
	b := types.TypedRelationHint{
		Relation:   types.TypedRelationImplements,
		SourceName: "Looper",
		Members: []types.TypedRelationMember{
			{Name: "alpha", File: "impl_alpha.go", Line: 14},
			{Name: "beta", File: "impl_beta.go", Line: 22},
		},
	}
	got := appendTypedRelationHints(nil, a)
	got = appendTypedRelationHints(got, b)
	if len(got) != 2 {
		t.Fatalf("expected two hint groups preserving provenance order, got %+v", got)
	}
	total := 0
	for _, hint := range got {
		total += len(hint.Members)
	}
	if total != 2 {
		t.Fatalf("expected duplicate alpha to be removed and beta kept, got %+v", got)
	}
	if got[1].Members[0].Name != "beta" {
		t.Fatalf("expected beta as second-carrier unique member, got %+v", got)
	}
}

// ── render-time merge tests (renderTypedRelationAppendix) ───────────

func TestRenderTypedRelationAppendix_AllNewRowsRendered(t *testing.T) {
	hints := []types.TypedRelationHint{{
		Relation:   "implements",
		SourceName: "Looper",
		SourceKind: "interface",
		Members: []types.TypedRelationMember{
			{Name: "alpha", File: "impl_alpha.go", Line: 14, Kind: "struct", Distance: 1},
			{Name: "beta", File: "impl_beta.go", Line: 22, Kind: "struct", Distance: 1},
		},
	}}
	out := renderTypedRelationAppendix(hints, nil) // no LLM evidence yet
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "impl_alpha.go:14") {
		t.Errorf("expected alpha row with file:line; got %q", out)
	}
	if !strings.Contains(out, "beta") {
		t.Errorf("expected beta row; got %q", out)
	}
	if !strings.Contains(out, "provenance=typed_graph") {
		t.Errorf("expected provenance tag; got %q", out)
	}
	if !strings.Contains(out, "anchor_kind=definition") {
		t.Errorf("expected anchor_kind tag matching implements relation; got %q", out)
	}
}

func TestRenderTypedRelationAppendix_DedupAgainstLLMEvidence(t *testing.T) {
	hints := []types.TypedRelationHint{{
		Relation:   "implements",
		SourceName: "Looper",
		SourceKind: "interface",
		Members: []types.TypedRelationMember{
			{Name: "alpha", File: "impl_alpha.go", Line: 14, Kind: "struct"},
			{Name: "beta", File: "impl_beta.go", Line: 22, Kind: "struct"},
		},
	}}
	llmItems := []types.EvidenceItem{
		{Subject: "Looper", Object: "alpha", AnchorKind: types.AnchorDefinition, Source: "impl_alpha.go", LineStart: 14},
	}
	out := renderTypedRelationAppendix(hints, llmItems)
	if strings.Contains(out, "alpha") {
		t.Errorf("alpha was already in LLM evidence; should be deduped out of typed appendix; got %q", out)
	}
	if !strings.Contains(out, "beta") {
		t.Errorf("beta NOT in LLM evidence; should appear in typed appendix; got %q", out)
	}
}

func TestRenderTypedRelationAppendix_AllDedupedReturnsEmpty(t *testing.T) {
	hints := []types.TypedRelationHint{{
		Relation:   "implements",
		SourceName: "Looper",
		Members: []types.TypedRelationMember{
			{Name: "alpha", File: "impl_alpha.go", Line: 14},
		},
	}}
	llmItems := []types.EvidenceItem{
		{Subject: "Looper", Object: "alpha", AnchorKind: types.AnchorDefinition},
	}
	if out := renderTypedRelationAppendix(hints, llmItems); out != "" {
		t.Errorf("expected empty appendix when LLM covers everything; got %q", out)
	}
}

func TestRenderTypedRelationAppendix_NoHintsReturnsEmpty(t *testing.T) {
	if out := renderTypedRelationAppendix(nil, nil); out != "" {
		t.Errorf("expected empty appendix when no hints; got %q", out)
	}
}

// TestFormatEvidenceItems_TypedHintsAppendedAfterLLMRows verifies the
// unified rendering: LLM rows first, typed_graph rows after, in one
// section, with dedup applied.
func TestFormatEvidenceItems_TypedHintsAppendedAfterLLMRows(t *testing.T) {
	llmItems := []types.EvidenceItem{
		{
			Subject:    "Looper",
			Object:     "alpha",
			AnchorKind: types.AnchorDefinition,
			Source:     "impl_alpha.go",
			LineStart:  14,
			Producer:   "llm.emit_evidence",
			Kind:       types.EvidenceMechanism,
			Scope:      types.ScopeLine,
			Confidence: 0.9,
		},
	}
	hints := []types.TypedRelationHint{{
		Relation:   "implements",
		SourceName: "Looper",
		SourceKind: "interface",
		Members: []types.TypedRelationMember{
			{Name: "alpha", File: "impl_alpha.go", Line: 14, Kind: "struct"}, // dup
			{Name: "beta", File: "impl_beta.go", Line: 22, Kind: "struct"},
			{Name: "gamma", File: "impl_gamma.go", Line: 31, Kind: "struct"},
		},
	}}
	out := formatEvidenceItemsWithOptions(llmItems, 18, evidenceRenderOptions{
		TypedRelationHints: hints,
	})
	// alpha appears once via LLM row; the typed appendix should NOT
	// emit a duplicate. Count rows tagged provenance=typed_graph that
	// mention alpha — must be zero.
	for _, ln := range strings.Split(out, "\n") {
		if strings.Contains(ln, "provenance=typed_graph") && strings.Contains(ln, " alpha ") {
			t.Errorf("alpha was already in LLM evidence; typed_graph row must dedup; got %q", ln)
		}
	}
	if !strings.Contains(out, "beta") {
		t.Errorf("expected beta to appear; got %q", out)
	}
	if !strings.Contains(out, "gamma") {
		t.Errorf("expected gamma to appear; got %q", out)
	}
	if !strings.Contains(out, "provenance=typed_graph") {
		t.Errorf("expected provenance=typed_graph tag on synthetic rows; got %q", out)
	}
	// Ordering: LLM row before typed rows
	llmIdx := strings.Index(out, "alpha")
	typedIdx := strings.Index(out, "beta")
	if llmIdx < 0 || typedIdx < 0 || llmIdx > typedIdx {
		t.Errorf("expected LLM row to precede typed rows; alpha at %d, beta at %d", llmIdx, typedIdx)
	}
}
