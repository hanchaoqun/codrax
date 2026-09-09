package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestB1630CFrequencyPolicyActualFinalContextSeparatesCensusAndRepresentative(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			ctx := boundedRuntimeReaderHandoffTestContext()
			ctx.Language, ctx.AnalysisIR.RequestModel.Language = lang, lang
			a := ctx.Mutable.TurnAArtifacts()
			w := &a.ToolResults[0].TraceEvidenceAuthority.FrequencyLimitWitnesses[0]
			w.WitnessLine, w.WitnessTs = 17113, 13762.940114
			ctx.Mutable.SetTurnAArtifacts(*a)
			before, err := json.Marshal(ctx.Mutable.TurnAArtifacts())
			if err != nil {
				t.Fatal(err)
			}
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			readerMarker := "## Reader-ready finite-window facts"
			if lang == "zh" {
				readerMarker = "## 有限窗口查询的读者事实卡"
			}
			_, reader, found := strings.Cut(prompt, readerMarker)
			if !found {
				t.Fatalf("real finalizer prompt must carry the bounded fact reader (%s)", lang)
			}
			if i := strings.Index(reader, "\n## "); i >= 0 {
				reader = reader[:i]
			}
			wants := []string{"28 valid frequency-policy records for this CPU in the current query scope", "one representative record with the lowest positive upper bound", "558000–2100000 kHz", "not a repetition count", "not whole-window constancy or policy duration", "same-CPU target-slice overlap"}
			if lang == "zh" {
				wants = []string{"此 CPU 当前查询范围内有效策略记录共 28 条", "正上限最低的一条代表记录", "558000–2100000 kHz", "不是这组上下限重复出现的次数", "不证明整窗保持该策略或策略持续时长", "同一 CPU 上目标运行切片与策略的重叠"}
			}
			for _, want := range wants {
				if !strings.Contains(reader, want) {
					t.Errorf("reader did not distinguish record census from one selected tuple %q:\n%s", want, reader)
				}
			}
			if strings.Count(reader, "558000–2100000 kHz") != 2 {
				t.Fatalf("main policy card and target/CPU policy card must both retain their values:\n%s", reader)
			}
			// The ordinary prompt retains the exact original fields and adds
			// their ownership explanation; it does not retally or rename them.
			for _, want := range []string{
				"min=558000kHz max=2100000kHz limit_rows=28 witness_line=17113 witness_ts=13762.940114",
				"limit_rows counts valid policy-limit records for this CPU in the current query scope",
				"min/max and witness_line/witness_ts describe one representative record",
				"present:min=558000kHz,max=2100000kHz,rows=28",
				"one lowest-positive-ceiling record; rows count valid same-CPU/query records",
				"target_effect_unproven_no_slice_binding",
			} {
				if !strings.Contains(prompt, want) {
					t.Errorf("ordinary guidance/matrix lost field or caliber %q", want)
				}
			}
			after, err := json.Marshal(ctx.Mutable.TurnAArtifacts())
			if err != nil || string(before) != string(after) {
				t.Fatal("display changed original values, tuple, coordinates, count, or model handoff")
			}
		})
	}
}

func TestB1630CFrequencyReaderUnknownLocationDoesNotInventDurationOrCoordinates(t *testing.T) {
	ctx := boundedRuntimeReaderHandoffTestContext()
	got := renderAnswerDocBoundedRuntimeFinalReaderHandoff(ctx)
	if !strings.Contains(got, "代表记录的行号/时间只定位这一次事件") {
		t.Errorf("missing coordinates must not turn a query envelope into a policy interval:\n%s", got)
	}
	for _, forbidden := range []string{"第 0 行", "0.000000 秒发生", "持续 233.190", "重复 28", "28 次保持"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("legacy missing coordinates gained an event time/duration %q:\n%s", forbidden, got)
		}
	}
	a := ctx.Mutable.TurnAArtifacts()
	a.ToolResults[0].TraceEvidenceAuthority.FrequencyLimitWitnesses = nil
	ctx.Mutable.SetTurnAArtifacts(*a)
	got = renderAnswerDocBoundedRuntimeFinalReaderHandoff(ctx)
	if strings.Contains(got, "上限最低的一条代表记录") || strings.Contains(got, "共有 0 条频率策略记录") {
		t.Fatalf("absent policy witness was converted into an observed zero or selected record:\n%s", got)
	}
}
