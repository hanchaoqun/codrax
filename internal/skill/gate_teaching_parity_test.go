package skill

import (
	"strings"
	"testing"
)

// gate_teaching_parity_test.go — EVALFIX-2A Tripwire A. Every
// GateTeaching's Text must appear verbatim in the rendered corpus of
// the skill it names (Goal + Workflow + OutputFormat + Prohibitions,
// both tiers). Verbatim containment is a precise signal, so the check
// is a hard gate; the typed escape is gateTeachingPromptExemptions.
// The reverse direction keeps exemptions from going stale.

// gateTeachingPromptExemptions — teachings deliberately carried ONLY
// on the repair surface (e.g. a disambiguation that would prime the
// wrong token if pre-taught, or a prompt-budget ruling). Key is
// GateTeaching.Key; the rationale must name the ruling. An exempted
// teaching that shows up in the prompt corpus anyway is a red test
// (stale exemption).
var gateTeachingPromptExemptions = map[string]string{}

func TestEveryGateTeachingIsCarriedByItsSkillPromptOrExempted(t *testing.T) {
	teachings := AllGateTeachings()
	if len(teachings) == 0 {
		t.Fatal("AllGateTeachings returned an empty universe — the tripwire would be vacuous")
	}
	r := NewRegistry()
	RegisterDefaults(r)
	for _, teaching := range teachings {
		if strings.TrimSpace(teaching.Key) == "" || strings.TrimSpace(teaching.SkillName) == "" || strings.TrimSpace(teaching.Text) == "" {
			t.Errorf("GateTeaching %+v has an empty Key, SkillName, or Text — every field is load-bearing", teaching)
			continue
		}
		sk, err := r.Get(teaching.SkillName)
		if err != nil {
			t.Errorf("teaching %q names skill %q which is not in the default registry: %v", teaching.Key, teaching.SkillName, err)
			continue
		}
		corpus := sk.Goal + "\n" + allWorkflowBodies(sk) + "\n" + sk.OutputFormat + "\n" + allProhibitionBodies(sk)
		carried := strings.Contains(corpus, teaching.Text)
		rationale, exempted := gateTeachingPromptExemptions[teaching.Key]
		switch {
		case exempted && strings.TrimSpace(rationale) == "":
			t.Errorf("teaching %q exemption has no rationale", teaching.Key)
		case exempted && carried:
			t.Errorf("teaching %q is exempted AND carried by skill %q — remove the stale exemption", teaching.Key, teaching.SkillName)
		case !exempted && !carried:
			t.Errorf("teaching %q not carried by skill %q — splice the GateTeaching Text constant into the skill's prompt surface (do not hand-copy the sentence) or declare an exemption with a rationale", teaching.Key, teaching.SkillName)
		}
	}
	// Exemptions must reference teachings that exist; a dangling key is
	// a stale entry.
	byKey := map[string]bool{}
	for _, teaching := range teachings {
		byKey[teaching.Key] = true
	}
	for key := range gateTeachingPromptExemptions {
		if !byKey[key] {
			t.Errorf("exemption %q names no registered GateTeaching — remove the stale entry", key)
		}
	}
}
