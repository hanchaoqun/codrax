package orchestrator

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1597RankRecord(id, channel string, rank int, value string) types.ObservationRecord {
	record := psgTraceRecord("trace_query:"+id+"#root_cause_rank:1", "root_cause_tertiary", value,
		fmt.Sprintf("rank=%d", rank), "tier=tertiary", "effective_impact_ms="+value,
		"chain_relevance="+channel, "selected_window=34579.490000..34579.500000",
		"rank_board_target=com.baidu.tieba-59566", "rank_board_params_fingerprint=2609fdd5")
	record.SourceRef.CaptureIdentityPath = "/capture/donghu_tieba_frame.systrace"
	// Distinct query payloads keep their own observations in the generic
	// ledger; the rank display must still use the shared original capture.
	record.SourceRef.PayloadRef = id + ".json"
	record.Subject = "CookieMonsterCl-59843"
	record.Object = "runnable_wait"
	return record
}

// Use the actual shipping attachment, not just the fact-line helper. A model
// mentioning a thread only selects its facts; its words cannot assign a lane.
func b1597PublishedFacts(t *testing.T, lang string, records ...types.ObservationRecord) string {
	t.Helper()
	mut := psgTraceMutable(records...)
	bus := psgBus(mut)
	bus.Language = lang
	doc := psgProseDoc("CookieMonsterCl-59843: this wording is the model's own conclusion.")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	beforeDoc, err := json.Marshal(mut.ShippedAnswerDocumentV2())
	if err != nil {
		t.Fatal(err)
	}
	beforeLedger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	out := &agent.StageOutput{FinalAnswer: "model-owned answer bytes"}
	o := &Orchestrator{busCtx: bus}
	o.attachSystemCrossCheckAppendix(out, "", nil)
	attachments := mut.AnswerDisplayAttachments()
	if len(attachments) != 1 || attachments[0].Source != types.AnswerDisplayAttachmentSourceSystemCrossCheck {
		t.Fatalf("expected the shipping system attachment, got %+v", attachments)
	}
	afterDoc, err := json.Marshal(mut.ShippedAnswerDocumentV2())
	if err != nil {
		t.Fatal(err)
	}
	afterLedger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	if string(beforeDoc) != string(afterDoc) || !strings.Contains(out.FinalAnswer, doc.Blocks[0].Text) || !reflect.DeepEqual(beforeLedger, afterLedger) {
		t.Fatal("fact display mutated the model answer or typed evidence")
	}
	return attachments[0].Body
}

func TestB1597RankChannelsRemainDistinctAtPublication(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", lang, reverse), func(t *testing.T) {
				chain := b1597RankRecord("chain", "on_chain", 1, "0.111")
				adjacent := b1597RankRecord("adjacent", "adjacent", 1, "0.111")
				background := b1597RankRecord("background", "background", 1, "0.111")
				unknown := b1597RankRecord("unknown", "future_internal_channel", 1, "0.111")
				records := []types.ObservationRecord{chain, adjacent, background, unknown, chain}
				if reverse {
					for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
						records[i], records[j] = records[j], records[i]
					}
				}
				body := b1597PublishedFacts(t, lang, records...)
				labels := []string{"链上根因排序=#1", "邻近参考排序=#1", "背景参考排序=#1", "排序归属未标明=#1"}
				if lang == "en" {
					labels = []string{"on-chain root-cause rank(s)=#1", "adjacent reference rank(s)=#1", "background reference rank(s)=#1", "rank scope unspecified=#1"}
				}
				for _, label := range labels {
					if strings.Count(body, label) != 1 {
						t.Fatalf("same ordinal/value on a different typed channel must retain its meaning; want once %q:\n%s", label, body)
					}
				}
				if strings.Contains(body, "future_internal_channel") {
					t.Fatalf("unknown internal channel leaked into the reader surface:\n%s", body)
				}
			})
		}
	}
}

func TestB1597H8FactsDoNotRenameAdjacentRanksAsRoots(t *testing.T) {
	chain := b1597RankRecord("chain", "on_chain", 4, "0.116")
	adjacent := b1597RankRecord("adjacent", "adjacent", 1, "0.111")
	for _, lang := range []string{"zh", "en"} {
		body := b1597PublishedFacts(t, lang, chain, adjacent)
		if strings.Contains(body, "根因排序=#1") || strings.Contains(body, "root-cause rank(s)=#1") {
			t.Fatalf("H8's adjacent #1 was promoted to the root-cause ordinal space:\n%s", body)
		}
		for _, value := range []string{"0.116ms", "0.111ms"} {
			if !strings.Contains(body, value) {
				t.Fatalf("channel disclosure cannot drop the original value %s:\n%s", value, body)
			}
		}
	}
}

func TestB1597RankFactsKeepAllFourDomainAxes(t *testing.T) {
	for _, axis := range []string{"capture", "target", "window", "params"} {
		t.Run(axis, func(t *testing.T) {
			first := b1597RankRecord("first", "on_chain", 1, "0.111")
			other := b1597RankRecord("other", "on_chain", 1, "0.111")
			want := ""
			switch axis {
			case "capture":
				other.SourceRef.CaptureIdentityPath = "/other/donghu_tieba_frame.systrace"
				want = "/other/donghu_tieba_frame.systrace"
			case "target":
				other.RichNotes[5] = "rank_board_target=other-ui-42"
				want = "other-ui-42"
			case "window":
				other.RichNotes[4] = "selected_window=34579.490000..34579.510000"
				want = "34579.510000"
			case "params":
				other.RichNotes[6] = "rank_board_params_fingerprint=other-depth"
				want = "other-depth"
			}
			for _, lang := range []string{"zh", "en"} {
				body := b1597PublishedFacts(t, lang, first, other, first)
				if strings.Count(body, "0.111ms") != 2 || !strings.Contains(body, want) || !strings.Contains(body, "2609fdd5") {
					t.Fatalf("distinct %s domains collapsed or lost their typed scope:\n%s", axis, body)
				}
			}
		})
	}
}

func TestB1597IncompleteDomainDoesNotInventRootScope(t *testing.T) {
	first := b1597RankRecord("first", "", 1, "0.111")
	first.RichNotes[6] = "rank_board_params_fingerprint="
	second := first
	second.ID = "trace_query:second#root_cause_rank:1"
	second.SourceRef.PayloadRef = "second.json"
	for _, lang := range []string{"zh", "en"} {
		body := b1597PublishedFacts(t, lang, first, second, first)
		if strings.Contains(body, "根因排序=") || strings.Contains(body, "root-cause rank(s)=") || strings.Count(body, "0.111ms") != 2 {
			t.Fatalf("incomplete unrelated records must remain separate without borrowing root authority:\n%s", body)
		}
	}
}

func TestB1597RankFactBudgetAndUnprovenRemainderPreserved(t *testing.T) {
	var records []types.ObservationRecord
	for i := 1; i <= 5; i++ {
		records = append(records, b1597RankRecord(fmt.Sprintf("seat-%d", i), "on_chain", i, fmt.Sprintf("%d.111", i)))
	}
	remainder := b1597RankRecord("remainder", "on_chain", 6, "6.111")
	remainder.RichNotes = append(remainder.RichNotes, types.TraceNoteKeyDStateCauseUnprovenRemainder+"=true", types.TraceNoteKeyMemberCount+"=7")
	records = append(records, remainder)
	for _, lang := range []string{"zh", "en"} {
		body := b1597PublishedFacts(t, lang, records...)
		for _, value := range []string{"1.111ms", "4.111ms", "6.111ms"} {
			if !strings.Contains(body, value) {
				t.Fatalf("ordinary retained slots or unproven remainder exception lost %q:\n%s", value, body)
			}
		}
		if strings.Contains(body, "5.111ms") {
			t.Fatalf("the four-seat display budget must not grow:\n%s", body)
		}
		if !strings.Contains(body, "原因未证") && !strings.Contains(body, "cause unproven") {
			t.Fatalf("cause-unproven counter-face lost its disclosure:\n%s", body)
		}
	}
}

func TestB1597ScopeScalarsAreBoundedEscapedAndDisplayOnly(t *testing.T) {
	first := b1597RankRecord("first", "on_chain", 1, "0.111")
	other := b1597RankRecord("other", "on_chain", 1, "0.111")
	other.RichNotes[6] = "rank_board_params_fingerprint=depth\n```unsafe\n" + strings.Repeat("x", 4096)
	for _, lang := range []string{"zh", "en"} {
		body := b1597PublishedFacts(t, lang, first, other)
		if strings.Contains(body, "\n```unsafe") || strings.Contains(body, strings.Repeat("x", 161)) || len(body) > 2500 {
			t.Fatalf("scope scalar escaped its bounded data field:\n%s", body)
		}
		if !strings.Contains(body, "字符未展示") && !strings.Contains(body, "characters omitted") {
			t.Fatalf("truncated scope must disclose its display omission:\n%s", body)
		}
		if strings.Count(body, "0.111ms") != 2 {
			t.Fatalf("display truncation cannot merge distinct domains:\n%s", body)
		}
	}
}

func TestB1597NoSeatOnlyStatesObservedRecordAbsence(t *testing.T) {
	for _, channel := range []string{"on_chain", "adjacent", "background", "", "future_channel"} {
		for _, lang := range []string{"zh", "en"} {
			t.Run(channel+"/"+lang, func(t *testing.T) {
				other := b1597RankRecord("other", channel, 1, "0.111")
				other.Subject = "other-worker-101"
				mentioned := psgTraceRecord("trace_query:load#thread_cpu_load:1", "thread_cpu_load", "1.000")
				mentioned.Subject = "CookieMonsterCl-59843"
				body := b1597PublishedFacts(t, lang, other, mentioned)
				want := "本次排序记录中未见该线程"
				if lang == "en" {
					want = "this thread is absent from the observed rank records"
				}
				if !strings.Contains(body, want) {
					t.Fatalf("negative fact must stay bounded to the observed records:\n%s", body)
				}
				for _, banned := range []string{"未进入根因排序", "not present in the root-cause ranking", "future_channel"} {
					if strings.Contains(body, banned) {
						t.Fatalf("partial/off-chain records invented a complete root-ranking absence %q:\n%s", banned, body)
					}
				}
			})
		}
	}
}
