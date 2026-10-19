package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Exercise the public named-input boundary: member spelling is not the
// format, and the normal .data name must not require a caller-side rename.
func TestHMC262ZIPNamedBinaryPublicQuery(t *testing.T) {
	hmc176DisableExternalProviders(t)
	for _, member := range []string{"nested/capture.sys", "nested/capture.data", "业务 采集/原件"} {
		t.Run(member, func(t *testing.T) {
			body := hmc176Zip(t, map[string][]byte{member: hmc176PerfData(true), "README.txt": []byte("capture description")})
			path := hmc176WriteSource(t, body)
			bus, preparer, conversions := hmc17NamedPathContext(t)
			bus.AttachedHitrace = hmc176UnrelatedText
			params := map[string]any{"source": "path", "path": path, "view": "event_search"}
			payload := hmc17NamedPayload(t, hmc17NamedQuery(t, bus, params))
			if payload.EventCount != 1 || len(payload.Events) != 1 || payload.Events[0].Type != tracequery.EventPerfSample {
				t.Fatalf("complete named binary was not queried: %+v", payload)
			}
			material, err := preparer.Prepare(context.Background(), path)
			if err != nil || material.SourcePath() != path || material.QueryPath() != payload.SourcePath || conversions.Load() != 1 {
				t.Fatalf("shared preparation identity/reuse lost: conversions=%d err=%v", conversions.Load(), err)
			}
			bundle := hmc176bReadBundle(t, material.QueryPath())
			if bundle.ArchiveProvenance == nil || bundle.ArchiveProvenance.Member != member || bundle.ArchiveProvenance.Selection != "unique_candidate" || bundle.Systrace != "" {
				t.Fatalf("member identity or sample-only capability changed: %+v", bundle)
			}
			if bus.AttachedHitrace != hmc176UnrelatedText || bus.AttachedTraceMaterial != nil {
				t.Fatal("named query replaced unrelated attachment")
			}
			hmc176AssertOriginal(t, path, body)
		})
	}
}

func TestHMC262ZIPPublicKnownFamiliesAndExactSources(t *testing.T) {
	hmc176DisableExternalProviders(t)
	for _, tc := range []struct {
		name      string
		body      []byte
		eventType tracequery.EventType
		count     int
	}{
		{"perf", hmc176PerfData(true), tracequery.EventPerfSample, 1},
		{"simpleperf", hmc176Simpleperf(), tracequery.EventPerfSample, 1},
		{"rmq", hmc17NamedBinaryTailFixture(), tracequery.EventSchedWakeup, 12},
		{"profiler", hmc176Profiler("bytrace_plugin", []byte(strings.Repeat(hmc176ScheduleText, 30)+hmc176bTailText)), tracequery.EventSchedWakeup, 31},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const member = "业务 采集/正常名称.data"
			body := hmc176Zip(t, map[string][]byte{member: tc.body, "README.txt": []byte("unrelated metadata")})
			path := hmc176WriteSource(t, body)
			bus, preparer, conversions := hmc17NamedPathContext(t)
			bus.AttachedHitrace = hmc176UnrelatedText
			params := map[string]any{"source": "path", "path": path, "view": "event_search", "limit": 100}
			payload := hmc17NamedPayload(t, hmc17NamedQuery(t, bus, params))
			if payload.EventCount != tc.count || len(payload.Events) != tc.count {
				t.Fatalf("lost complete members: count=%d rows=%d", payload.EventCount, len(payload.Events))
			}
			material, err := preparer.Prepare(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range payload.Events {
				if event.Type != tc.eventType || event.SourcePath == path {
					t.Fatalf("wrong event/source authority: %+v", event)
				}
				hmc176bAssertManaged(t, filepath.Dir(material.QueryPath()), event.SourcePath)
			}
			var receipt struct {
				SourcePath       string             `json:"source_path"`
				SourceBytes      int64              `json:"source_bytes"`
				SourceSHA256     string             `json:"source_sha256"`
				SourceGeneration string             `json:"source_generation"`
				QueryPath        string             `json:"query_path"`
				Conversion       hitraceconv.Result `json:"conversion"`
			}
			raw, err := os.ReadFile(filepath.Join(filepath.Dir(material.QueryPath()), "preparation.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(raw, &receipt); err != nil {
				t.Fatal(err)
			}
			outer, inner := sha256.Sum256(body), sha256.Sum256(tc.body)
			p := receipt.Conversion.ArchiveProvenance
			if receipt.SourcePath != path || receipt.SourceBytes != int64(len(body)) || receipt.SourceSHA256 != hex.EncodeToString(outer[:]) || receipt.SourceGeneration == "" || receipt.QueryPath != material.QueryPath() || p == nil || p.Member != member || p.MemberBytes != int64(len(tc.body)) || p.MemberSHA256 != hex.EncodeToString(inner[:]) || p.ArchiveSHA256 != receipt.SourceSHA256 || p.ArchiveBytes != receipt.SourceBytes || p.Selection != "unique_candidate" {
				t.Fatalf("lost original/member identity: %+v", receipt)
			}
			bundle := hmc176bReadBundle(t, material.QueryPath())
			if !reflect.DeepEqual(bundle.ArchiveProvenance, p) || bundle.GzipInputProvenance != nil {
				t.Fatal("archive identity changed between receipt and bundle")
			}
			if tc.eventType == tracequery.EventPerfSample && (bundle.Systrace != "" || len(bundle.Artifacts) != 1 || bundle.Artifacts[0].Type != hitraceconv.ArtifactPerfTrace) {
				t.Fatal("sample-only ZIP gained causal/scheduling or raw-container child")
			}
			warm := hmc17NamedPayload(t, hmc17NamedQuery(t, bus, params))
			if warm.EventCount != tc.count || conversions.Load() != 1 {
				t.Fatal("warm query reconverted/lost exact population")
			}
			if tc.name == "rmq" {
				if strings.Contains(material.Preview(), "tail-target") {
					t.Fatal("tail no longer beyond preview")
				}
				tail := hmc17NamedPayload(t, hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "event_search", "pattern": "tail-target", "time_start": 1.0105, "time_end": 1.012}))
				if len(tail.Events) != 1 || tail.TimeStart != 1.0105 || tail.TimeEnd != 1.012 {
					t.Fatal("explicit window or full tail lost")
				}
			}
			if bus.AttachedHitrace != hmc176UnrelatedText || bus.AttachedTraceMaterial != nil {
				t.Fatal("named query replaced sticky attachment")
			}
			hmc176AssertOriginal(t, path, body)
		})
	}
}

func TestHMC262ZIPPublicRejectionsNeverBorrowOrCache(t *testing.T) {
	hmc176DisableExternalProviders(t)
	for _, tc := range []struct {
		name    string
		members map[string][]byte
		code    string
	}{
		{"ambiguous_normal_names", map[string][]byte{"a.data": hmc176PerfData(true), "b": hmc176Simpleperf()}, "multiple_trace_members"},
		{"legacy_and_new", map[string][]byte{"legacy.sys": hmc17NamedBinaryTailFixture(), "capture.data": hmc176PerfData(true)}, "multiple_trace_members"},
		{"unknown", map[string][]byte{"capture.data": {0, 1, 2, 3}}, "trace_member_not_found"},
		{"inventory", map[string][]byte{"capture.data": hmc176PerfData(false)}, "no_query_ready_material"},
		{"malformed_known_prefix", map[string][]byte{"capture.data": []byte("PERFILE2")}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := hmc176Zip(t, tc.members)
			path := hmc176WriteSource(t, body)
			bus, preparer, conversions := hmc17NamedPathContext(t)
			bus.AttachedHitrace = hmc176UnrelatedText
			for i := 0; i < 2; i++ {
				result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "event_search"})
				if result.Success || len(result.Observations) != 0 || result.RawRef != "" || (tc.code != "" && !strings.Contains(result.Summary, tc.code)) {
					t.Fatalf("rejected input acquired evidence/lost failure: %+v", result)
				}
			}
			if conversions.Load() != 2 || len(preparer.PreparedMaterials()) != 0 || bus.AttachedHitrace != hmc176UnrelatedText || bus.AttachedTraceMaterial != nil {
				t.Fatal("failure cached success, skipped retry or borrowed attachment")
			}
			if paths, err := filepath.Glob(filepath.Join(bus.WorkDir, ".codrax", "trace-input-*")); err != nil || len(paths) != 0 {
				t.Fatalf("failed preparation left publication: %v %v", paths, err)
			}
			hmc176AssertOriginal(t, path, body)
		})
	}
}

func TestHMC262ZIPWarmChangedSourceCannotReuseOldMember(t *testing.T) {
	hmc176DisableExternalProviders(t)
	path := hmc176WriteSource(t, hmc176Zip(t, map[string][]byte{"capture.data": hmc176PerfData(true)}))
	bus, preparer, conversions := hmc17NamedPathContext(t)
	params := map[string]any{"source": "path", "path": path, "view": "event_search"}
	_ = hmc17NamedPayload(t, hmc17NamedQuery(t, bus, params))
	before, err := preparer.Prepare(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	// Replace the owned test namespace, never chmod/truncate the read-only
	// original file that the prepared material still identifies.
	replacement := path + ".replacement"
	if err := os.WriteFile(replacement, hmc176Zip(t, map[string][]byte{"capture.data": hmc176PerfData(false)}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if err := before.Validate(t.Context(), before.Preview()); err == nil {
		t.Fatal("old source generation remained valid")
	}
	result := hmc17NamedQuery(t, bus, params)
	if result.Success || len(result.Observations) != 0 || result.RawRef != "" || conversions.Load() != 1 {
		t.Fatalf("changed ZIP borrowed old sample publication: %+v", result)
	}
}
