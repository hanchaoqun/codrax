package tracequery

import "testing"

func TestIOFamilyNeverMergesAcrossPhysicalArtifacts(t *testing.T) {
	q := Query{TimeStart: 5.000, TimeEnd: 5.010, Limit: 12}
	thread := ThreadRef{PID: 200, TGID: 200, Comm: "io-worker"}
	stats := WindowStats{IOLatencies: []IOLatencySummary{
		{
			SourcePath:     "/trace/source-a.systrace",
			Dev:            "8,0",
			Op:             "R",
			Sector:         128,
			Len:            8,
			IssueThread:    thread,
			CompleteThread: thread,
			IssueTs:        5.001,
			CompleteTs:     5.004,
			DurationMs:     3,
			IssueLine:      10,
			CompleteLine:   11,
		},
		{
			SourcePath:     "/trace/source-b.systrace",
			Dev:            "8,0",
			Op:             "R",
			Sector:         128,
			Len:            8,
			IssueThread:    thread,
			CompleteThread: thread,
			IssueTs:        5.001,
			CompleteTs:     5.004,
			DurationMs:     3,
			IssueLine:      10,
			CompleteLine:   11,
		},
	}}

	rank := buildRootCauseRankFrom(nil, q, ChainResult{}, stats)
	seen := map[string]int{}
	for _, item := range rank.Items {
		if item.Type == "io_latency" {
			seen[item.PhysicalSourcePath]++
		}
	}
	if len(seen) != 2 || seen["/trace/source-a.systrace"] != 1 || seen["/trace/source-b.systrace"] != 1 {
		t.Fatalf("lookalike IO intervals from two physical artifacts must retain two independent rank seats: %+v", rank.Items)
	}
}

func TestSemanticFamilyNeverCrossFoldsPhysicalArtifacts(t *testing.T) {
	thread := ThreadRef{PID: 200, TGID: 200, Comm: "worker"}
	span := func(source, name string, start, end float64, line int) TraceSpanSummary {
		row := rcmSpan(thread, name, start, end, line, line+1)
		row.SourcePath = source
		return row
	}
	spans := []TraceSpanSummary{
		span("/trace/source-a.systrace", "VerifyClass com.example.A", 5.001, 5.002, 10),
		span("/trace/source-a.systrace", "VerifyClass com.example.B", 5.003, 5.004, 12),
		// Same thread, semantic class, timestamps and local lines in another
		// attachment are a different physical occurrence set.
		span("/trace/source-b.systrace", "VerifyClass com.example.A", 5.001, 5.002, 10),
		span("/trace/source-b.systrace", "VerifyClass com.example.B", 5.003, 5.004, 12),
	}

	families := FoldSemanticSpanFamilies(nil, spans)
	if len(families) != 2 {
		t.Fatalf("semantic family identity must include the physical artifact, got %+v", families)
	}
	items := make([]RootCauseRankItem, 0, len(families))
	seenFamilies := map[string]int{}
	for _, family := range families {
		seenFamilies[family.SourcePath]++
		if len(family.Members) != 2 {
			t.Fatalf("each physical artifact should retain its own two-member family: %+v", family)
		}
		item, ok := rootCauseItemFromSemanticSpanFamily(Query{TimeStart: 5.000, TimeEnd: 5.010}, family, false)
		if !ok {
			t.Fatalf("semantic family did not mint a rank item: %+v", family)
		}
		items = append(items, item)
	}
	if seenFamilies["/trace/source-a.systrace"] != 1 || seenFamilies["/trace/source-b.systrace"] != 1 {
		t.Fatalf("family source provenance was not preserved: %+v", families)
	}

	items = foldSameThreadTypeRankFamilies(Query{TimeStart: 5.000, TimeEnd: 5.010}, false, items)
	if len(items) != 2 {
		t.Fatalf("semantic rank families from different physical artifacts must not cross-fold: %+v", items)
	}
	seenItems := map[string]int{}
	for _, item := range items {
		seenItems[item.PhysicalSourcePath]++
	}
	if seenItems["/trace/source-a.systrace"] != 1 || seenItems["/trace/source-b.systrace"] != 1 {
		t.Fatalf("semantic rank rows lost physical source provenance: %+v", items)
	}
}
