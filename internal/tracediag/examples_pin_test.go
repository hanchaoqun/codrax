package tracediag

import (
	"path/filepath"
	"testing"
)

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
		"collect_g12.yaml":                 false,
		"collect_d10.yaml":                 false,
		"collect_acceptance_snapshot.yaml": false,
		"collect_format_census.yaml":       false,
	}
	for _, path := range paths {
		script, err := LoadScript(path)
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
}

// P3-6 budget pin (主会话裁定: "单结果≤1k 行" = 单输出文件≤1000 行): every
// shipped script's per-step caps sum to ≤950, leaving header/summary head
// room inside the 1000-line file budget.
func TestShippedScriptsSingleFileBudget(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "examples", "tracediag", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no shipped scripts found")
	}
	for _, path := range paths {
		script, err := LoadScript(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
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
