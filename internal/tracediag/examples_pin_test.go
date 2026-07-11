package tracediag

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func loadShippedScript(path string) (*Script, error) {
	switch filepath.Base(path) {
	case "collect_open_gap_witness.yaml":
		return LoadScriptWithOverrides(path, ScriptOverrides{Window: "100.000..100.500", TID: "12345"})
	case "collect_acceptance_snapshot.yaml":
		return LoadScriptWithOverrides(path, ScriptOverrides{Window: "100.000..100.500", TID: "12345"})
	case "collect_io_pairing_witness.yaml":
		return LoadScriptWithOverrides(path, ScriptOverrides{Window: "100.000..100.500"})
	case "collect_berlin_pairing_witness.yaml":
		return LoadScriptWithOverrides(path, ScriptOverrides{Window: "100.000..100.500", TID: "12345"})
	default:
		return LoadScript(path)
	}
}

// The shipped collection scripts must always load through the strict
// loader — a schema drift between loader and examples reddens here before a
// customer ever sees it.
func TestShippedExampleScriptsParse(t *testing.T) {
	dir := filepath.Join("..", "..", "examples", "tracediag")
	paths, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"collect_g12.yaml":                    false,
		"collect_d10.yaml":                    false,
		"collect_acceptance_snapshot.yaml":    false,
		"collect_format_census.yaml":          false,
		"collect_open_gap_witness.yaml":       false,
		"collect_io_pairing_witness.yaml":     false,
		"collect_berlin_pairing_witness.yaml": false,
	}
	for _, path := range paths {
		script, err := loadShippedScript(path)
		if err != nil {
			t.Errorf("shipped script %s must parse: %v", path, err)
			continue
		}
		if len(script.Steps) == 0 {
			t.Errorf("shipped script %s has no steps", path)
		}
		if _, ok := want[filepath.Base(path)]; ok {
			want[filepath.Base(path)] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("shipped script %s missing from examples/tracediag/", name)
		}
	}
}

// The census script must actually carry a format_census step (its whole
// point), and the G12 script must carry the two member steps plus the rank
// cross-check ruled in §28.12.
func TestShippedScriptShapes(t *testing.T) {
	census, err := LoadScript(filepath.Join("..", "..", "examples", "tracediag", "collect_format_census.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	hasCensus := false
	for _, step := range census.Steps {
		if step.View == ViewFormatCensus {
			hasCensus = true
		}
	}
	if !hasCensus {
		t.Error("collect_format_census.yaml must contain a format_census step")
	}

	// G12 (P1-1 修后形): BOTH member lanes cover the SAME full line range
	// 1017021–1625582 with thread filters — a pre-split range would decide
	// the "do the two segments overlap" question by construction. The
	// dstate-entry steps carry the prev_state=D pattern so the dispositive
	// D-entry rows fit inside the cap regardless of position in the range.
	g12, err := LoadScript(filepath.Join("..", "..", "examples", "tracediag", "collect_g12.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(g12.Steps) != 5 {
		t.Fatalf("collect_g12.yaml steps = %d, want 5", len(g12.Steps))
	}
	if g12.Steps[0].Thread != "hmfs_discard-26-562" || g12.Steps[1].Thread != "oney.hmn.berlin-42591" {
		t.Errorf("g12 dstate-entry threads = %q / %q", g12.Steps[0].Thread, g12.Steps[1].Thread)
	}
	for i, step := range g12.Steps[:4] {
		if step.LineStart != 1017021 || step.LineEnd != 1625582 {
			t.Errorf("g12 step %d line range = %d..%d, want the FULL 1017021..1625582 (no pre-split)", i, step.LineStart, step.LineEnd)
		}
	}
	for i, step := range g12.Steps[:2] {
		if step.Pattern != "prev_state=D" {
			t.Errorf("g12 dstate-entry step %d pattern = %q, want prev_state=D", i, step.Pattern)
		}
	}
	if last := g12.Steps[4]; last.View != "root_cause_rank" || last.PID != 42591 {
		t.Errorf("g12 cross-check step = %+v", last)
	}

	// D-10 (P0-1 修后形): thread selector ONLY — the engine is pid-first and
	// a pid would silently redirect collection to the main thread.
	d10, err := LoadScript(filepath.Join("..", "..", "examples", "tracediag", "collect_d10.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	views := map[string]bool{}
	for _, step := range d10.Steps {
		views[step.View] = true
		if step.Thread != "#RxComputationT-16816" {
			t.Errorf("d10 step thread = %q", step.Thread)
		}
		if step.PID != 0 {
			t.Errorf("d10 step carries pid=%d — pid-first engine would ignore the thread selector", step.PID)
		}
	}
	if !views["window_stats"] || !views["thread_timeline"] {
		t.Errorf("d10 views = %v, want window_stats + thread_timeline", views)
	}

	// CAP CPU frequency evidence is a CPU-global lane. The target PID belongs
	// only on target-oriented stats/rank steps; inheriting it into raw frequency
	// searches filters by incidental emitter and manufactured a production zero
	// result despite 926 same-window cpu_frequency rows.
	cap2, err := LoadScript(filepath.Join("..", "..", "examples", "tracediag", "collect_cap2.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cap2.Steps) != 6 {
		t.Fatalf("collect_cap2.yaml steps = %d, want 6", len(cap2.Steps))
	}
	for i, step := range cap2.Steps[:4] {
		if step.PID != 42591 {
			t.Errorf("cap2 target step %d pid=%d, want 42591", i, step.PID)
		}
	}
	for i, step := range cap2.Steps[4:] {
		if step.PID != 0 || step.Thread != "" {
			t.Errorf("cap2 CPU-global step %d must be unscoped, got pid=%d thread=%q", i+4, step.PID, step.Thread)
		}
		if len(step.EventTypes) != 1 || step.EventTypes[0] != string(tracequery.EventCPUFrequency) {
			t.Errorf("cap2 CPU-global step %d event_types=%v, want [cpu_frequency]", i+4, step.EventTypes)
		}
	}

	// Open-gap witness: the four semantic/causal views are target-scoped,
	// while the four raw evidence lanes intentionally stay unscoped so a
	// completion emitted by IRQ/kworker or an upstream trace-mark thread is
	// not filtered out. The exact eight-step shape is the customer handoff
	// contract documented in the script header.
	openGapPath := filepath.Join("..", "..", "examples", "tracediag", "collect_open_gap_witness.yaml")
	if _, err := LoadScript(openGapPath); err == nil || !strings.Contains(err.Error(), "requires --trace-window") {
		t.Fatalf("open-gap template must fail loud when the required parent window is omitted, got %v", err)
	}
	if _, err := LoadScriptWithOverrides(openGapPath, ScriptOverrides{Window: "100..101"}); err == nil || !strings.Contains(err.Error(), "requires --trace-tid") {
		t.Fatalf("open-gap template must fail loud when the required TID is omitted, got %v", err)
	}
	openGap, err := loadShippedScript(openGapPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(openGap.Steps) != 8 {
		t.Fatalf("collect_open_gap_witness.yaml steps = %d, want 8", len(openGap.Steps))
	}
	if openGap.Version != ScriptVersionV2 || len(openGap.Discoveries) != 1 {
		t.Fatalf("open-gap must use one v2 typed discovery, got version=%d discoveries=%d", openGap.Version, len(openGap.Discoveries))
	}
	discovery := openGap.Discoveries[0]
	if discovery.Label != "io_pairing_windows" || discovery.Strategy != string(tracequery.WindowDiscoveryPairingIntegrity) || discovery.MaxWindows != 2 || discovery.MaxWindowMS > 50 {
		t.Fatalf("open-gap pairing discovery = %+v", discovery)
	}
	for i, step := range openGap.Steps[:4] {
		if step.PID != 12345 || step.PIDFrom != "tid" || step.Thread != "" {
			t.Errorf("open-gap target step %d selector = pid:%d pid_from:%q thread:%q, want typed TID binding", i, step.PID, step.PIDFrom, step.Thread)
		}
	}
	for i, step := range openGap.Steps[4:] {
		if step.PID != 0 || step.PIDFrom != "" || step.Thread != "" {
			t.Errorf("open-gap raw step %d must be unscoped, got pid:%d pid_from:%q thread:%q", i+4, step.PID, step.PIDFrom, step.Thread)
		}
	}
	if ref := openGap.Steps[4].WindowsFrom; ref == nil || ref.Discovery != discovery.Label {
		t.Fatalf("open-gap raw IO lane must consume the typed discovery, got %+v", ref)
	}
	if openGap.Steps[4].Window != "" {
		t.Fatalf("dynamic raw IO lane must not inherit the parent window directly: %q", openGap.Steps[4].Window)
	}
	if openGap.v2WorstReportLines != 966 {
		t.Fatalf("open-gap validated worst report lines=%d, want pinned 966", openGap.v2WorstReportLines)
	}
	for i, step := range openGap.Steps[4:] {
		if step.View != "event_search" {
			t.Errorf("open-gap raw view %d = %q, want event_search", i+4, step.View)
		}
	}
	if got := openGap.Steps[5].EventTypes; len(got) != 2 || got[0] != "workqueue" || got[1] != "dma_fence" {
		t.Errorf("open-gap workqueue/DMA lane event_types = %v, want [workqueue dma_fence]", got)
	}
	if got := openGap.Steps[7].EventTypes; len(got) != 1 || got[0] != "unknown" {
		t.Errorf("open-gap unknown-print lane event_types = %v, want [unknown]", got)
	}

	ioPairingPath := filepath.Join("..", "..", "examples", "tracediag", "collect_io_pairing_witness.yaml")
	if _, err := LoadScript(ioPairingPath); err == nil || !strings.Contains(err.Error(), "requires --trace-window") {
		t.Fatalf("dedicated IO template must fail loud when the required parent window is omitted, got %v", err)
	}
	ioPairing, err := loadShippedScript(ioPairingPath)
	if err != nil {
		t.Fatal(err)
	}
	if ioPairing.Version != ScriptVersionV2 || len(ioPairing.Discoveries) != 1 || len(ioPairing.Steps) != 1 {
		t.Fatalf("dedicated IO pairing shape = version:%d discoveries:%d steps:%d", ioPairing.Version, len(ioPairing.Discoveries), len(ioPairing.Steps))
	}
	if ioPairing.Discoveries[0].MaxWindows != tracequery.HardWindowDiscoveryMaxWindows || ioPairing.v2WorstReportLines > 1000 {
		t.Fatalf("dedicated IO pairing budget = discovery:%+v worst:%d", ioPairing.Discoveries[0], ioPairing.v2WorstReportLines)
	}
	if _, err := LoadScriptWithOverrides(ioPairingPath, ScriptOverrides{Window: "100..101", TID: "7"}); err == nil || !strings.Contains(err.Error(), "does not declare inputs.tid") {
		t.Fatalf("IO-only template must not silently ignore --trace-tid, got %v", err)
	}
}

// P3-6 budget pin (主会话裁定: "单结果≤1k 行" = 单输出文件≤1000 行): v1
// scripts retain the ≤950 body-cap rule; v2 scripts use their stricter static
// worst-case planner, which includes discovery, fan-out, headers and summary.
func TestShippedScriptsSingleFileBudget(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "examples", "tracediag", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no shipped scripts found")
	}
	for _, path := range paths {
		script, err := loadShippedScript(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if script.Version == ScriptVersionV2 {
			if script.v2WorstReportLines > 1000 {
				t.Errorf("%s: v2 validated worst report is %d lines, budget is <=1000", filepath.Base(path), script.v2WorstReportLines)
			}
			continue
		}
		sum := 0
		for i := range script.Steps {
			sum += script.Steps[i].EffectiveMaxLines()
		}
		if sum > 950 {
			t.Errorf("%s: step caps sum to %d, budget is \u2264950 (P3-6 \u5355\u6587\u4ef6\u22641000 \u884c)", filepath.Base(path), sum)
		}
	}
}
