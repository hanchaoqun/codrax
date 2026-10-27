package types

import (
	"reflect"
	"testing"
)

func TestHMC300NativeMeasurementDomainFallbackIdentity(t *testing.T) {
	for _, lane := range []string{"dedupe", "same_fact", "duplicate_publication"} {
		t.Run(lane, func(t *testing.T) {
			for _, state := range []string{"s_sleep", "running", "runnable", "d_state", "io_wait"} {
				t.Run(state, func(t *testing.T) {
					for _, variation := range []string{"window", "target", "method", "source", "clock", "same_domain", "unknown"} {
						t.Run(variation, func(t *testing.T) {
							a, b := b1638b3RestrictionNode("a", false), b1638b3RestrictionNode("b", false)
							a.SupportRefs, b.SupportRefs = []string{"/capture/original.trace:100-110"}, []string{"/capture/original.trace:100-110"}
							a.StateKind, a.Object, b.StateKind, b.Object = state, state, state, state
							switch variation {
							case "window":
								b.MeasurementOrigins[0].MeasurementSources.Domains[0].WindowEndTs = 3
							case "target":
								b.MeasurementOrigins[0].MeasurementSources.Domains[0].TargetTID++
							case "method":
								b.MeasurementOrigins[0].MeasurementSources.Domains[0].Method = "off_cpu_sweep"
							case "source":
								b.MeasurementOrigins[0].SourceRef.Path = "/capture/other.trace"
							case "clock":
								a.MeasurementOrigins[0].SourceRef.TimeDomain, b.MeasurementOrigins[0].SourceRef.TimeDomain = "trace_seconds", "wall_seconds"
							case "unknown":
								b.MeasurementOrigins = nil
							}
							want := 2
							if variation == "same_domain" || variation == "unknown" {
								want = 1
							}
							for _, reverse := range []bool{false, true} {
								rows := []TraceCausalProjectionNode{a, b}
								if reverse {
									rows[0], rows[1] = rows[1], rows[0]
								}
								before := append([]TraceCausalProjectionNode(nil), rows...)
								switch lane {
								case "dedupe":
									rows = traceCausalProjectionDedupeNodes(rows)
								case "same_fact":
									p := TraceCausalProjection{AdjacentCauses: rows}
									traceCausalProjectionMergeSameFacts(&p)
									rows = p.AdjacentCauses
								case "duplicate_publication":
									rows = traceCausalProjectionDedupDuplicatePublications(rows)
								}
								if len(rows) != want {
									t.Fatalf("reverse=%t: native domain identity got %d rows, want %d", reverse, len(rows), want)
								}
								if want == 2 && !reflect.DeepEqual(rows, before) {
									t.Fatal("conflict veto mutated preserved native rows")
								}
								if want == 1 && (rows[0].ImpactMS != 5 || rows[0].MergedCount != 0) {
									t.Fatal("legal publication convergence changed value/SUM semantics")
								}
							}
						})
					}
				})
			}
		})
	}
}

func TestHMC300UnknownOriginCannotBridgeMeasurementWindows(t *testing.T) {
	a, b := b1638b3RestrictionNode("a", false), b1638b3RestrictionNode("b", false)
	b.MeasurementOrigins[0].MeasurementSources.Domains[0].WindowEndTs = 3
	unknown := b1638b3RestrictionNode("unknown", false)
	unknown.MeasurementOrigins = nil
	for _, n := range []*TraceCausalProjectionNode{&a, &b, &unknown} {
		n.SupportRefs = []string{"/capture/original.trace:100-110"}
	}
	for _, order := range [][]TraceCausalProjectionNode{{a, unknown, b}, {unknown, a, b}, {b, unknown, a}} {
		for _, lane := range []string{"dedupe", "same_fact", "duplicate_publication"} {
			rows := append([]TraceCausalProjectionNode(nil), order...)
			switch lane {
			case "dedupe":
				rows = traceCausalProjectionDedupeNodes(rows)
			case "same_fact":
				p := TraceCausalProjection{AdjacentCauses: rows}
				traceCausalProjectionMergeSameFacts(&p)
				rows = p.AdjacentCauses
			case "duplicate_publication":
				rows = traceCausalProjectionDedupDuplicatePublications(rows)
			}
			if len(rows) != 2 {
				t.Fatalf("%s unknown bridged conflicting native windows: %d", lane, len(rows))
			}
		}
	}
}

func TestHMC300OverlappingNativeDomainsKeepInstances(t *testing.T) {
	for _, mode := range []string{"overlap", "disjoint", "unknown", "same_domain"} {
		rows := []TraceCausalProjectionNode{b1638b3RestrictionNode("a", false), b1638b3RestrictionNode("b", false), b1638b3RestrictionNode("c", false)}
		for i := range rows {
			rows[i].StartTs, rows[i].EndTs = float64(i+1), float64(i+2)
			rows[i].ImpactMS, rows[i].CumulativeImpactMS = float64(i+5), float64(i+5)
			rows[i].MeasurementOrigins[0].MeasurementSources.Domains[0].WindowStartTs = rows[i].StartTs
			rows[i].MeasurementOrigins[0].MeasurementSources.Domains[0].WindowEndTs = rows[i].EndTs
		}
		if mode != "disjoint" {
			rows[0].EndTs = 3
			rows[0].MeasurementOrigins[0].MeasurementSources.Domains[0].WindowEndTs = 3
		}
		if mode == "unknown" {
			for i := range rows {
				rows[i].MeasurementOrigins = nil
			}
		}
		if mode == "same_domain" {
			for i := range rows {
				rows[i].MeasurementOrigins[0].MeasurementSources.Domains[0].WindowStartTs = 1
				rows[i].MeasurementOrigins[0].MeasurementSources.Domains[0].WindowEndTs = 4
			}
		}
		out := traceCausalProjectionAggregateSameKind(rows)
		if mode == "overlap" && len(out) != 3 {
			t.Fatalf("overlapping native domains lost instances: %+v", out)
		}
		if mode != "overlap" && (len(out) != 1 || out[0].MergedCount != 3) {
			t.Fatalf("%s lost existing family aggregation", mode)
		}
	}
}

func TestHMC300ExactStateCoverKeepsIndependentAuthority(t *testing.T) {
	rows := []TraceCausalProjectionNode{stateCoverTestNode("whole", 1, 2, 100), stateCoverTestNode("a", 1.1, 1.2, 10), stateCoverTestNode("b", 1.4, 1.5, 15)}
	if !TraceCausalProjectionMeasurementDomainsConflict(rows[0].MeasurementOrigins, rows[1].MeasurementOrigins) {
		t.Fatal("fixture must contain different native measurement windows")
	}
	if TraceCausalProjectionOccurrenceDomainsConflict(rows) {
		t.Fatal("negative fallback guard overrode independent complete-state-cover authority")
	}
	merged := traceCausalProjectionAggregateSameKind(rows)
	if len(merged) != 1 || merged[0].ImpactMS != 100 || !merged[0].MergedIntervalUnion {
		t.Fatal("complete cover lost its existing non-additive merge")
	}
}
