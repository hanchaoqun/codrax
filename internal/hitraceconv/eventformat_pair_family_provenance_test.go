package hitraceconv

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPoisonedEventFormatRetainsOnlyExactPairCriticalFamilyProvenance(t *testing.T) {
	workqueue := pairFamilyFormatBlock("workqueue_execute_start", 301, `"work=%p function=%p"`)
	dmaFence := pairFamilyFormatBlock("dma_fence_wait_end", 301, `"driver=%s timeline=%s context=%u seqno=%u"`)
	catalog, err := parseEventFormats([]byte(workqueue + "\n" + dmaFence))
	if err != nil {
		t.Fatal(err)
	}
	if !catalog.Poisoned[301] {
		t.Fatalf("cross-family descriptor conflict was not quarantined: %+v", catalog)
	}
	wantBoth := pairCriticalFormatFamilyWorkqueue | pairCriticalFormatFamilyDMAFence
	if got := catalog.PoisonedFamilies[301]; got != wantBoth {
		t.Fatalf("cross-family provenance=%02b, want %02b", got, wantBoth)
	}

	malformedDMA := pairFamilyFormatBlock("dma_fence_wait_start", 302, `"first"`) + "\nprint fmt: \"duplicate\""
	catalog, err = parseEventFormats([]byte(malformedDMA))
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.PoisonedFamilies[302]; got != pairCriticalFormatFamilyDMAFence {
		t.Fatalf("malformed exact DMA descriptor provenance=%02b, want DMA", got)
	}

	for _, fixture := range []struct {
		id   int
		body string
	}{
		{303, strings.Replace(pairFamilyFormatBlock("workqueue_execute_end", 303, `"work=%p"`), "name: workqueue_execute_end\n", "", 1)},
		{304, pairFamilyFormatBlock("workqueue_execute_begin", 304, `"work=%p"`) + "\n" + pairFamilyFormatBlock("sched_wakeup", 304, `"pid=%d"`)},
	} {
		candidate, err := parseEventFormats([]byte(fixture.body))
		if err != nil {
			t.Fatal(err)
		}
		if !candidate.Poisoned[fixture.id] {
			t.Fatalf("fixture ID %d was not quarantined: %+v", fixture.id, candidate)
		}
		if got, exists := candidate.PoisonedFamilies[fixture.id]; exists || got != 0 {
			t.Fatalf("anonymous/near-miss ID %d acquired guessed family provenance: mask=%02b exists=%v", fixture.id, got, exists)
		}
	}
	precedingExact := pairFamilyFormatBlock("workqueue_execute_start", 306, `"work=%p function=%p"`)
	followingAnonymous := strings.Replace(
		pairFamilyFormatBlock("dma_fence_wait_end", 307, `"driver=%s timeline=%s context=%u seqno=%u"`),
		"name: dma_fence_wait_end\n", "", 1,
	)
	catalog, err = parseEventFormats([]byte(precedingExact + "\n" + followingAnonymous))
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.PoisonedFamilies[306]; got != pairCriticalFormatFamilyWorkqueue {
		t.Fatalf("preceding exact descriptor lost its family when followed by anonymous syntax: %02b", got)
	}
	if got, exists := catalog.PoisonedFamilies[307]; exists || got != 0 {
		t.Fatalf("anonymous descriptor inherited the preceding exact family: mask=%02b exists=%v", got, exists)
	}

	clean, err := parseEventFormats([]byte(pairFamilyFormatBlock("workqueue_execute_end", 305, `"work=%p"`)))
	if err != nil {
		t.Fatal(err)
	}
	if clean.Poisoned[305] || len(clean.PoisonedFamilies) != 0 {
		t.Fatalf("non-poisoned descriptor leaked into poison provenance: %+v", clean)
	}
}

func TestPoisonedEventFormatFamilyProvenanceUnionsAcrossSegments(t *testing.T) {
	destination, err := parseEventFormats([]byte(pairFamilyFormatBlock("workqueue_execute_end", 311, `"work=%p"`)))
	if err != nil {
		t.Fatal(err)
	}
	source, err := parseEventFormats([]byte(
		pairFamilyFormatBlock("dma_fence_wait_start", 311, `"driver=%s timeline=%s context=%u seqno=%u"`) + "\n" +
			pairFamilyFormatBlock("sched_switch", 311, `"synthetic"`),
	))
	if err != nil {
		t.Fatal(err)
	}
	if got := source.PoisonedFamilies[311]; got != pairCriticalFormatFamilyDMAFence {
		t.Fatalf("source poison lost DMA provenance: %02b", got)
	}
	mergeEventFormatCatalog(&destination, source)
	wantBoth := pairCriticalFormatFamilyWorkqueue | pairCriticalFormatFamilyDMAFence
	if got := destination.PoisonedFamilies[311]; got != wantBoth {
		t.Fatalf("cross-segment poison provenance=%02b, want %02b", got, wantBoth)
	}

	// Once an ID is poisoned, a later clean descriptor cannot rescue it, but
	// its exact family name remains useful provenance and must still be unioned.
	base := pairFamilyFormatBlock("sched_wakeup", 312, `"pid=%d"`) + "\n" +
		pairFamilyFormatBlock("sched_switch", 312, `"synthetic"`)
	alreadyPoisoned, err := parseEventFormats([]byte(base))
	if err != nil {
		t.Fatal(err)
	}
	laterExact, err := parseEventFormats([]byte(pairFamilyFormatBlock("dma_fence_wait_end", 312, `"driver=%s timeline=%s context=%u seqno=%u"`)))
	if err != nil {
		t.Fatal(err)
	}
	mergeEventFormatCatalog(&alreadyPoisoned, laterExact)
	if !alreadyPoisoned.Poisoned[312] {
		t.Fatal("later descriptor rescued a poisoned ID")
	}
	if got := alreadyPoisoned.PoisonedFamilies[312]; got != pairCriticalFormatFamilyDMAFence {
		t.Fatalf("later exact descriptor provenance was discarded: %02b", got)
	}
}

func TestScanMetadataExposesPoisonedPairCriticalFamilyProvenance(t *testing.T) {
	var capture bytes.Buffer
	writeFileHeader(&capture, 1)
	writeSegment(&capture, segmentEventsFormat, []byte(pairFamilyFormatBlock("workqueue_execute_start", 321, `"work=%p function=%p"`)))
	writeSegment(&capture, segmentEventsFormat, []byte(pairFamilyFormatBlock("dma_fence_wait_end", 321, `"driver=%s timeline=%s context=%u seqno=%u"`)))
	writeSegment(&capture, segmentEventsFormat, []byte(strings.Replace(
		pairFamilyFormatBlock("workqueue_execute_end", 322, `"work=%p"`),
		"name: workqueue_execute_end\n", "", 1,
	)))

	path := filepath.Join(t.TempDir(), "pair-family-provenance.sys")
	if err := os.WriteFile(path, capture.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	meta, err := scanMetadata(context.Background(), path, int64(capture.Len()))
	if err != nil {
		t.Fatal(err)
	}
	wantBoth := pairCriticalFormatFamilyWorkqueue | pairCriticalFormatFamilyDMAFence
	if got := meta.formatPoisonFamilies[321]; got != wantBoth {
		t.Fatalf("metadata family provenance=%02b, want %02b", got, wantBoth)
	}
	if got, exists := meta.formatPoisonFamilies[322]; exists || got != 0 {
		t.Fatalf("metadata guessed anonymous descriptor family: mask=%02b exists=%v", got, exists)
	}
}

func pairFamilyFormatBlock(name string, id int, printFmt string) string {
	return strings.Join([]string{
		"name: " + name,
		"ID: " + strconv.Itoa(id),
		"format:",
		"\tfield:unsigned short common_type; offset:0; size:2; signed:0;",
		"\tfield:unsigned char common_flags; offset:2; size:1; signed:0;",
		"\tfield:unsigned char common_preempt_count; offset:3; size:1; signed:0;",
		"\tfield:int common_pid; offset:4; size:4; signed:1;",
		"print fmt: " + printFmt,
	}, "\n")
}
