package types

import "testing"

func TestHMC227EffectiveComparisonWindows(t *testing.T) {
	start, end, otherStart, otherEnd := 1.0, 2.0, 4.0, 5.0
	base := &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1–2 seconds",
		Comparison: &RuntimeArtifactComparisonScope{Baseline: RuntimeArtifactComparisonSide{Path: "/a", RuntimeArtifactTimeWindow: RuntimeArtifactTimeWindow{SourceQuote: "/a"}}, Current: RuntimeArtifactComparisonSide{Path: "/b", RuntimeArtifactTimeWindow: RuntimeArtifactTimeWindow{SourceQuote: "/b"}}}}
	for _, mode := range []string{"shared", "unbounded", "one_side", "independent", "mixed", "half", "pooled", "wrong_scope"} {
		t.Run(mode, func(t *testing.T) {
			p := CloneRuntimeArtifactScopeProfile(base)
			switch mode {
			case "unbounded", "one_side", "independent":
				p.TimeStart, p.TimeEnd = nil, nil
				if mode == "unbounded" {
					p.RequestedScope = RuntimeArtifactScopeUnspecified
				} else {
					p.Comparison.Baseline.RuntimeArtifactTimeWindow = RuntimeArtifactTimeWindow{TimeStart: &start, TimeEnd: &end, SourceQuote: "a 1–2"}
					if mode == "independent" {
						p.Comparison.Current.RuntimeArtifactTimeWindow = RuntimeArtifactTimeWindow{TimeStart: &otherStart, TimeEnd: &otherEnd, SourceQuote: "b 4–5"}
					}
				}
			case "mixed":
				p.Comparison.Baseline.RuntimeArtifactTimeWindow = RuntimeArtifactTimeWindow{TimeStart: &start, TimeEnd: &end, SourceQuote: "a 1–2"}
			case "half":
				p.TimeEnd = nil
			case "pooled":
				p.TimeWindows = []RuntimeArtifactTimeWindow{}
			case "wrong_scope":
				p.RequestedScope = RuntimeArtifactScopeFullArtifact
			}
			valid := mode == "shared" || mode == "unbounded" || mode == "one_side" || mode == "independent"
			for i, side := range p.Comparison.Sides() {
				window, ok := p.EffectiveComparisonWindow(i)
				projected, visible := RuntimeArtifactDisplayScopeForOriginal(p, side.Path, "")
				if ok != valid || visible != valid {
					t.Fatalf("shape validity disagrees: window=%v display=%v want=%v", ok, visible, valid)
				}
				if !valid {
					if p.ComparisonWindowAllows(side.Path, "", 1, 2) {
						t.Fatal("invalid mixed binding acquired a window")
					}
					continue
				}
				bounded := mode == "shared" || mode == "independent" || mode == "one_side" && i == 0
				if window.Valid() != bounded || projected.HasExplicitTimeWindows() != bounded {
					t.Fatalf("peer window was borrowed or own window lost: %+v", window)
				}
				if bounded {
					if !p.ComparisonWindowAllows(side.Path, "", *window.TimeStart, *window.TimeEnd) || p.ComparisonWindowAllows(side.Path, "", 0, 9) {
						t.Fatal("window acceptance disagrees with effective display")
					}
					*window.TimeStart = 99
					next, _ := p.EffectiveComparisonWindow(i)
					if *next.TimeStart == 99 {
						t.Fatal("accessor exposed mutable request endpoints")
					}
				}
			}
			if _, ok := RuntimeArtifactDisplayScopeForOriginal(p, "/foreign", ""); ok || p.ComparisonWindowAllows("/foreign", "", 1, 2) {
				t.Fatal("same numeric window authorized a foreign source")
			}
		})
	}
}
