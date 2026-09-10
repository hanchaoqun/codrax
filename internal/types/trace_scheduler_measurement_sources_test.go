package types

import (
	"math"
	"reflect"
	"testing"
)

func TestSchedulerMeasurementSourcesCompleteExactUnion(t *testing.T) {
	base := TraceSchedulerMeasurementDomain{1, "constructed_partition", "thread_timeline", 7, 1, 2, 3, 4, "partition"}
	typ := reflect.TypeOf(base)
	wantFields := []string{"Version", "Status", "Method", "TargetTID", "WindowStartTs", "WindowEndTs", "QueryLineStart", "QueryLineEnd", "PartitionID"}
	if typ.NumField() != len(wantFields) {
		t.Fatal("new native domain field requires exact-key/clone disposition")
	}
	var all []*TraceSchedulerMeasurementSources
	all = append(all, TraceSchedulerMeasurementSourcesFromDomain(&base))
	for i, name := range wantFields {
		if typ.Field(i).Name != name {
			t.Fatal("native domain field census changed")
		}
		d := base
		field := reflect.ValueOf(&d).Elem().Field(i)
		switch field.Kind() {
		case reflect.String:
			field.SetString(field.String() + ":different")
		case reflect.Int:
			field.SetInt(field.Int() + 1)
		case reflect.Float64:
			field.SetFloat(math.Nextafter(field.Float(), math.Inf(1)))
		default:
			t.Fatalf("unhandled source field %s", name)
		}
		all = append(all, TraceSchedulerMeasurementSourcesFromDomain(&d))
	}
	merged := MergeTraceSchedulerMeasurementSources(all...)
	if merged.HasUnknown || len(merged.Domains) != 10 {
		t.Fatalf("exact field distinctions lost: %+v", merged)
	}
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	if !reflect.DeepEqual(merged, MergeTraceSchedulerMeasurementSources(all...)) ||
		!reflect.DeepEqual(merged, MergeTraceSchedulerMeasurementSources(merged, merged)) {
		t.Fatal("source union is order dependent or non-idempotent")
	}
	// This is not a bounded display roster: no source may disappear at 8/32/128.
	all = nil
	for i := 0; i < 257; i++ {
		d := base
		d.TargetTID = i + 1
		all = append(all, TraceSchedulerMeasurementSourcesFromDomain(&d))
	}
	if got := len(MergeTraceSchedulerMeasurementSources(all...).Domains); got != 257 {
		t.Fatalf("native source inventory truncated: %d", got)
	}
}

func TestSchedulerMeasurementSourcesUnknownAndOwnership(t *testing.T) {
	d := TraceSchedulerMeasurementDomain{PartitionID: "original"}
	known := TraceSchedulerMeasurementSourcesFromDomain(&d)
	d.PartitionID = "changed"
	if known.Domains[0].PartitionID != "original" {
		t.Fatal("source input aliases native receipt")
	}
	if MergeTraceSchedulerMeasurementSources() != nil || MergeTraceSchedulerMeasurementSources(nil, nil) != nil || TraceSchedulerMeasurementSourcesFromDomain(nil) != nil {
		t.Fatal("legacy absence acquired a receipt")
	}
	for _, inputs := range [][]*TraceSchedulerMeasurementSources{{known, nil}, {nil, known}, {known, {}}, {known, {HasUnknown: true}}} {
		got := MergeTraceSchedulerMeasurementSources(inputs...)
		if got == nil || !got.HasUnknown || len(got.Domains) != 1 {
			t.Fatalf("unknown member disappeared: %+v", got)
		}
		if !MergeTraceSchedulerMeasurementSources(got, known).HasUnknown {
			t.Fatal("later known member cleared unknown")
		}
		copy := CloneTraceSchedulerMeasurementSources(got)
		copy.Domains[0].PartitionID = "mutated"
		if got.Domains[0].PartitionID != "original" || known.Domains[0].PartitionID != "original" {
			t.Fatal("union/clone shares mutable domain storage")
		}
	}
}

func TestSchedulerMeasurementSourcesDoNotExpandPromptOrChangeClaims(t *testing.T) {
	r := ObservationRecord{ID: "obs", Origin: AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", Subject: "worker-7", Predicate: "wakeup_causal_impact",
		Value: "12.000", Unit: "ms", Summary: "Observed scheduler wait", RichNotes: []string{"on_chain=true"}}
	opts := DefaultObservationPromptProjectionOptions(10)
	want := ProjectObservationPromptRecords([]ObservationRecord{r}, nil, nil, opts)
	d := TraceSchedulerMeasurementDomain{PartitionID: "opaque-native-id"}
	r.MeasurementSources = TraceSchedulerMeasurementSourcesFromDomain(&d)
	r.MeasurementSources.HasUnknown = true
	got := ProjectObservationPromptRecords([]ObservationRecord{r}, nil, nil, opts)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("source bookkeeping changed prompt facts or injected internal source terminology")
	}
}
