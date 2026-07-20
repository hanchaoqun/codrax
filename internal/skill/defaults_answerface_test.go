package skill

// defaults_answerface_test.go — ANSWERFACE-1 teaching pins (§29.140
// G4/G9/叙事消费, 2026-07-19): the three prompt-face additions stay present,
// trace-gated, and soft (prompt guidance only — no OnViolation hard lane for
// the census/word-face clauses; the caliber-separation second direction rides
// the existing FIN-BIND (g) clause).

import (
	"strings"
	"testing"
)

// TestAnswerfaceBlockedReasonCensusConsumption — 件1 (G4 教学半场): the
// census inventory is authoritative; impression-based drops are named as the
// failure shape.
func TestAnswerfaceBlockedReasonCensusConsumption(t *testing.T) {
	item := finBindTierBItem(t, "BLOCKED-REASON CENSUS CONSUMPTION")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("census-consumption rule must be trace-gated: %+v", item.AppliesTo)
	}
	if len(item.OnViolation) != 0 {
		t.Fatalf("census-consumption rule is soft guidance only — no violation lane: %+v", item.OnViolation)
	}
	for _, want := range []string{
		"`blocked_reason_census` note",
		"authoritative record inventory",
		"never drop a record because it no longer fits the narrative",
		"the census wins and the raw rows deserve a re-read",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("census-consumption rule missing %q:\n%s", want, item.Body)
		}
	}
}

// TestAnswerfaceSegmentVsWindowTotalDirection — 件5 (G9 教学): FIN-BIND (g)
// carries the second caliber direction — a segment value never impersonates
// the full-window per-state total; window totals come from the published
// target_window_states partition only.
func TestAnswerfaceSegmentVsWindowTotalDirection(t *testing.T) {
	item := finBindTierBItem(t, "STATE-DURATION CALIBER SEPARATION")
	for _, want := range []string{
		"never the thread's full-window total for that state",
		"target_window_states partition",
		"not from a segment row and not from your own reconstruction of segments",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("segment-vs-total direction missing %q:\n%s", want, item.Body)
		}
	}
}

// TestAnswerfaceTypedWordFaceConsumption — 件6 (叙事消费教学): the four
// word-face lanes (fix_direction / cross_direction_overlaps / member rosters
// / chain credentials) are taught with their LLM-visible note keys and the
// non-stacking disciplines.
func TestAnswerfaceTypedWordFaceConsumption(t *testing.T) {
	item := finBindTierBItem(t, "TYPED WORD-FACE CONSUMPTION")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("word-face rule must be trace-gated: %+v", item.AppliesTo)
	}
	if len(item.OnViolation) != 0 {
		t.Fatalf("word-face rule is soft guidance only — no violation lane: %+v", item.OnViolation)
	}
	for _, want := range []string{
		"`fix_direction`",
		"benefits across DIFFERENT directions never add up",
		"`cross_direction_overlaps`",
		"never present the two benefits as stackable",
		"`member_roster` / `member_wall_ms` / `member_count`",
		"`chain_credential_segments`",
		"`chain_credential_envelope_level`",
		"`chain_credential_lane_demoted`",
		"never a proven cause",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("word-face rule missing %q:\n%s", want, item.Body)
		}
	}
}

// TestAnswerfaceExploreCensusSentence — 件1 explore 半场: the investigation
// face carries the census inventory sentence too (the c2 drop happened during
// exploration, before drafting).
func TestAnswerfaceExploreCensusSentence(t *testing.T) {
	registry := NewRegistry()
	RegisterDefaults(registry)
	config, err := registry.Get("explore-skill")
	if err != nil {
		t.Fatalf("explore-skill must be registered: %v", err)
	}
	var body string
	for _, item := range config.WorkflowTierB {
		if strings.HasPrefix(item.Body, "TRACE QUERY:") {
			body = item.Body
		}
	}
	if body == "" {
		t.Fatalf("explore-skill TRACE QUERY guidance missing")
	}
	for _, want := range []string{
		"`blocked_reason_census` line",
		"authoritative blocked_reason inventory",
		"re-read the rows instead of silently dropping one",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("explore census sentence missing %q", want)
		}
	}
}

// TestAnswerfaceFixDirectionCompletenessDuty — FREQDIR-1 件3 (§29.149 修向③,
// 2026-07-19): the fix-direction clause carries the completeness duty (every
// published direction enters the enumeration) PAIRED with the independent-
// caliber participation rule (a 折算 seat enters WITH its caliber words and
// never joins a cross-direction sum), and never teaches self-crowning by a
// cross-caliber comparison. Both arms pinned so the pair can never travel
// apart (防铸混尺和); still soft guidance only (no violation lane).
func TestAnswerfaceFixDirectionCompletenessDuty(t *testing.T) {
	item := finBindTierBItem(t, "TYPED WORD-FACE CONSUMPTION")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("fix-direction duty must be trace-gated: %+v", item.AppliesTo)
	}
	if len(item.OnViolation) != 0 {
		t.Fatalf("fix-direction duty is soft guidance only — no violation lane: %+v", item.OnViolation)
	}
	// 完备句 (completeness arm). 返工 P2-1 (双复核): the duty is CHANNEL-
	// QUALIFIED to on-chain seats — the ◎ direction sections fold on-chain
	// members only, so an unqualified duty would teach a lane maximum the
	// rendered board never publishes.
	for _, want := range []string{
		"the direction enumeration MUST cover EVERY direction value published on ON-CHAIN seated causes",
		"adjacent-channel rows stay conditional upper bounds and never set a direction's maximum",
		"a direction is never omitted because its seat's value rides a different caliber",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("completeness arm missing %q:\n%s", want, item.Body)
		}
	}
	// 口径句 (independent-caliber participation arm — same sentence pair).
	for _, want := range []string{
		"discounted (折算) caliber enters the enumeration as its OWN direction entry stated together with its published caliber words",
		"its value never joins any wall-clock total",
		"never summed into them, never silently dropped",
		"never ranked by your own cross-caliber comparison",
		"let the published board order speak for which direction leads",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("caliber-participation arm missing %q:\n%s", want, item.Body)
		}
	}
}
