package tracequery

import "testing"

func TestMatchFrameCatalogUsesIncomingGapForRepeatedWork(t *testing.T) {
	catalog := func(times []uint64) FrameCatalog {
		var out FrameCatalog
		for i, ms := range times {
			out.Frames = append(out.Frames, FrameCatalogEntry{
				ID: string(rune('a' + i)), Vsync: uint64(i + 10), TID: 7, StartLine: i + 1,
				StartNS: ms * 1e6, EndNS: ms*1e6 + 1e6,
				Points: []string{"draw-VRI[Main]", "traversal"},
			})
		}
		return out
	}
	target := catalog([]uint64{100, 110, 200, 210, 220})
	baseline := catalog([]uint64{1000, 1010, 1100, 1110, 1120})
	got, err := MatchFrameCatalog(target, baseline, 12, 5)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "reference_only" || got.Considered != 5 || got.Candidates[0].Frame.StartNS != 1100*1e6 ||
		got.Candidates[0].Evidence["incoming_gap"] != 1 {
		t.Fatalf("unexpected selection: %+v", got)
	}
	target.Frames[2].VideoOutputPath = "surface_texture"
	for i := range baseline.Frames {
		baseline.Frames[i].VideoOutputPath = "surface_view"
	}
	got, err = MatchFrameCatalog(target, baseline, 12, 5)
	if err != nil || got.Status != "reference_only" || got.Candidates[0].Evidence["mixed_video_output_path"] != 1 ||
		got.Candidates[0].Evidence["incoming_gap"] != 0 {
		t.Fatalf("mixed output paths retained an abnormal cadence cue: %+v (%v)", got, err)
	}
	target.Frames[3].Vsync = 12
	if _, err := MatchFrameCatalog(target, baseline, 12, 5); err == nil {
		t.Fatal("duplicate source-local vsync must require a narrower target")
	}
}

func TestFrameMatchTrajectoryUsesOrdinalAndPointDuration(t *testing.T) {
	sequence := func(values ...float64) []frameMatchObservation {
		rows := make([]frameMatchObservation, len(values))
		for i, value := range values {
			rows[i].durations = map[string]float64{"draw": 1}
			if value > 0 {
				rows[i].durations["measure"] = value
			}
		}
		return rows
	}
	target := frameMatchTrajectories(sequence(0, 8, 5, 3, 0), 2)
	good := frameMatchTrajectories(sequence(0, 6, 4, 2, 0), 2)
	reversed := frameMatchTrajectories(sequence(0, 2, 4, 6, 0), 2)
	first := frameMatchTrajectories(sequence(0, 6, 4, 2, 0), 1)
	similarity, ok := frameMatchTrajectorySimilarity(target, good)
	wrongOrder, _ := frameMatchTrajectorySimilarity(target, first)
	wrongTrend, _ := frameMatchTrajectorySimilarity(target, reversed)
	if !ok || target["measure"].before != 1 || target["measure"].after != 1 ||
		!target["measure"].hasSlope || similarity <= wrongOrder || similarity <= wrongTrend {
		t.Fatalf("ordinal and trend are not retained: target=%+v right=%v ordinal=%v trend=%v",
			target, similarity, wrongOrder, wrongTrend)
	}
}

func TestMatchFrameCatalogDistinguishesWindowLayoutFromParameterUpdates(t *testing.T) {
	const window = "VRI[Main]-relayoutWindow#"
	const initial = window + "first=true/resize=true/vis=false/params=true/force=false"
	const update = window + "first=false/resize=false/vis=false/params=true/force=false"
	makeCatalog := func(firstVsync uint64, times []uint64, names []string) FrameCatalog {
		var catalog FrameCatalog
		for i, ms := range times {
			line := 10*i + 1
			start := ms * 1_000_000
			catalog.Frames = append(catalog.Frames, FrameCatalogEntry{
				Vsync: firstVsync + uint64(i), TID: 7, StartLine: line,
				StartNS: start, EndNS: start + 3_000_000,
			})
			catalog.PointSpans = append(catalog.PointSpans,
				FrameCatalogPoint{Name: names[i], TID: 7, FrameLine: line, StartNS: start + 1, EndNS: start + 2},
				FrameCatalogPoint{Name: "draw-VRI[Main]", TID: 7, FrameLine: line, StartNS: start + 3, EndNS: start + 4},
			)
		}
		return catalog
	}
	current := makeCatalog(10, []uint64{100, 120, 140}, []string{initial, update, update})
	baseline := makeCatalog(100, []uint64{1000, 1020, 1040, 1400, 1420, 1440}, []string{update, initial, "animation", update, update, initial})
	match, err := MatchFrameCatalog(current, baseline, 11, 5)
	if err != nil {
		t.Fatal(err)
	}
	if match.Candidates[0].Frame.Vsync != 103 || match.Candidates[0].Evidence["window_update_role"] != 1 {
		t.Fatalf("first parameter update did not outrank the initial layout or second update: %+v", match.Candidates[0])
	}
	if updates := frameWindowUpdates(baseline, 7); updates[1].generation != 0 || updates[51].generation != 2 {
		t.Fatalf("window update sequence ignored missing or restarted lifecycles: %+v", updates)
	}
}

func TestFrameMatchEpisodeRequiresConsistentPointShifts(t *testing.T) {
	center := map[string]float64{"first": -100, "second": -70, "third": -30}
	aligned := map[string]float64{"first": -110, "second": -79, "third": -42}
	dispersed := map[string]float64{"first": -155, "second": -70, "third": 15}
	positive, covered := frameMatchEpisodeSimilarity(center, aligned)
	negative, _ := frameMatchEpisodeSimilarity(center, dispersed)
	if !covered || positive <= .5 || negative != 0 {
		t.Fatalf("wrong episode support: aligned=%v, out-of-order=%v", positive, negative)
	}
}

func TestFrameMatchLandmarksRequireOrderedGroupsAndBracketing(t *testing.T) {
	catalog := func(offsets []uint64) FrameCatalog {
		var out FrameCatalog
		for i, when := range offsets {
			for j, prefix := range []string{"open", "measure", "commit"} {
				start := when + uint64(j)*1_000_000
				name := prefix + string(rune('A'+i))
				out.PointSpans = append(out.PointSpans, FrameCatalogPoint{Name: name, TID: 5, StartNS: start, EndNS: start + 1})
			}
		}
		return out
	}
	current := catalog([]uint64{100_000_000, 300_000_000, 500_000_000})
	baseline := catalog([]uint64{1000_000_000, 950_000_000, 1600_000_000})
	chain := frameMatchLandmarks(current, baseline, 5, 5)
	if len(chain) != 2 || chain[0].currentMS != 101 || chain[1].currentMS != 501 {
		t.Fatalf("crossing group retained: %+v", chain)
	}
	candidates := []FrameMatchCandidate{
		{Frame: FrameCatalogEntry{ID: "before", StartNS: 1100_000_000}, Score: .76, Evidence: map[string]float64{}},
		{Frame: FrameCatalogEntry{ID: "inside", StartNS: 1301_000_000}, Score: .74, Evidence: map[string]float64{}},
	}
	rerankByFrameMatchLandmarks(candidates, chain, 301_000_000)
	if candidates[0].Frame.ID != "inside" || candidates[0].Evidence["landmark_residual_ms"] != 0 {
		t.Fatalf("wrong local bracket projection: %+v", candidates)
	}
	if _, _, ok := frameMatchLandmarkProjection(chain, 550); ok {
		t.Fatal("projection extrapolated outside the observed bracket")
	}
}

func TestFrameMatchHostCompositionRevisitsOnlyNearbyComparableFrames(t *testing.T) {
	makeCandidate := func(id string, start uint64, score, hostMS float64, secondary bool) FrameMatchCandidate {
		host := &FrameHostComposition{Cycles: 1, FrameToHostMS: hostMS}
		if secondary {
			host.SecondaryAcquireLine = 1
		}
		return FrameMatchCandidate{
			Frame: FrameCatalogEntry{ID: id, StartNS: start * 1_000_000, HostComposition: host},
			Score: score, Evidence: map[string]float64{"local_points": .5, "before_gaps": .5},
		}
	}
	target := FrameCatalogEntry{HostComposition: &FrameHostComposition{Cycles: 1, FrameToHostMS: 9, SecondaryAcquireLine: 1}}
	first := makeCandidate("first", 1_000, .58, 4, false)
	near := makeCandidate("near", 1_060, .40, 10, true)
	remote := makeCandidate("remote", 1_900, .57, 9, true)
	candidates := []FrameMatchCandidate{first, remote, near}
	rerankByFrameHostComposition(target, candidates)
	if candidates[0].Frame.ID != "near" || candidates[0].Evidence["host_composition_similarity"] == 0 {
		t.Fatalf("host evidence failed to rank a comparable nearby frame: %+v", candidates)
	}
	first.Frame.HostComposition.Cycles = 2
	candidates = []FrameMatchCandidate{first, remote, near}
	rerankByFrameHostComposition(target, candidates)
	if candidates[0].Frame.ID != "first" {
		t.Fatalf("cross-cycle evidence changed the selection: %+v", candidates)
	}
}
