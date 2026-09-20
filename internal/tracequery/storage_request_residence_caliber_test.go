package tracequery

import (
	"encoding/json"
	"strings"
	"testing"
)

// Decode the public wire independently so the pre-change implementation gives
// a behavioral RED, not a compile error for a field that does not exist yet.
func storageRequestResidenceCaliberWire(t *testing.T, group StorageLatencySummary) (string, bool) {
	t.Helper()
	data, err := json.Marshal(group)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	raw, present := fields["request_residence_caliber"]
	if !present {
		return "", false
	}
	var caliber string
	if err := json.Unmarshal(raw, &caliber); err != nil {
		t.Fatal(err)
	}
	return caliber, true
}

func TestStorageRequestResidenceCaliberPublicEndpointFamilies(t *testing.T) {
	for _, tc := range []struct {
		name, family, start, done, caliber string
	}{
		{"rq", "block_rq", "block_rq_issue: 8,0 R 4096 () 123 + 8 [io]", "block_rq_complete: 8,0 R () 123 + 8 [0]", BlockIOWaitCaliberIssueToComplete},
		{"bio", "block_bio", "block_bio_queue: 8,0 R 123 + 8 [io]", "block_bio_complete: 8,0 R 123 + 8 [0]", BlockIOWaitCaliberBIOQueueToComplete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, fixture := range []struct {
				name, body                                         string
				count, paired, open, orphan, ambiguous, suppressed int
				zero                                               bool
			}{
				{"paired", "io-40 (40) [003] .... 1.000000: " + tc.start + "\nirq-2 (2) [003] .... 1.003000: " + tc.done + "\n", 1, 1, 0, 0, 0, 0, false},
				{"measured zero", "io-40 (40) [003] .... 0.000000: " + tc.start + "\nirq-2 (2) [003] .... 0.000000: " + tc.done + "\n", 1, 1, 0, 0, 0, 0, true},
				{"missing completion", "io-40 (40) [003] .... 1.000000: " + tc.start + "\n", 1, 0, 1, 0, 0, 0, false},
				{"orphan completion", "irq-2 (2) [003] .... 1.003000: " + tc.done + "\n", 1, 0, 0, 1, 0, 0, false},
				{"one ambiguous cohort two requests", "io-40 (40) [003] .... 1.000000: " + tc.start + "\nio-41 (41) [003] .... 1.001000: " + tc.start + "\nirq-2 (2) [003] .... 1.002000: " + tc.done + "\nirq-2 (2) [003] .... 1.003000: " + tc.done + "\n", 2, 0, 0, 0, 1, 2, false},
			} {
				t.Run(fixture.name, func(t *testing.T) {
					idx := buildTraceIndex(t, "storage-caliber.systrace", fixture.body)
					result := Run(idx, Query{View: "window_stats"})
					if result.WindowStats == nil || len(result.WindowStats.StorageLatencyByLayer) != 1 {
						t.Fatalf("expected one public endpoint group: %+v", result.WindowStats)
					}
					group := result.WindowStats.StorageLatencyByLayer[0]
					if got, present := storageRequestResidenceCaliberWire(t, group); !present || got != tc.caliber {
						t.Errorf("public %s group lacks engine endpoint caliber: got=%q present=%v want=%q", tc.family, got, present, tc.caliber)
					}
					if group.Event != tc.family || group.Count != fixture.count || group.PairedCount != fixture.paired || group.UnpairedStartCount != fixture.open || group.UnpairedDoneCount != fixture.orphan || group.AmbiguousCohortCount != fixture.ambiguous || group.PairingSuppressedCount != fixture.suppressed {
						t.Errorf("endpoint accounting changed: %+v", group)
					}
					if fixture.paired == 0 {
						if group.RequestLatencyDistribution != nil || group.MaxLatencyMs != 0 || group.AvgLatencyMs != 0 {
							t.Errorf("caliber minted an unmeasured duration: %+v", group)
						}
					} else if fixture.zero {
						assertIORequestDistribution(t, &group, 1, 0, 0, 0, 0, 0, 0, 0)
					} else {
						assertIORequestDistribution(t, &group, 1, 3, 3, 3, 3, 3, 3, 3)
					}
				})
			}
		})
	}
}

func TestStorageRequestResidenceCaliberPublicUnknownAndGenericStayAbsent(t *testing.T) {
	body := strings.Join([]string{
		"io-40 (40) [003] .... 1.000000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096",
		"io-40 (40) [003] .... 1.003000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096",
		"io-40 (40) [003] .... 2.000000: block_rq_insert: 8,0 R 4096 () 123 + 8 [io]",
		"io-40 (40) [003] .... 2.001000: block_future_queue: 8,0 R 123 + 8 [io]",
		"irq-2 (2) [003] .... 2.003000: block_future_complete: 8,0 R 123 + 8 [0]",
	}, "\n") + "\n"
	result := Run(buildTraceIndex(t, "storage-generic.systrace", body), Query{View: "window_stats"})
	if result.WindowStats == nil || len(result.WindowStats.StorageLatencyByLayer) != 1 {
		t.Fatalf("unknown endpoints gained a group: %+v", result.WindowStats)
	}
	group := result.WindowStats.StorageLatencyByLayer[0]
	if group.Layer != "scsi" || group.Event != "scsi_dispatch_cmd" || group.Count != 2 || group.PairedCount != 1 {
		t.Fatalf("generic endpoint accounting changed: %+v", group)
	}
	if got, present := storageRequestResidenceCaliberWire(t, group); present {
		t.Errorf("generic group borrowed a block endpoint ruler: %q", got)
	}
	assertIORequestDistribution(t, &group, 1, 3, 3, 3, 3, 3, 3, 3)
}

func TestStorageRequestResidenceCaliberPublicWindowDoesNotChangeFullLifetime(t *testing.T) {
	body := "io-40 (40) [003] .... 1.000000: block_bio_queue: 8,0 R 123 + 8 [io]\nirq-2 (2) [003] .... 3.000000: block_bio_complete: 8,0 R 123 + 8 [0]\n"
	result := Run(buildTraceIndex(t, "storage-caliber-window.systrace", body), Query{View: "window_stats", TimeStart: 2, TimeEnd: 2, TimeStartSet: true, TimeEndSet: true})
	if result.WindowStats == nil || len(result.WindowStats.StorageLatencyByLayer) != 1 {
		t.Fatalf("carry-through pair missing: %+v", result.WindowStats)
	}
	group := result.WindowStats.StorageLatencyByLayer[0]
	if got, present := storageRequestResidenceCaliberWire(t, group); !present || got != BlockIOWaitCaliberBIOQueueToComplete {
		t.Errorf("carry-through group lacks BIO endpoint caliber: %q present=%v", got, present)
	}
	assertIORequestDistribution(t, &group, 1, 2000, 2000, 2000, 2000, 2000, 2000, 2000)
}
