package types

import "testing"

func TestNormalizeWriteBehaviorContracts_PreservesOrderedTransition(t *testing.T) {
	got := NormalizeWriteBehaviorContracts([]WriteBehaviorContract{{
		ID:       "shared-cursor-exhaustion",
		Kind:     WriteBehaviorInvariant,
		Polarity: WriteBehaviorPolarityExpected,
		Operator: WriteBehaviorOpSatisfies,
		Subject:  "bidirectional cursor",
		Expected: "past-end transition exhausts the shared cursor",
		Transition: &WriteBehaviorTransition{Steps: []WriteBehaviorTransitionStep{
			{Phase: WriteBehaviorTransitionPhase(" SETUP "), Operation: " consume from front ", EvidenceRef: " test:front "},
			{Phase: WriteBehaviorTransitionPhase(" ACTION "), Operation: " skip from back ", Expected: " None "},
			{Phase: WriteBehaviorTransitionPhase(" OBSERVATION "), Expected: " no consumed member is returned "},
			{Phase: WriteBehaviorTransitionPhase(" POSTCONDITION "), Operation: " read both directions ", Expected: " both return None "},
		}},
		Required: true,
		Source:   "write_analyzer",
	}}, nil)
	if len(got) != 1 || got[0].Transition == nil {
		t.Fatalf("ordered transition missing: %+v", got)
	}
	steps := got[0].Transition.Steps
	if len(steps) != 4 {
		t.Fatalf("transition steps len = %d, want 4: %+v", len(steps), steps)
	}
	if steps[0].Phase != WriteBehaviorTransitionSetup || steps[0].Operation != "consume from front" || steps[0].EvidenceRef != "test:front" {
		t.Fatalf("setup step normalized incorrectly: %+v", steps[0])
	}
	if steps[3].Phase != WriteBehaviorTransitionPostcondition || steps[3].Expected != "both return None" {
		t.Fatalf("postcondition step normalized incorrectly: %+v", steps[3])
	}
	if IsHardRequiredWriteBehaviorContract(got[0]) {
		t.Fatalf("operator=satisfies transition must remain soft guidance: %+v", got[0])
	}
}

func TestNormalizeWriteBehaviorContracts_PreservesNotRaisesAndPolarity(t *testing.T) {
	got := NormalizeWriteBehaviorContracts([]WriteBehaviorContract{{
		ID:       "no-crash",
		Kind:     WriteBehaviorException,
		Polarity: WriteBehaviorPolarityExpected,
		Operator: WriteBehaviorOpNotRaises,
		Expected: "ValueError",
		Required: true,
		Source:   "write_analyzer",
	}}, nil)
	if len(got) != 1 {
		t.Fatalf("contracts len = %d, want 1: %+v", len(got), got)
	}
	if got[0].Operator != WriteBehaviorOpNotRaises {
		t.Fatalf("Operator = %q, want not_raises", got[0].Operator)
	}
	if got[0].Polarity != WriteBehaviorPolarityExpected {
		t.Fatalf("Polarity = %q, want expected", got[0].Polarity)
	}
	if !got[0].Required {
		t.Fatal("expected target contract to remain required")
	}
}

func TestNormalizeWriteBehaviorContracts_PreservesComparatorBaseline(t *testing.T) {
	got := NormalizeWriteBehaviorContracts([]WriteBehaviorContract{{
		ID:       "array-empty-shape",
		Kind:     WriteBehaviorInvariant,
		Polarity: WriteBehaviorPolarityExpected,
		Subject:  "Array([]).shape",
		Operator: WriteBehaviorOpEquals,
		Expected: "(0,)",
		Comparator: &WriteBehaviorComparator{
			Subject:     "Matrix([]).shape",
			Operator:    WriteBehaviorOperator(" EQUALS "),
			Expected:    "(0,)",
			Relation:    WriteBehaviorComparatorRelation(" REGRESSION_BASELINE "),
			EvidenceRef: "issue:1",
		},
		Required: true,
		Source:   "write_analyzer",
	}}, nil)
	if len(got) != 1 {
		t.Fatalf("contracts len = %d, want 1: %+v", len(got), got)
	}
	if got[0].Comparator == nil {
		t.Fatalf("comparator baseline missing: %+v", got[0])
	}
	if got[0].Comparator.Subject != "Matrix([]).shape" ||
		got[0].Comparator.Operator != WriteBehaviorOpEquals ||
		got[0].Comparator.Expected != "(0,)" ||
		got[0].Comparator.Relation != WriteBehaviorComparatorRegressionBaseline ||
		got[0].Comparator.EvidenceRef != "issue:1" {
		t.Fatalf("comparator baseline drifted: %+v", got[0].Comparator)
	}
	if !got[0].Required {
		t.Fatalf("expected comparator contract should remain required: %+v", got[0])
	}
}

func TestNormalizeWriteBehaviorContracts_PreservesRenderedTextPlacement(t *testing.T) {
	got := NormalizeWriteBehaviorContracts([]WriteBehaviorContract{{
		ID:       "units-placement",
		Kind:     WriteBehaviorStdout,
		Polarity: WriteBehaviorPolarityExpected,
		Subject:  "DataArray repr line",
		Operator: WriteBehaviorOpContains,
		Expected: ", in mm",
		Placement: &WriteRenderedTextPlacement{
			Surface:     WriteRenderedTextSurface(" REPR "),
			Anchor:      "rainfall",
			Relation:    WriteRenderedTextRelation(" BETWEEN_ANCHOR_AND_DELIMITER "),
			Delimiter:   "float32",
			EvidenceRef: "issue:repr-example",
		},
		Required: true,
		Source:   "write_analyzer",
	}}, nil)
	if len(got) != 1 {
		t.Fatalf("contracts len = %d, want 1: %+v", len(got), got)
	}
	p := got[0].Placement
	if p == nil {
		t.Fatalf("placement missing: %+v", got[0])
	}
	if p.Surface != WriteRenderedTextSurfaceRepr ||
		p.Anchor != "rainfall" ||
		p.Expected != ", in mm" ||
		p.Relation != WriteRenderedTextBetweenAnchorAndDelimiter ||
		p.Delimiter != "float32" ||
		p.EvidenceRef != "issue:repr-example" {
		t.Fatalf("placement normalized incorrectly: %+v", p)
	}
	if !IsHardRequiredWriteBehaviorContract(got[0]) || !IsPlacementRequiredWriteBehaviorContract(got[0]) {
		t.Fatalf("placement contract should be hard placement-required: %+v", got[0])
	}
	ids := PlacementRequiredWriteBehaviorContractIDs(got)
	if _, ok := ids["units-placement"]; !ok {
		t.Fatalf("placement ids missing contract: %+v", ids)
	}
}

func TestNormalizeWriteBehaviorContracts_DefaultsExpectedAndForbiddenToRequired(t *testing.T) {
	got := NormalizeWriteBehaviorContracts([]WriteBehaviorContract{
		{
			ID:       "no-crash",
			Kind:     WriteBehaviorException,
			Polarity: WriteBehaviorPolarityExpected,
			Operator: WriteBehaviorOpNotRaises,
			Expected: "request no longer crashes",
			Source:   "write_analyzer",
		},
		{
			ID:       "no-regression",
			Kind:     WriteBehaviorInvariant,
			Polarity: WriteBehaviorPolarityForbidden,
			Operator: WriteBehaviorOpRaises,
			Expected: "old regression does not return",
			Source:   "write_analyzer",
		},
	}, nil)
	if len(got) != 2 {
		t.Fatalf("contracts len = %d, want 2: %+v", len(got), got)
	}
	ids := RequiredWriteBehaviorContractIDs(got, false)
	for _, want := range []string{"no-crash", "no-regression"} {
		if _, ok := ids[want]; !ok {
			t.Fatalf("expected %q to be required after normalization: contracts=%+v ids=%+v", want, got, ids)
		}
	}
}

func TestNormalizeWriteBehaviorContracts_ObservedIsNotRequiredTarget(t *testing.T) {
	got := NormalizeWriteBehaviorContracts([]WriteBehaviorContract{{
		ID:       "current-crash",
		Kind:     WriteBehaviorException,
		Polarity: WriteBehaviorPolarityObserved,
		Operator: WriteBehaviorOpRaises,
		Expected: "ZeroDivisionError",
		Required: true,
		Source:   "write_analyzer",
	}}, nil)
	if len(got) != 1 {
		t.Fatalf("contracts len = %d, want 1: %+v", len(got), got)
	}
	if got[0].Polarity != WriteBehaviorPolarityObserved {
		t.Fatalf("Polarity = %q, want observed", got[0].Polarity)
	}
	if got[0].Required {
		t.Fatalf("observed pre-fix facts must not remain required completion targets: %+v", got[0])
	}
	if ids := RequiredWriteBehaviorContractIDs(got, false); len(ids) != 0 {
		t.Fatalf("observed contract should not be required coverage: %+v", ids)
	}
}

func TestNormalizeWriteBehaviorContracts_FallbacksAreExpectedTargets(t *testing.T) {
	got := NormalizeWriteBehaviorContracts(nil, []string{"bug no longer reproduces"})
	if len(got) != 1 {
		t.Fatalf("contracts len = %d, want 1: %+v", len(got), got)
	}
	if got[0].Polarity != WriteBehaviorPolarityExpected || !got[0].Required {
		t.Fatalf("fallback expected outcome should be required expected target: %+v", got[0])
	}
	if ids := RequiredWriteBehaviorContractIDs(got, false); len(ids) != 0 {
		t.Fatalf("explicit-required view should exclude fallback ids: %+v", ids)
	}
	if ids := RequiredWriteBehaviorContractIDs(got, true); len(ids) != 1 {
		t.Fatalf("fallback-inclusive required ids missing: %+v", ids)
	}
}

func TestNormalizeWriteBehaviorContracts_AppendsDistinctExpectedOutcomes(t *testing.T) {
	got := NormalizeWriteBehaviorContracts([]WriteBehaviorContract{{
		ID:       "nan-height",
		Kind:     WriteBehaviorException,
		Polarity: WriteBehaviorPolarityExpected,
		Operator: WriteBehaviorOpNotRaises,
		Expected: "ax.bar([np.nan], [0]) does not raise",
		Source:   "write_analyzer",
	}}, []string{
		"ax.bar([np.nan], [0]) does not raise",
		"ax.bar([np.nan], [np.nan]) does not raise",
	})
	if len(got) != 2 {
		t.Fatalf("contracts len = %d, want explicit plus one distinct fallback: %+v", len(got), got)
	}
	if got[0].ID != "nan-height" || !got[0].Required || got[0].Source != "write_analyzer" {
		t.Fatalf("explicit required contract drifted: %+v", got[0])
	}
	if got[1].Source != "expected_outcome_fallback" || got[1].ID != "outcome-2" || !got[1].Required {
		t.Fatalf("distinct expected outcome should append as required fallback: %+v", got[1])
	}
	if ids := RequiredWriteBehaviorContractIDs(got, false); len(ids) != 1 {
		t.Fatalf("explicit-required view should exclude fallback ids: %+v", ids)
	}
	if ids := RequiredWriteBehaviorContractIDs(got, true); len(ids) != 2 {
		t.Fatalf("fallback-inclusive required ids missing: %+v", ids)
	}
}

func TestRebaseExpectedOutcomeFallbackWriteBehaviorContractsPreservesExplicitAndReplacesFallbacks(t *testing.T) {
	got := RebaseExpectedOutcomeFallbackWriteBehaviorContracts([]WriteBehaviorContract{
		{
			ID:       "explicit-api",
			Kind:     WriteBehaviorInvariant,
			Polarity: WriteBehaviorPolarityExpected,
			Operator: WriteBehaviorOpEquals,
			Subject:  "public API",
			Expected: "remains compatible",
			Required: true,
			Source:   "write_analyzer",
		},
		{
			ID:       "outcome-1",
			Kind:     WriteBehaviorObservable,
			Polarity: WriteBehaviorPolarityExpected,
			Operator: WriteBehaviorOpSatisfies,
			Expected: "line 16 remains unchanged",
			Required: true,
			Source:   WriteBehaviorContractSourceExpectedOutcomeFallback,
		},
	}, []string{
		"line 12 returns the callback error",
		"line 16 returns the negative lookup error",
	})

	if len(got) != 3 {
		t.Fatalf("contracts len = %d, want explicit plus two current fallbacks: %+v", len(got), got)
	}
	if got[0].ID != "explicit-api" || got[0].Expected != "remains compatible" || got[0].Source != "write_analyzer" {
		t.Fatalf("explicit analyzer contract drifted during fallback rebase: %+v", got[0])
	}
	for _, contract := range got {
		if contract.Expected == "line 16 remains unchanged" {
			t.Fatalf("stale fallback survived current-generation rebase: %+v", got)
		}
	}
	if got[1].Expected != "line 12 returns the callback error" || got[2].Expected != "line 16 returns the negative lookup error" {
		t.Fatalf("current acceptance tests were not preserved in order: %+v", got)
	}
	if !IsExpectedOutcomeFallbackWriteBehaviorContract(got[1]) || !IsExpectedOutcomeFallbackWriteBehaviorContract(got[2]) {
		t.Fatalf("rebased acceptance tests must remain soft fallback contracts: %+v", got)
	}
	if got[1].Source != WriteBehaviorContractSourcePlanAcceptanceFallback || got[2].Source != WriteBehaviorContractSourcePlanAcceptanceFallback {
		t.Fatalf("rebased fallbacks must expose their newer typed generation: %+v", got)
	}
}

func TestRebaseVerifyFailureWriteBehaviorContractsRetiresUngroundedSoftGeneration(t *testing.T) {
	got, retired := RebaseVerifyFailureWriteBehaviorContracts([]WriteBehaviorContract{
		{
			ID:       "hard-user-contract",
			Kind:     WriteBehaviorInvariant,
			Polarity: WriteBehaviorPolarityExpected,
			Operator: WriteBehaviorOpEquals,
			Expected: "public API remains compatible",
			Required: true,
			Source:   "write_analyzer",
		},
		{
			ID:       "stale-soft-shape",
			Kind:     WriteBehaviorInvariant,
			Polarity: WriteBehaviorPolarityExpected,
			Operator: WriteBehaviorOpSatisfies,
			Expected: "MIN calls the constructor that reads MIN",
			Required: true,
			Source:   "write_analyzer",
		},
		{
			ID:          "grounded-soft-invariant",
			Kind:        WriteBehaviorInvariant,
			Polarity:    WriteBehaviorPolarityExpected,
			Operator:    WriteBehaviorOpSatisfies,
			Expected:    "wire layout remains compatible",
			EvidenceRef: "file:protocol.rs:42",
			Required:    true,
			Source:      "write_analyzer",
		},
		{
			ID:       "observed-failure",
			Kind:     WriteBehaviorObservable,
			Polarity: WriteBehaviorPolarityObserved,
			Operator: WriteBehaviorOpSatisfies,
			Expected: "the original check failed",
			Source:   "write_analyzer",
		},
		{
			ID:       "outcome-1",
			Kind:     WriteBehaviorObservable,
			Polarity: WriteBehaviorPolarityExpected,
			Operator: WriteBehaviorOpSatisfies,
			Expected: "the stale plan shape passes",
			Required: true,
			Source:   WriteBehaviorContractSourceExpectedOutcomeFallback,
		},
	}, []string{"the repaired direct constant passes"})

	if len(got) != 4 {
		t.Fatalf("contracts = %+v, want hard + grounded + observed + current fallback", got)
	}
	if got[0].ID != "hard-user-contract" || got[1].ID != "grounded-soft-invariant" || got[2].ID != "observed-failure" {
		t.Fatalf("stable contracts were not retained in order: %+v", got)
	}
	if got[3].Expected != "the repaired direct constant passes" || got[3].Source != WriteBehaviorContractSourcePlanAcceptanceFallback {
		t.Fatalf("current acceptance generation missing: %+v", got)
	}
	wantRetired := []string{"outcome-1", "stale-soft-shape"}
	if len(retired) != len(wantRetired) || retired[0] != wantRetired[0] || retired[1] != wantRetired[1] {
		t.Fatalf("retired ids = %+v, want %+v", retired, wantRetired)
	}
}

func TestHardRequiredWriteBehaviorContractIDs_ExcludesSatisfiesAndFallback(t *testing.T) {
	got := NormalizeWriteBehaviorContracts([]WriteBehaviorContract{
		{
			ID:       "exact-shape",
			Kind:     WriteBehaviorInvariant,
			Polarity: WriteBehaviorPolarityExpected,
			Operator: WriteBehaviorOpEquals,
			Subject:  "Array([]).shape",
			Expected: "(0,)",
			Required: true,
			Source:   "write_analyzer",
		},
		{
			ID:       "soft-outcome",
			Kind:     WriteBehaviorObservable,
			Polarity: WriteBehaviorPolarityExpected,
			Operator: WriteBehaviorOpSatisfies,
			Subject:  "Array([])",
			Expected: "creates an empty array with the right shape",
			Required: true,
			Source:   "write_analyzer",
		},
	}, []string{"fallback outcome text"})

	required := RequiredWriteBehaviorContractIDs(got, true)
	if len(required) != 3 {
		t.Fatalf("wide required view should retain all completion targets, got %+v", required)
	}
	hard := HardRequiredWriteBehaviorContractIDs(got)
	if len(hard) != 1 {
		t.Fatalf("hard required view should contain only exact typed contract, got %+v", hard)
	}
	if _, ok := hard["exact-shape"]; !ok {
		t.Fatalf("hard required view missing exact contract: %+v", hard)
	}
}
