package types

import "testing"

// PARTSPLIT-1 双复核收编(对抗官 P3-1,2026-07-19):parser 层身份检查此前无
// 负臂——剥掉 identity check 全套仍绿(display 层同容差复验兜底,谎不出厂,
// 但防御纵深层无牙)。本电池给 parser 的每条 fail-closed 臂配独立负臂。
func partsplitShareRecord(mutate func(map[string]string)) ObservationRecord {
	notes := map[string]string{
		TraceNoteKeyGatedCompositeEdgePreShare:      "13.959",
		TraceNoteKeyGatedCompositeEdgePostShare:     "0.020",
		TraceNoteKeyGatedCompositeEdgeAccount:       "13.979",
		TraceNoteKeyGatedCompositeEdgeAnchorTs:      "34579.555890",
		TraceNoteKeyGatedCompositeEdgeAnchorVia:     "direct",
		TraceNoteKeyGatedCompositeEdgeSeatPublished: "false",
	}
	if mutate != nil {
		mutate(notes)
	}
	rich := make([]string, 0, len(notes))
	for k, v := range notes {
		if v != "" {
			rich = append(rich, k+"="+v)
		}
	}
	return ObservationRecord{Subject: "Binder:43397_19-23088", RichNotes: rich}
}

func TestGatedCompositeEdgeShareParserFailClosedArms(t *testing.T) {
	if out, ok := traceCausalProjectionGatedCompositeEdgeShareFromRecord(partsplitShareRecord(nil)); !ok {
		t.Fatalf("positive control must parse, got drop")
	} else if out.PreMS != 13.959 || out.PostMS != 0.020 || out.AccountMS != 13.979 || out.SeatPublished {
		t.Fatalf("positive control fields wrong: %+v", out)
	}
	cases := []struct {
		name   string
		mutate func(map[string]string)
	}{
		{"identity broken beyond 3µs headroom", func(n map[string]string) {
			n[TraceNoteKeyGatedCompositeEdgeAccount] = "13.984"
		}},
		{"partial quartet: account note absent", func(n map[string]string) {
			n[TraceNoteKeyGatedCompositeEdgeAccount] = ""
		}},
		{"seat_published outside strict true/false", func(n map[string]string) {
			n[TraceNoteKeyGatedCompositeEdgeSeatPublished] = "yes"
		}},
		{"via outside the R3 closed set", func(n map[string]string) {
			n[TraceNoteKeyGatedCompositeEdgeAnchorVia] = "inferred"
		}},
		{"non-positive pre share", func(n map[string]string) {
			n[TraceNoteKeyGatedCompositeEdgePreShare] = "0.000"
		}},
	}
	for _, tc := range cases {
		if _, ok := traceCausalProjectionGatedCompositeEdgeShareFromRecord(partsplitShareRecord(tc.mutate)); ok {
			t.Fatalf("%s: record must drop whole (all-or-nothing), got parsed", tc.name)
		}
	}
}
