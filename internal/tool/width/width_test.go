package width

import "testing"

// TestWidthDefaultsPinned pins every governor default verbatim to the values
// the per-tool constants carried before consolidation (builtin.go
// grepGovernor* block, nativegrep.go, trace_query.go, repomap/tool.go).
// Compaction trigger values are answer-visible — they decide when grep output
// is compacted and when refinement hints fire — so any change here must be a
// deliberate test edit, never a refactor side effect.
func TestWidthDefaultsPinned(t *testing.T) {
	caps := Defaults()
	pins := []struct {
		name string
		got  int64
		want int64
	}{
		{"Grep.LineEntryThreshold", int64(caps.Grep.LineEntryThreshold), 80},
		{"Grep.FileEntryThreshold", int64(caps.Grep.FileEntryThreshold), 120},
		{"Grep.ByteThreshold", int64(caps.Grep.ByteThreshold), 16 * 1024},
		{"Grep.LineProductionCap", int64(caps.Grep.LineProductionCap), 48},
		{"Grep.FileProductionCap", int64(caps.Grep.FileProductionCap), 80},
		{"Grep.AuxiliaryCap", int64(caps.Grep.AuxiliaryCap), 24},
		{"Grep.OtherCap", int64(caps.Grep.OtherCap), 16},
		{"Grep.LineWindowHintMax", int64(caps.Grep.LineWindowHintMax), 4},
		{"Grep.LineWindowHalfSpan", int64(caps.Grep.LineWindowHalfSpan), 20},
		{"Grep.DirScanMaxFileBytes", caps.Grep.DirScanMaxFileBytes, 4 << 20},
		{"PathDiscoveryCandidateFileLimit", int64(caps.PathDiscoveryCandidateFileLimit), 256},
		{"ReadFile.PageWindowDefault", int64(caps.ReadFile.PageWindowDefault), 100},
		{"ReadFile.PageWindowMax", int64(caps.ReadFile.PageWindowMax), 200},
		{"TraceQuery.StreamSearchMinLimit", int64(caps.TraceQuery.StreamSearchMinLimit), 20},
		{"TraceQuery.TypedEvidenceFactCap", int64(caps.TraceQuery.TypedEvidenceFactCap), 64},
		{"TraceQuery.TypedFamilyRowCap", int64(caps.TraceQuery.TypedFamilyRowCap), 32},
		{"TraceQuery.StateDrilldownSummaryCap", int64(caps.TraceQuery.StateDrilldownSummaryCap), 5},
		{"TraceQuery.EventSearchLimitOverride", int64(caps.TraceQuery.EventSearchLimitOverride), 0},
		{"RepoMap.BroadNavigationMaxRows", int64(caps.RepoMap.BroadNavigationMaxRows), 96},
		{"RepoMap.BroadNavigationScanLimit", int64(caps.RepoMap.BroadNavigationScanLimit), 4096},
		{"RepoMap.BroadRootDefaultTopN", int64(caps.RepoMap.BroadRootDefaultTopN), 50},
		{"RepoMap.BroadRootMaxTopN", int64(caps.RepoMap.BroadRootMaxTopN), 100},
		// Explicit named aliases of the grep values (list_files historically
		// reused these silently at its call site).
		{"ListFilesEntryThreshold", int64(caps.ListFilesEntryThreshold()), 120},
		{"ListFilesByteThreshold", int64(caps.ListFilesByteThreshold()), 16 * 1024},
		// Referenced from the tracequery capacity table (E4), not duplicated.
		{"TraceQueryEventSearchDefaultLimit", int64(caps.TraceQueryEventSearchDefaultLimit()), 40},
	}
	for _, pin := range pins {
		if pin.got != pin.want {
			t.Errorf("%s = %d, want %d", pin.name, pin.got, pin.want)
		}
	}
}

func TestWidthSetOverridesAndSanity(t *testing.T) {
	t.Cleanup(Reset)

	notes := Set(Overrides{
		GrepLineEntryThreshold:      100,
		GrepByteThreshold:           32 * 1024,
		GrepDirScanMaxFileBytes:     8 << 20,
		PathDiscoveryCandidateLimit: 128,
		ReadFilePageWindowMax:       300,
		TraceQueryEventSearchLimit:  60,
		RepoMapNavigationMaxRows:    48,
	})
	if len(notes) != 0 {
		t.Fatalf("unexpected sanity notes: %v", notes)
	}
	caps := Current()
	if caps.Grep.LineEntryThreshold != 100 || caps.Grep.ByteThreshold != 32*1024 {
		t.Fatalf("grep overrides not applied: %+v", caps.Grep)
	}
	if caps.Grep.FileEntryThreshold != DefaultGrepFileEntryThreshold {
		t.Fatalf("unset override must keep default, got %d", caps.Grep.FileEntryThreshold)
	}
	if caps.Grep.DirScanMaxFileBytes != 8<<20 {
		t.Fatalf("dirscan override not applied: %d", caps.Grep.DirScanMaxFileBytes)
	}
	if caps.PathDiscoveryCandidateFileLimit != 128 {
		t.Fatalf("path discovery override not applied: %d", caps.PathDiscoveryCandidateFileLimit)
	}
	if caps.ReadFile.PageWindowMax != 300 {
		t.Fatalf("read_file window override not applied: %+v", caps.ReadFile)
	}
	if caps.TraceQueryEventSearchDefaultLimit() != 60 {
		t.Fatalf("trace_query event_search override not applied: %d", caps.TraceQueryEventSearchDefaultLimit())
	}
	if caps.RepoMap.BroadNavigationMaxRows != 48 {
		t.Fatalf("repo_map navigation override not applied: %+v", caps.RepoMap)
	}
	// list_files aliases must track the overridden grep values.
	Reset()
	Set(Overrides{GrepFileEntryThreshold: 150})
	if got := Current().ListFilesEntryThreshold(); got != 150 {
		t.Fatalf("list_files alias must track grep file entry threshold, got %d", got)
	}
}

func TestWidthSetCrossFieldSanityClampsProductionCaps(t *testing.T) {
	t.Cleanup(Reset)

	// Entry thresholds pushed below the default production caps: the caps
	// must clamp down (production cap <= entry threshold) with a note each.
	notes := Set(Overrides{
		GrepLineEntryThreshold: 10,
		GrepFileEntryThreshold: 12,
	})
	caps := Current()
	if caps.Grep.LineProductionCap != 10 || caps.Grep.FileProductionCap != 12 {
		t.Fatalf("production caps not clamped to entry thresholds: %+v", caps.Grep)
	}
	if len(notes) != 2 {
		t.Fatalf("expected 2 sanity notes, got %v", notes)
	}

	Reset()
	// Non-positive override keeps the default entirely.
	Set(Overrides{GrepLineEntryThreshold: -5, TraceQueryEventSearchLimit: 0})
	caps = Current()
	if caps.Grep.LineEntryThreshold != DefaultGrepLineEntryThreshold {
		t.Fatalf("non-positive override must keep default, got %d", caps.Grep.LineEntryThreshold)
	}
	if caps.TraceQueryEventSearchDefaultLimit() != 40 {
		t.Fatalf("zero override must keep engine default, got %d", caps.TraceQueryEventSearchDefaultLimit())
	}

	Reset()
	// read_file window max below the default window must pull the default down.
	notes = Set(Overrides{ReadFilePageWindowMax: 50})
	caps = Current()
	if caps.ReadFile.PageWindowMax != 50 || caps.ReadFile.PageWindowDefault != 50 {
		t.Fatalf("read_file default not clamped to max: %+v", caps.ReadFile)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 sanity note, got %v", notes)
	}
}
