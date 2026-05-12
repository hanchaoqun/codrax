package types

import "testing"

// TestClaimForm_AllValuesValidExceptUnknown pins the IsValid contract.
// ClaimUnknown is intentionally NOT valid (it is the safe fallback);
// every other declared constant must report valid.
func TestClaimForm_AllValuesValidExceptUnknown(t *testing.T) {
	if ClaimUnknown.IsValid() {
		t.Errorf("ClaimUnknown must NOT report valid (it is the fallback sentinel)")
	}
	for _, c := range AllClaimForms() {
		if !c.IsValid() {
			t.Errorf("ClaimForm %q must report valid", c)
		}
	}
	// Drift guard: AllClaimForms must enumerate every non-Unknown
	// declared constant. Adding a new ClaimForm constant requires
	// updating the allClaimForms slice; this guard catches that
	// drift via length comparison against the known constant count
	// (9 as of 2026-05-02 Phase 0; bump this assertion if you add
	// a new ClaimForm).
	if len(AllClaimForms()) != 9 {
		t.Errorf("AllClaimForms length %d != 9 — update allClaimForms slice when adding a new ClaimForm constant", len(AllClaimForms()))
	}
}

func TestClaimForm_UsesNonSymbolLabelSurface(t *testing.T) {
	for _, c := range []ClaimForm{ClaimImportEdge, ClaimPrecedenceRole, ClaimExternalObservation} {
		if !c.UsesNonSymbolLabelSurface() {
			t.Fatalf("%s should use typed display labels instead of declaration-symbol labels", c)
		}
	}
	for _, c := range []ClaimForm{ClaimDefinitionFact, ClaimCallEdge, ClaimGuardCondition, ClaimAssignmentFact, ClaimReturnFact, ClaimAbsenceFact} {
		if c.UsesNonSymbolLabelSurface() {
			t.Fatalf("%s should keep symbol/prose-specific validation", c)
		}
	}
}

func TestClaimForm_CitationRoleIdentityKindExhaustive(t *testing.T) {
	for _, c := range AllClaimForms() {
		switch c.CitationRoleIdentityKind() {
		case ClaimCitationRoleIdentityNone, ClaimCitationRoleDirectedEdge, ClaimCitationRoleDisplaySurface:
		default:
			t.Fatalf("claim form %q returned unknown citation role identity kind %q", c, c.CitationRoleIdentityKind())
		}
	}
	for _, c := range []ClaimForm{ClaimCallEdge, ClaimImportEdge, ClaimPrecedenceRole, ClaimExternalObservation} {
		if !c.SupportsCitationRoleAlignment() {
			t.Fatalf("claim form %q should support typed citation-role alignment", c)
		}
	}
}

func TestEvidenceClaimRoleMentionedBySurface_DirectedEdge(t *testing.T) {
	ev := EvidenceItem{
		AnchorKind:      AnchorCall,
		Subject:         "ParseOutput",
		Object:          "buildAnalysisIR",
		GroundingStatus: GroundingGrounded,
	}
	if !EvidenceClaimRoleMentionedBySurface(ev, []ClaimForm{ClaimCallEdge}, "`ParseOutput` -> `buildAnalysisIR`") {
		t.Fatalf("directed edge endpoints should be recognized as typed role surface")
	}
	if EvidenceClaimRoleMentionedBySurface(ev, []ClaimForm{ClaimCallEdge}, "`ParseOutputX` -> `buildAnalysisIR`") {
		t.Fatalf("endpoint matching must use code-token boundaries")
	}
}

func TestEvidenceClaimRoleMentionedBySurface_DisplaySurface(t *testing.T) {
	ev := EvidenceItem{
		Origin:          ClaimOriginLog,
		Subject:         "java.io.IOException",
		SurfaceTerms:    []string{"Connection refused"},
		GroundingStatus: GroundingGrounded,
	}
	if !EvidenceClaimRoleMentionedBySurface(ev, []ClaimForm{ClaimExternalObservation}, "Caused by java.io.IOException: Connection refused") {
		t.Fatalf("display surface terms from typed evidence should be recognized")
	}
}

func TestSameEvidenceClaimRole_RejectsDefinitionForCallEdge(t *testing.T) {
	call := EvidenceItem{
		ID:         "call",
		AnchorKind: AnchorCall,
		Subject:    "ParseOutput",
		Object:     "buildAnalysisIR",
	}
	def := EvidenceItem{
		AnchorKind:   AnchorDefinition,
		Subject:      "buildAnalysisIR",
		AnchorSymbol: "buildAnalysisIR",
	}
	if SameEvidenceClaimRole(def, call) {
		t.Fatalf("definition evidence must not satisfy a call-edge role")
	}
}

func TestClaimForm_LabelSurfaceKindAllRegisteredFormsClassified(t *testing.T) {
	for _, c := range AllClaimForms() {
		kind := c.LabelSurfaceKind()
		if !kind.IsValid() {
			t.Fatalf("ClaimForm %s must declare a valid label surface kind; got %q", c, kind)
		}
	}
	if kind := ClaimUnknown.LabelSurfaceKind(); kind != ClaimLabelSurfaceUnknown {
		t.Fatalf("ClaimUnknown label surface kind = %q, want unknown", kind)
	}
}

// TestClaimFormOf_OriginLogPerfDominates pins priority rule 1:
// Origin=Log or Origin=Perf returns ClaimExternalObservation
// regardless of AnchorKind / Scope / DiagramRole.
func TestClaimFormOf_OriginLogPerfDominates(t *testing.T) {
	cases := []struct {
		name string
		in   EvidenceItem
	}{
		{
			name: "log origin + call anchor → external_observation (call ignored)",
			in: EvidenceItem{
				Origin:     ClaimOriginLog,
				AnchorKind: AnchorCall,
				Scope:      ScopeLine,
			},
		},
		{
			name: "perf origin + condition anchor → external_observation",
			in: EvidenceItem{
				Origin:     ClaimOriginPerf,
				AnchorKind: AnchorCondition,
				Scope:      ScopeLine,
			},
		},
		{
			name: "log origin + diagram role config → external_observation",
			in: EvidenceItem{
				Origin:      ClaimOriginLog,
				DiagramRole: EvidenceDiagramRoleConfig,
				Scope:       ScopeLine,
			},
		},
		{
			name: "log origin + negative scope → external_observation (log dominates)",
			in: EvidenceItem{
				Origin: ClaimOriginLog,
				Scope:  ScopeNegative,
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ClaimFormOf(c.in)
			if got != ClaimExternalObservation {
				t.Errorf("got %q, want ClaimExternalObservation", got)
			}
		})
	}
}

// TestClaimFormOf_NegativeScopeBeforeAnchorKind pins priority rule 2:
// Scope=Negative returns ClaimAbsenceFact regardless of AnchorKind
// or DiagramRole, but ONLY when Origin is not log/perf (rule 1
// dominates).
func TestClaimFormOf_NegativeScopeBeforeAnchorKind(t *testing.T) {
	cases := []struct {
		name string
		in   EvidenceItem
		want ClaimForm
	}{
		{
			name: "negative scope + definition anchor (no origin) → absence",
			in: EvidenceItem{
				Scope:      ScopeNegative,
				AnchorKind: AnchorDefinition,
			},
			want: ClaimAbsenceFact,
		},
		{
			name: "negative scope + current_repo origin → absence",
			in: EvidenceItem{
				Origin:     ClaimOriginCurrentRepo,
				Scope:      ScopeNegative,
				AnchorKind: AnchorAssignment,
			},
			want: ClaimAbsenceFact,
		},
		{
			name: "negative scope + diagram role → absence (negative dominates non-default)",
			in: EvidenceItem{
				Scope:       ScopeNegative,
				DiagramRole: EvidenceDiagramRoleConfig,
			},
			want: ClaimAbsenceFact,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClaimFormOf(c.in); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestClaimFormOf_DiagramRoleNonDefaultProjects pins priority rule 3:
// when DiagramRole is non-default and non-unknown, ClaimPrecedenceRole
// wins over the AnchorKind branch.
func TestClaimFormOf_DiagramRoleNonDefaultProjects(t *testing.T) {
	cases := []struct {
		name string
		in   EvidenceItem
		want ClaimForm
	}{
		{
			name: "diagram role config + assignment anchor → precedence",
			in: EvidenceItem{
				DiagramRole: EvidenceDiagramRoleConfig,
				AnchorKind:  AnchorAssignment,
				Scope:       ScopeLine,
			},
			want: ClaimPrecedenceRole,
		},
		{
			name: "diagram role default + call anchor → call_edge (default doesn't trigger precedence)",
			in: EvidenceItem{
				DiagramRole: EvidenceDiagramRoleDefault,
				AnchorKind:  AnchorCall,
				Scope:       ScopeLine,
			},
			want: ClaimCallEdge,
		},
		{
			name: "diagram role unknown + return anchor → return_fact",
			in: EvidenceItem{
				DiagramRole: EvidenceDiagramRoleUnknown,
				AnchorKind:  AnchorReturn,
				Scope:       ScopeLine,
			},
			want: ClaimReturnFact,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClaimFormOf(c.in); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestClaimFormOf_AnchorKindDispatch pins priority rule 4: each
// AnchorKind value projects to its corresponding ClaimForm.
func TestClaimFormOf_AnchorKindDispatch(t *testing.T) {
	cases := []struct {
		anchor AnchorKind
		want   ClaimForm
	}{
		{AnchorCall, ClaimCallEdge},
		{AnchorCondition, ClaimGuardCondition},
		{AnchorReturn, ClaimReturnFact},
		{AnchorAssignment, ClaimAssignmentFact},
		{AnchorInitializer, ClaimAssignmentFact},
		{AnchorImport, ClaimImportEdge},
		{AnchorDefinition, ClaimDefinitionFact},
	}
	for _, c := range cases {
		t.Run(string(c.anchor), func(t *testing.T) {
			got := ClaimFormOf(EvidenceItem{
				AnchorKind: c.anchor,
				Scope:      ScopeLine,
				Origin:     ClaimOriginCurrentRepo,
			})
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestClaimFormOf_FallthroughToUnknown pins priority rule 5: when
// no typed signal exists (no origin / no negative / no diagram role
// / no anchor kind), ClaimUnknown is the safe fallback.
func TestClaimFormOf_FallthroughToUnknown(t *testing.T) {
	cases := []struct {
		name string
		in   EvidenceItem
	}{
		{"empty item", EvidenceItem{}},
		{"current repo origin alone (no anchor kind, no negative)", EvidenceItem{Origin: ClaimOriginCurrentRepo}},
		{"line scope alone (no anchor kind)", EvidenceItem{Scope: ScopeLine}},
		{"line range scope alone", EvidenceItem{Scope: ScopeLineRange}},
		{"file scope (no anchor kind, not negative)", EvidenceItem{Scope: ScopeFile}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ClaimFormOf(c.in)
			if got != ClaimUnknown {
				t.Errorf("got %q, want ClaimUnknown", got)
			}
		})
	}
}

// TestClaimFormOf_CrossSourceFallsThroughToAnchorKind pins the
// CrossSource origin semantic: it does NOT trigger external_observation
// (rule 1 only fires on Log or Perf), so the AnchorKind branch
// runs as if the evidence were repo-side. This reflects that
// cross-source confirmation promotes the evidence to a current-
// mechanism class.
func TestClaimFormOf_CrossSourceFallsThroughToAnchorKind(t *testing.T) {
	got := ClaimFormOf(EvidenceItem{
		Origin:     ClaimOriginCrossSource,
		AnchorKind: AnchorCall,
		Scope:      ScopeLine,
	})
	if got != ClaimCallEdge {
		t.Errorf("cross-source origin must fall through to AnchorKind dispatch; got %q want ClaimCallEdge", got)
	}
}

// TestAllClaimForms_DefensiveCopy pins that AllClaimForms returns a
// fresh slice — mutating the result must not corrupt the package
// state.
func TestAllClaimForms_DefensiveCopy(t *testing.T) {
	a := AllClaimForms()
	original := a[0]
	a[0] = ClaimUnknown
	b := AllClaimForms()
	if b[0] != original {
		t.Errorf("AllClaimForms must return a defensive copy; mutation leaked into package state")
	}
}
