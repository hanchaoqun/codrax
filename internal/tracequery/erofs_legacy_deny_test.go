package tracequery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var legacyEROFSSemanticDenyNames = [...]string{
	"erofs_read_enter",
	"erofs_read_exit",
	"erofs_read_iter_enter",
	"erofs_read_iter_exit",
	"erofs_readdir",
	"erofs_lookup_start",
	"erofs_lookup_end",
	"erofs_getattr_enter",
	"erofs_getattr_exit",
	"erofs_listxattr_enter",
	"erofs_listxattr_exit",
	"erofs_raw_access_readpages_start",
	"erofs_raw_access_readpages_end",
	"erofs_read_raw_page_start",
	"erofs_read_raw_page_end",
	"z_erofs_vle_normalaccess_readpage_start",
	"z_erofs_vle_normalaccess_readpage_end",
	"z_erofs_vle_normalaccess_readpages_start",
	"z_erofs_vle_normalaccess_readpages_end",
}

var upstreamEROFSSemanticDenyRepresentatives = [...]string{
	"erofs_lookup",
	"erofs_fill_inode",
	"erofs_readpage",
	"erofs_readpages",
	"erofs_read_folio",
	"erofs_map_blocks_flatmode_enter",
	"erofs_map_blocks_flatmode_exit",
	"erofs_map_blocks_enter",
	"erofs_map_blocks_exit",
	"z_erofs_map_blocks_iter_enter",
	"z_erofs_map_blocks_iter_exit",
	"erofs_destroy_inode",
}

func TestEROFSCoverageOnlyNameCandidateScope(t *testing.T) {
	for _, name := range []string{
		"erofs_lookup_start",
		" EROFS_LOOKUP_START ",
		"z_erofs_map_blocks_iter_enter",
		"\tZ_EROFS_FUTURE_VENDOR_EVENT\t",
	} {
		if !EROFSCoverageOnlyNameCandidate(name) {
			t.Errorf("EROFS name escaped the shared coverage-only gate: %q", name)
		}
	}
	for _, name := range []string{
		"sched_blocked_reason",
		"caller=z_erofs_readpage",
		"x_erofs_lookup_start",
		"erofs",
		"z_erofs",
		"ext4_erofs_bridge",
	} {
		if EROFSCoverageOnlyNameCandidate(name) {
			t.Errorf("non-EROFS event name entered the coverage-only gate: %q", name)
		}
	}
}

func legacyEROFSNearName(name string) string {
	for _, suffix := range []string{"_start", "_end", "_enter", "_exit"} {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix) + suffix + "x"
		}
	}
	return name + "x"
}

func legacyEROFSNameVariants(name string) []struct {
	kind string
	name string
} {
	return []struct {
		kind string
		name string
	}{
		{kind: "exact", name: name},
		{kind: "case", name: strings.ToUpper(name)},
		{kind: "near", name: legacyEROFSNearName(name)},
		{kind: "suffix", name: name + "_vendor"},
	}
}

func legacyEROFSTestLine(line int, ts float64, name string) string {
	return fmt.Sprintf("erofs-42 (42) [001] .... %.6f: %s: dev=8:0 ino=0x9 nid=17 index=3 pos=0 len=4096 bytes=4096 rw=read ret=0", ts, name)
}

func TestLegacyEROFSNamesRemainRawUnknownInventory(t *testing.T) {
	var failures []string
	line := 1
	for _, canonical := range legacyEROFSSemanticDenyNames {
		for _, variant := range legacyEROFSNameVariants(canonical) {
			raw := legacyEROFSTestLine(line, 1+float64(line)/1000, variant.name)
			ev, ok := ParseLine(line, raw, newStringInterner())
			if !ok {
				failures = append(failures, canonical+"/"+variant.kind+": rejected instead of retained")
			} else if ev.Type != EventUnknown || ev.SubsystemKind != "" || ev.FileFields != nil || ev.ResourceFields != nil {
				failures = append(failures, fmt.Sprintf("%s/%s: type=%s subsystem=%q file=%t resource=%t",
					canonical, variant.kind, ev.Type, ev.SubsystemKind, ev.FileFields != nil, ev.ResourceFields != nil))
			}
			if durationOrderRawCandidate(raw) || durationEndpointFallbackCandidate(raw) {
				failures = append(failures, canonical+"/"+variant.kind+": entered a raw duration candidate lane")
			}
			line++
		}
	}
	if len(failures) != 0 {
		limit := len(failures)
		if limit > 12 {
			limit = 12
		}
		t.Fatalf("legacy EROFS semantic deny failed for %d observations (first %d):\n%s", len(failures), limit, strings.Join(failures[:limit], "\n"))
	}
}

func TestLegacyEROFSRawSearchSurvivesWhileSemanticSurfacesStayEmpty(t *testing.T) {
	var lines []string
	for _, canonical := range legacyEROFSSemanticDenyNames {
		for _, variant := range legacyEROFSNameVariants(canonical) {
			line := len(lines) + 1
			lines = append(lines, legacyEROFSTestLine(line, 1+float64(line)/1000, variant.name))
		}
	}
	path := filepath.Join(t.TempDir(), "erofs-legacy-raw-only.systrace")
	if err := os.WriteFile(path, []byte(strings.Join(append(lines, ""), "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{View: "event_search", TimeStart: 0.9, TimeEnd: 2, Limit: len(lines) + 1}
	indexed := Run(idx, q)
	streamed, err := StreamEventSearch(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}
	for label, result := range map[string]Result{"indexed": indexed, "streamed": streamed} {
		if len(result.Events) != len(lines) {
			t.Fatalf("%s raw search retained %d/%d EROFS rows", label, len(result.Events), len(lines))
		}
		for _, ev := range result.Events {
			if ev.Type != EventUnknown || strings.TrimSpace(ev.Raw) == "" {
				t.Fatalf("%s raw-only row regained a type or lost raw text: %+v", label, ev)
			}
		}
		if len(result.EvidencePack) != 0 {
			t.Fatalf("%s raw-only EROFS search minted evidence: %+v", label, result.EvidencePack)
		}
	}

	stats := ComputeWindowStats(idx, q)
	if stats.EventCounts[EventUnknown] != len(lines) ||
		stats.EventCounts[EventFilesystem] != 0 || stats.EventCounts[EventStorage] != 0 ||
		stats.FilesystemEventCount != 0 || stats.StorageEventCount != 0 ||
		len(stats.SubsystemEvents) != 0 || len(stats.FilesystemResources) != 0 ||
		len(stats.FileIOByInode) != 0 || stats.TopIOInodes != nil ||
		len(stats.StorageLatencyByLayer) != 0 || len(stats.IOLatencies) != 0 ||
		stats.IOPressureSummary != nil || len(stats.IOBurstEpisodes) != 0 ||
		len(stats.BlockIOByInode) != 0 {
		t.Fatalf("raw-only EROFS rows leaked semantic aggregates: %+v", stats)
	}
	if facts := evidenceFromStats(stats); len(facts) != 0 {
		t.Fatalf("raw-only EROFS rows minted stats evidence: %+v", facts)
	}
	if rank := BuildRootCauseRank(idx, q); len(rank.Items) != 0 || len(rank.AbsorbedItems) != 0 {
		t.Fatalf("raw-only EROFS rows minted root-rank seats: items=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}

	for _, eventType := range []EventType{EventFilesystem, EventStorage, "file_io", "storage_latency", "io_pressure"} {
		semanticQuery := Query{View: "event_search", EventTypes: []EventType{eventType}, TimeStart: 0.9, TimeEnd: 2, Limit: len(lines) + 1}
		indexedSemantic := Run(idx, semanticQuery)
		streamedSemantic, streamErr := StreamEventSearch(context.Background(), path, semanticQuery)
		if streamErr != nil {
			t.Fatal(streamErr)
		}
		if len(indexedSemantic.Events) != 0 || len(streamedSemantic.Events) != 0 ||
			len(indexedSemantic.EvidencePack) != 0 || len(streamedSemantic.EvidencePack) != 0 {
			t.Fatalf("semantic filter %q admitted EROFS: indexed=%+v streamed=%+v", eventType, indexedSemantic, streamedSemantic)
		}
	}

	discovery, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto, WindowDiscoveryRequest{
		Strategy:     WindowDiscoveryPairingIntegrity,
		Families:     []WindowDiscoveryFamily{WindowDiscoveryFamilyStorage},
		TimeStart:    0.9,
		TimeEnd:      2,
		TimeStartSet: true,
		TimeEndSet:   true,
		MaxWindows:   4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if discovery.EndpointCount != 0 || discovery.ScopedEndpointCount != 0 ||
		len(discovery.Candidates) != 0 || len(discovery.Windows) != 0 {
		t.Fatalf("raw-only EROFS rows entered pairing-window discovery: %+v", discovery)
	}
}

func TestEROFSTextInsideBlockedReasonDoesNotTriggerNameBasedDeny(t *testing.T) {
	raw := "worker-20 (20) [004] .... 1.000000: sched_blocked_reason: pid=562 iowait=1 caller=z_erofs_vle_normalaccess_readpage+0x10/0x20[kernel]"
	ev, ok := ParseLine(1, raw, newStringInterner())
	if !ok || ev.Type != EventSchedBlockedReason || ev.WakeePID != 562 || ev.IOWait != 1 ||
		!strings.Contains(ev.FieldText, "z_erofs_vle_normalaccess_readpage") {
		t.Fatalf("legitimate blocked_reason was damaged by EROFS text in its payload: ok=%t event=%+v", ok, ev)
	}
	if durationOrderRawCandidate(raw) || durationEndpointFallbackCandidate(raw) {
		t.Fatal("EROFS text in a non-EROFS event body entered a raw duration endpoint lane")
	}
	idx := &Index{Events: []Event{ev}, FirstTs: ev.Ts, LastTs: ev.Ts}
	stats := ComputeWindowStats(idx, Query{TimeStart: 0.9, TimeEnd: 1.1, Limit: 10})
	if stats.BlockedReasonCount != 1 || stats.IOWaitBlockedCount != 1 || len(stats.BlockedReasons) != 1 {
		t.Fatalf("legitimate blocked_reason disappeared from its typed inventory: %+v", stats)
	}
	got := EventSearch(idx, Query{EventTypes: []EventType{EventSchedBlockedReason}, TimeStart: 0.9, TimeEnd: 1.1, Limit: 10})
	if len(got) != 1 || got[0].Type != EventSchedBlockedReason {
		t.Fatalf("blocked_reason was hidden by an EROFS payload token: %+v", got)
	}
}

func TestUpstreamEROFSRepresentativesRemainRawOnlyUntilDescriptorAuthorityExists(t *testing.T) {
	var lines []string
	for _, name := range upstreamEROFSSemanticDenyRepresentatives {
		line := len(lines) + 1
		raw := legacyEROFSTestLine(line, 1+float64(line)/1000, name)
		ev, ok := ParseLine(line, raw, newStringInterner())
		if !ok || ev.Type != EventUnknown || ev.SubsystemKind != "" || ev.FileFields != nil || ev.ResourceFields != nil {
			t.Fatalf("unimplemented upstream EROFS event acquired semantic authority: name=%q ok=%t event=%+v", name, ok, ev)
		}
		if durationOrderRawCandidate(raw) || durationEndpointFallbackCandidate(raw) {
			t.Fatalf("unimplemented upstream EROFS event entered duration audit: %q", name)
		}
		lines = append(lines, raw)
	}
	path := filepath.Join(t.TempDir(), "upstream-erofs-raw-only.systrace")
	if err := os.WriteFile(path, []byte(strings.Join(append(lines, ""), "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{View: "event_search", TimeStart: 0.9, TimeEnd: 2, Limit: len(lines) + 1}
	indexed := Run(idx, q)
	streamed, err := StreamEventSearch(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(indexed.Events) != len(lines) || len(streamed.Events) != len(lines) ||
		len(indexed.EvidencePack) != 0 || len(streamed.EvidencePack) != 0 {
		t.Fatalf("upstream EROFS raw-only parity failed: indexed=%+v streamed=%+v", indexed, streamed)
	}
	stats := ComputeWindowStats(idx, q)
	if stats.EventCounts[EventFilesystem] != 0 || stats.EventCounts[EventStorage] != 0 ||
		len(stats.SubsystemEvents) != 0 || len(stats.FilesystemResources) != 0 ||
		len(stats.FileIOByInode) != 0 || stats.TopIOInodes != nil ||
		len(stats.StorageLatencyByLayer) != 0 || stats.IOPressureSummary != nil ||
		len(stats.IOBurstEpisodes) != 0 || len(evidenceFromStats(stats)) != 0 ||
		len(BuildRootCauseRank(idx, q).Items) != 0 {
		t.Fatalf("unimplemented upstream EROFS event leaked semantic output: %+v", stats)
	}
}

func TestHandBuiltTypedEROFSEventCannotBypassSemanticDeny(t *testing.T) {
	ev := Event{
		Line: 1, Ts: 1, CPU: 1, Type: EventFilesystem, Name: "erofs_lookup_start", Comm: "erofs", PID: 42,
		SubsystemKind:  "fs_erofs",
		ResourceFields: &ResourceFields{Path: "/forged", Op: "read", LatencyMs: 7, Bytes: 4096},
		FileFields:     &FileFields{Dev: "8:0", Ino: "0x9", Offset: 0, Len: 4096, RW: "read"},
		FieldText:      "dev=8:0 ino=0x9 pos=0 len=4096 bytes=4096 rw=read",
	}
	if isFileIOEvent(ev) || runtimeResourceKind(ev) != "" || isStorageLatencyEvent(ev) {
		t.Fatalf("hand-built EROFS event bypassed a derived semantic gate: %+v", ev)
	}
	if _, _, endpoint := genericStorageEndpoint(ev); endpoint {
		t.Fatalf("hand-built EROFS event acquired generic storage pairing: %+v", ev)
	}
	if profile, ok := genericStoragePairingProfile(ev.Name); ok || profile.Family != "" {
		t.Fatalf("hand-built EROFS name acquired pairing profile: %+v", profile)
	}
	if verdict := FingerprintPairingEvent(ev); verdict.Recognized || verdict.KeyKnown || verdict.PayloadAdmitted {
		t.Fatalf("hand-built EROFS event acquired a typed fingerprint: %+v", verdict)
	}
	idx := &Index{Events: []Event{ev}, FirstTs: ev.Ts, LastTs: ev.Ts}
	q := Query{TimeStart: 0.9, TimeEnd: 1.1, Limit: 10}
	stats := ComputeWindowStats(idx, q)
	if len(stats.SubsystemEvents) != 0 || len(stats.FilesystemResources) != 0 ||
		len(stats.FileIOByInode) != 0 || stats.TopIOInodes != nil ||
		len(stats.StorageLatencyByLayer) != 0 || stats.IOPressureSummary != nil ||
		len(stats.IOBurstEpisodes) != 0 || len(stats.BlockIOByInode) != 0 {
		t.Fatalf("hand-built EROFS event leaked into semantic aggregates: %+v", stats)
	}
	views := EventSearch(idx, q)
	if len(views) != 1 || len(evidenceFromEvents(views)) != 0 || len(evidenceFromStats(stats)) != 0 {
		t.Fatalf("hand-built EROFS event leaked evidence or disappeared from raw inventory: views=%+v stats=%+v", views, stats)
	}
	if rank := BuildRootCauseRank(idx, q); len(rank.Items) != 0 || len(rank.AbsorbedItems) != 0 {
		t.Fatalf("hand-built EROFS event acquired root-rank seats: items=%+v absorbed=%+v", rank.Items, rank.AbsorbedItems)
	}
}
