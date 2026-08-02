package tool

// answer_document_projection_rule3cr_test.go — RULE3-1 双复核修复轮 pins
// (adversarial F1-F6 + 冷读 P1/P2/P3 findings + §29.187 user rulings,
// 2026-07-21):
//
//	件1  EN snake_case 漏网 — the compound type word's EN base word rides
//	     runtimeTraceRootCauseTypeENLabel (件8 verdict table), never the raw
//	     wire token (the retired PTV6-C D2 discipline).
//	件2  渲染面 grep pin — EN tree fence + ◎ fence verdict-word lane carries
//	     ZERO snake_case tokens (whitelist: state-identity parentheticals
//	     s_sleep/d_sleep, wait-object identifiers, detail "- type:" rows).
//	     RED before 件1 (evidence log in scratchpad rule3cr/), green after.
//	§29.187① 凭证四字族 — ·唤醒锚定/·目标自身/·交集证明/·成员继承; every ⛓
//	     seat row wears exactly one; legend 强→弱 four-row table.
//	§29.187② 修向词 IO/内核/依赖 — pure rename, closed set unchanged.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/types"
)

// rule3crSnakeToken matches a snake_case wire-token shape: letter-led parts
// joined by underscores (thread ids like binder:14214_1 stay unmatched — the
// digit-led tail is not a wire token).
var rule3crSnakeToken = regexp.MustCompile(`\b[A-Za-z][A-Za-z0-9]*(?:_[A-Za-z][A-Za-z0-9]*)+\b`)

// rule3crSnakeWhitelisted reports whether a snake-shaped hit is one of the
// audit-fidelity lanes the ruling deliberately KEEPS raw (§29.182② "wire
// token 留证据引用键位"):
//   - state-identity parentheticals: the label（token） combined form for the
//     raw scheduler state (s_sleep / d_sleep / r_runnable families);
//   - kernel wait-call-site identifiers beside 内核调用点 / kernel wait
//     call-site words;
//   - the detail table's "- type:" rows (raw token column by design).
//
// context is the de-wrapped neighborhood around the hit (the PTV4 T3 width
// governor may break a physical line anywhere, so line-scoped checks would
// see split tokens; the joined neighborhood keeps the check local).
func rule3crSnakeWhitelisted(context, token string) bool {
	switch token {
	case "s_sleep", "d_sleep", "d_state":
		return true
	}
	for _, marker := range []string{"- type:", "- 类型:", "kernel wait call-site", "内核调用点"} {
		if strings.Contains(context, marker) {
			return true
		}
	}
	return false
}

// TestRule3crENRenderFaceZeroSnakeCase — 修复轮 件2 (对抗 F2 + 冷读 P1-1,
// 2026-07-21): the EN tree fence AND the EN ◎ overview fence speak reader
// verdict words end to end — zero snake_case wire tokens on the verdict-word
// lane. The fixture carries the supply-gap-dominant inversion form, the exact
// shape whose EN compound base word leaked the raw wire token
// (priority_inversion_candidate · supply-gap dominant) through the retired
// PTV6-C D2 arm while every sibling face already spoke §29.182② words.
func TestRule3crENRenderFaceZeroSnakeCase(t *testing.T) {
	projection := elimInvSupplyCompoundProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	tree := rspaFenceJoined(runtimeTraceProjTreeFence(model, false))
	overview := runtimeTraceProjElimOverviewFence(projection, model, false)
	for _, surface := range []struct{ name, text string }{
		{"tree fence", tree},
		{"elim overview fence", rspaFenceJoined(overview)},
	} {
		for _, hit := range rule3crSnakeToken.FindAllStringIndex(surface.text, -1) {
			token := surface.text[hit[0]:hit[1]]
			lo, hi := hit[0]-60, hit[1]+60
			if lo < 0 {
				lo = 0
			}
			if hi > len(surface.text) {
				hi = len(surface.text)
			}
			context := surface.text[lo:hi]
			if rule3crSnakeWhitelisted(context, token) {
				continue
			}
			t.Errorf("件2: EN %s carries snake_case verdict token %q near %q", surface.name, token, context)
		}
	}
	// The compound seat itself must still render (the pin is about WORDING,
	// not suppression): the EN compound word rides both faces.
	if !strings.Contains(overview, tracefence.SupplyGapDominantWordEN) {
		t.Fatalf("件2: the supply-gap-dominant compound seat must render on the ◎ face:\n%s", overview)
	}
}

// TestRule3crENCompoundWordSpeaksVerdictLabel — 修复轮 件1: the single
// composer's EN branch consumes the 件8 verdict table for the base word.
func TestRule3crENCompoundWordSpeaksVerdictLabel(t *testing.T) {
	projection := elimInvSupplyCompoundProjection()
	word, ok := runtimeTraceProjInversionSupplyGapCompoundWord(projection.OnChainCauses[0], false)
	if !ok {
		t.Fatalf("the dominant fixture seat must mint the compound word")
	}
	want := runtimeTraceRootCauseTypeENLabel("priority_inversion_candidate") + " · " + tracefence.SupplyGapDominantWordEN
	if word != want {
		t.Fatalf("件1: EN compound word = %q, want the verdict-label form %q", word, want)
	}
	if strings.Contains(word, "priority_inversion_candidate") {
		t.Fatalf("件1: the raw wire token must not ride the EN compound word: %q", word)
	}
}

// TestRule3crMemberInheritedChipLiveSpecimen — 件9/§29.187① 活体标本臂
// (构造继承席 fixture): a hand-built observation set carrying the typed
// chain_identity_inheritance admission rides the FULL user-face render path
// (ApplyAndPersistMutation → RenderAnswerDocument) and the rendered ◎ face
// wears the ·成员继承 / ·member-inherited family chip (both languages).
func TestRule3crMemberInheritedChipLiveSpecimen(t *testing.T) {
	obs := []types.ObservationRecord{{
		ID: "chain-seat", Subject: "helper-400", Predicate: "root_cause_primary", Object: "runnable_wait",
		Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard,
		Value:           "17.000", Unit: "ms",
		RichNotes: []string{
			"rank=1", "tier=primary", "effective_impact_ms=17.000",
			"chain_relevance=on_chain", "dominant_state=runnable",
			"window_start_ts=1.000000", "window_end_ts=1.300000",
			"line_start=10", "line_end=20",
		},
	}, {
		ID: "identity-seat", Subject: "worker-200", Predicate: "root_cause_secondary", Object: "d_state_or_io_wait",
		Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard,
		Value:           "4.000", Unit: "ms",
		RichNotes: []string{
			"rank=2", "tier=secondary", "effective_impact_ms=4.000",
			"chain_relevance=on_chain", "dominant_state=d_sleep",
			"chain_identity_inheritance=true",
			"window_start_ts=1.000000", "window_end_ts=1.300000",
			"line_start=30", "line_end=40",
		},
	}}
	for _, lang := range []string{"zh", "en"} {
		md := p3mRenderUserFace(t, obs, lang)
		want := " ·" + tracefence.CredentialTierMemberInheritedZH
		if lang == "en" {
			want = " ·" + tracefence.CredentialTierMemberInheritedEN
		}
		if !strings.Contains(md, want) {
			t.Fatalf("活体标本臂 (%s): the member-inherited family chip %q must reach the rendered user face:\n%s", lang, want, md)
		}
	}
}
