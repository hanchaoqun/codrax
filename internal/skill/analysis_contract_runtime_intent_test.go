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
