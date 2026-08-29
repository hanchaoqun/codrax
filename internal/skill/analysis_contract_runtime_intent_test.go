package skill

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeIntentTeachingDoesNotTurnAttachmentPresenceIntoRootCause(t *testing.T) {
	var rootCauseDesc string
	for _, choice := range AnalysisIntentChoices() {
		if choice.Value == string(types.IntentRootCause) {
			rootCauseDesc = choice.Desc
			break
		}
	}
	if rootCauseDesc == "" {
		t.Fatal("root-cause intent description is missing")
	}
	for _, want := range []string{
		"attached runtime artifact is evidence context, not intent authority",
		"finite request for observed frame locations",
		"non-root-cause intent",
	} {
		if !strings.Contains(rootCauseDesc, want) {
			t.Fatalf("root-cause intent description missing consistency boundary %q: %s", want, rootCauseDesc)
		}
	}
	for _, forbidden := range []string{
		"has attached a runtime log",
		"wants the code location responsible",
	} {
		if strings.Contains(rootCauseDesc, forbidden) {
			t.Fatalf("attachment presence still mints root-cause intent via %q: %s", forbidden, rootCauseDesc)
		}
	}
	if !strings.Contains(AnalysisRuntimeScopeFromDimensionTeaching, "locating each observed crash frame") {
		t.Fatal("runtime scope teaching lost its finite crash-frame lookup example")
	}
}

func TestRuntimeScenarioTeachingKeepsFiniteObservationsOutOfRootCause(t *testing.T) {
	var rootCauseDesc, genericDesc string
	for _, choice := range analysisScenarios {
		switch choice.Value {
		case string(types.ScenarioRootCause):
			rootCauseDesc = choice.Desc
		case string(types.ScenarioGeneric):
			genericDesc = choice.Desc
		}
	}
	if rootCauseDesc == "" || genericDesc == "" {
		t.Fatalf("runtime scenario descriptions missing: root_cause=%q generic=%q", rootCauseDesc, genericDesc)
	}
	for _, want := range []string{"open-ended diagnosis", "not a finite lookup", "runtime frames"} {
		if !strings.Contains(rootCauseDesc, want) {
			t.Fatalf("root-cause scenario description missing finite-observation boundary %q: %s", want, rootCauseDesc)
		}
	}
	for _, want := range []string{"finite runtime observation lookup", "no causal relation"} {
		if !strings.Contains(genericDesc, want) {
			t.Fatalf("generic scenario description missing finite-observation lane %q: %s", want, genericDesc)
		}
	}
	if strings.TrimSpace(rootCauseDesc) == "debug a failure" {
		t.Fatal("broad failure label still turns every finite runtime observation into a root-cause scenario")
	}
}

func TestRuntimeCallChainTeachingRequiresRequestedRelation(t *testing.T) {
	desc := questionKindDescriptions[types.ReqCallChain]
	for _, want := range []string{
		"CURRENT request explicitly asks",
		"ordered caller/source → callee/sink",
		"independent observed stack-frame locations",
		"not a call chain",
	} {
		if !strings.Contains(desc, want) {
			t.Fatalf("call-chain question-kind description missing relation boundary %q: %s", want, desc)
		}
	}
	cfg := BuildAnalysisSkill()
	if cfg == nil {
		t.Fatal("BuildAnalysisSkill returned nil")
	}
	for _, want := range []string{
		"which first frame, source location, type, value, state, count, time, or message",
		"bounded fact set or finite enumeration",
		"not a call chain merely because the structured artifact also carries caller frames",
	} {
		if !strings.Contains(cfg.OutputFormat, want) {
			t.Fatalf("runtime call-chain guard missing finite-observation boundary %q", want)
		}
	}
}
