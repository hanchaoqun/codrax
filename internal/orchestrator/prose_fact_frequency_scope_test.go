package orchestrator

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1633FrequencyRecord(id, path, subject, predicate, window string, khz int) types.ObservationRecord {
	r := psgTraceRecord(id, predicate+":"+subject, "1.5",
		"cpu=7", fmt.Sprintf("freq=%dkHz", khz), "selected_window="+window)
	r.Subject, r.Predicate, r.Object = subject, predicate, "running"
	r.SourceRef = types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact,
		Path: path, PayloadRef: path + ".query.json", RawRef: path + ".query.json",
		QueryScopeID: "opaque-query-" + id}
	r.ObservedAt = "2026-09-09T08:00:00Z"
	return r
}

func b1633PublishedFrequencyAppendix(t *testing.T, lang string, records []types.ObservationRecord) string {
	t.Helper()
	mut := psgTraceMutable(records...)
	bus := psgBus(mut)
	bus.Language = lang
	doc := psgProseDoc("CPU7 frequency=777000 kHz; the model's explanation stays unchanged.")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	before, err := json.Marshal(mut.AnswerDocumentV2())
	if err != nil {
		t.Fatal(err)
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	if len(ledger.Records) < len(records) {
		t.Fatalf("fixture lost records before the frequency consumer: %d < %d", len(ledger.Records), len(records))
	}
	for _, r := range records {
		found := false
		for _, got := range ledger.Records {
			if got.ID == r.ID && got.Subject == r.Subject && got.SourceRef.Path == r.SourceRef.Path &&
				proseWallClockNoteValue(got.RichNotes, "freq") == proseWallClockNoteValue(r.RichNotes, "freq") {
				found = true
			}
		}
		if !found {
			t.Fatalf("raw source/subject/frequency did not survive the actual ledger: %+v", r)
		}
	}
	o := &Orchestrator{busCtx: bus}
	modelAnswer := render.RenderAnswerDocument(doc, lang)
	out := &agent.StageOutput{FinalAnswer: modelAnswer}
	o.attachSystemCrossCheckAppendix(out, "", nil)
	if !strings.HasPrefix(out.FinalAnswer, modelAnswer) {
		t.Fatalf("appendix rewrote model answer: %q", out.FinalAnswer)
	}
	after, err := json.Marshal(mut.AnswerDocumentV2())
	if err != nil || string(before) != string(after) {
		t.Fatalf("appendix mutated accepted model document: err=%v before=%s after=%s", err, before, after)
	}
	atts := mut.AnswerDisplayAttachments()
	if len(atts) != 1 || atts[0].Source != types.AnswerDisplayAttachmentSourceSystemCrossCheck {
		t.Fatalf("want actual system fact appendix: %+v", atts)
	}
	return atts[0].Body
}

func TestB1633FrequencyAppendixRetainsEachSourceSubjectAndWindow(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a", "same.systrace")
	b := filepath.Join(root, "b", "same.systrace")
	rows := []types.ObservationRecord{
		b1633FrequencyRecord("a1", a, "worker-101", "running_time", "10..11", 558000),
		b1633FrequencyRecord("a2", a, "other-202", "runnable_wait", "10..11", 640000),
		b1633FrequencyRecord("b1", b, "worker-101", "running_time", "20..21", 1380000),
	}
	rows[1].Object = "runnable"
	for _, lang := range []string{"zh", "en"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", lang, reverse), func(t *testing.T) {
				input := append([]types.ObservationRecord(nil), rows...)
				if reverse {
					input[0], input[2] = input[2], input[0]
				}
				body := b1633PublishedFrequencyAppendix(t, lang, input)
				for _, want := range []struct{ value, subject, path, window string }{
					{"558MHz", "worker-101", a, "10.000000..11.000000"},
					{"640MHz", "other-202", a, "10.000000..11.000000"},
					{"1380MHz", "worker-101", b, "20.000000..21.000000"},
				} {
					matches := 0
					for _, line := range strings.Split(body, "\n") {
						if !strings.Contains(line, want.value) {
							continue
						}
						matches++
						for _, piece := range []string{want.subject, want.path, want.path + ".query.json", want.window} {
							if !strings.Contains(line, piece) {
								t.Errorf("frequency %s lost its own %q: %s", want.value, piece, line)
							}
						}
					}
					if matches != 1 {
						t.Errorf("want one scoped line for %s, got %d:\n%s", want.value, matches, body)
					}
				}
				for _, forbidden := range []string{"opaque-query-", "running_time", "runnable_wait", "窗内观测频点=", "observed in-window frequency point"} {
					if strings.Contains(body, forbidden) {
						t.Errorf("frequency appendix exposed opaque identity or overclaimed measurement %q:\n%s", forbidden, body)
					}
				}
				wants := []string{"representative", "not", "raw", "entire"}
				if lang == "zh" {
					wants = []string{"附带频率", "不证明", "原始", "全程"}
				}
				for _, want := range wants {
					if !strings.Contains(body, want) {
						t.Errorf("missing measurement boundary %q:\n%s", want, body)
					}
				}
			})
		}
	}
}

func TestB1633FrequencySameCPUDoesNotMergeBucketCaseOrNearbyValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.systrace")
	rows := []types.ObservationRecord{
		b1633FrequencyRecord("r1", path, "worker-10", "running_time", "1..2", 1000000),
		b1633FrequencyRecord("r2", path, "Worker-10", "running_time", "1..2", 1000100),
		b1633FrequencyRecord("r3", path, "worker-10", "sleep_wait", "1..2", 1000200),
		b1633FrequencyRecord("r4", path, "worker-10", "running_time", "3..4", 1000300),
	}
	rows[2].Object = "sleep"
	for _, lang := range []string{"zh", "en"} {
		body := b1633PublishedFrequencyAppendix(t, lang, rows)
		for _, value := range []string{"1000MHz", "1000.1MHz", "1000.2MHz", "1000.3MHz"} {
			if strings.Count(body, value) != 1 {
				t.Fatalf("nearby values must not coalesce: missing %s:\n%s", value, body)
			}
		}
		for _, exact := range []string{"worker-10", "Worker-10", "1.000000..2.000000", "3.000000..4.000000"} {
			if !strings.Contains(body, exact) {
				t.Fatalf("lost exact scope %q:\n%s", exact, body)
			}
		}
		if lang == "zh" && (!strings.Contains(body, "睡眠状态统计桶") || !strings.Contains(body, "运行状态统计桶")) {
			t.Fatalf("state buckets must stay distinct:\n%s", body)
		}
	}
}

func TestB1633FrequencyUnknownScopeAndUnknownPredicateStayLocal(t *testing.T) {
	r := b1633FrequencyRecord("unknown", "", "worker-10", "future_internal_bucket", "", 987600)
	r.SourceRef = types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact}
	r.RichNotes = []string{"cpu=7", "freq=987600", "window=10..20"}
	r.Span = types.ObservationSpan{StartTs: 10, EndTs: 20, LineStart: 12, LineEnd: 19}
	for _, lang := range []string{"zh", "en"} {
		body := b1633PublishedFrequencyAppendix(t, lang, []types.ObservationRecord{r})
		wants := []string{"987.6MHz", "worker-10", "12-19", "source not stated", "query window not stated", "record measurement scope not stated"}
		if lang == "zh" {
			wants = []string{"987.6MHz", "worker-10", "12-19", "来源未明确", "查询范围未明确", "记录口径未明确"}
		}
		for _, want := range wants {
			if !strings.Contains(body, want) {
				t.Errorf("unknown record lost %q:\n%s", want, body)
			}
		}
		for _, forbidden := range []string{"future_internal_bucket", "10.000000..20.000000", "0.000000..0.000000"} {
			if strings.Contains(body, forbidden) {
				t.Errorf("unknown query/predicate invented authority %q:\n%s", forbidden, body)
			}
		}
	}
}

func TestB1633FrequencyDedupNeedsFullIdenticalRecordAndKeepsDisplayCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.systrace")
	full := b1633FrequencyRecord("full", path, "worker-10", "running_time", "1..2", 700000)
	unknown := b1633FrequencyRecord("unknown", path, "worker-20", "running_time", "1..2", 800000)
	unknown.SourceRef.QueryScopeID = ""
	unknown.ObservedAt = ""
	input := []types.ObservationRecord{full, full, unknown, unknown}
	for i := 0; i < 4; i++ {
		input = append(input, b1633FrequencyRecord(fmt.Sprintf("extra-%d", i), path,
			fmt.Sprintf("extra-%d", i+30), "running_time", "1..2", 900000+i*100000))
	}
	before, _ := json.Marshal(input)
	findings := proseTypedFactJuxtapositionFindingsImpl(psgProseDoc("CPU7 freq=1"), psgBus(psgTraceMutable()), psgTraceMutable())
	if len(findings) != 0 {
		t.Fatalf("empty production ledger must stay quiet: %+v", findings)
	}
	// Use the same production frequency provider with an explicit ledger to
	// keep duplicate rows visible to its own dedup boundary, before ledger dedup.
	findings = proseFactFrequencyScopeFindings(types.ObservationLedger{Records: input}, collectModelProseUnits(psgProseDoc("CPU7 freq=1")))
	if len(findings) != 1 {
		t.Fatalf("CPU display budget/group changed: %+v", findings)
	}
	for _, lang := range []string{"zh", "en"} {
		body := findings[0].userReadable(lang)
		if strings.Count(body, "700MHz") != 1 || strings.Count(body, "800MHz") != 2 || strings.Count(body, "900MHz") != 1 {
			t.Fatalf("full duplicate should collapse; unknown records and fourth row must remain:\n%s", body)
		}
		if strings.Contains(body, "1000MHz") {
			t.Fatalf("per-CPU four-record budget exceeded:\n%s", body)
		}
		want := "3 more omitted"
		if lang == "zh" {
			want = "另省略 3 条记录"
		}
		if !strings.Contains(body, want) {
			t.Fatalf("display omission not disclosed accurately:\n%s", body)
		}
	}
	after, _ := json.Marshal(input)
	if string(before) != string(after) {
		t.Fatal("frequency provider modified raw observations")
	}
}

func TestB1633FrequencyMissingValueIsNotTraceAbsenceAndSelectionStaysSoft(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.systrace")
	r := b1633FrequencyRecord("no-value", path, "worker-10", "running_time", "1..2", 0)
	for _, lang := range []string{"zh", "en"} {
		body := b1633PublishedFrequencyAppendix(t, lang, []types.ObservationRecord{r})
		want := "positive representative frequency not provided"
		if lang == "zh" {
			want = "未提供正的附带频率值"
		}
		if !strings.Contains(body, want) || !strings.Contains(body, "worker-10") || strings.Contains(body, "0MHz") {
			t.Fatalf("missing value must remain record-local, not measured zero:\n%s", body)
		}
	}
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{r}}
	for _, prose := range []string{"CPU7 was active", "frequency=123000"} {
		if got := proseFactFrequencyScopeFindings(ledger, collectModelProseUnits(psgProseDoc(prose))); len(got) != 0 {
			t.Fatalf("original two-token selection widened for %q: %+v", prose, got)
		}
	}
	got := proseFactFrequencyScopeFindings(ledger, collectModelProseUnits(psgProseDoc("CPU7 CPU8 CPU9 CPU10 frequency=1")))
	if len(got) != proseFactCPUCap {
		t.Fatalf("original CPU cap changed: %+v", got)
	}
	for _, f := range got {
		if strings.Contains(f.entry, "CPU10") || strings.Contains(f.entryZH, "无窗内频率观测记录") {
			t.Fatalf("budget/absence boundary regressed: %+v", got)
		}
	}
}
