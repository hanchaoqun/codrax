package hitraceconv

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/tracebundle"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestGzipExistingTraceDBBothEntrypointsBindExactPayload(t *testing.T) {
	rawInput := existingTraceDBFixture(t)
	body, err := os.ReadFile(rawInput)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := PrepareExistingTraceDB(t.Context(), existingTraceDBOptions(t, rawInput))
	if err != nil {
		t.Fatal(err)
	}
	rawText, err := os.ReadFile(raw.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                    string
		prepare, keep, explicit bool
	}{
		{"convert-no-retention", false, false, false},
		{"convert-retention", false, true, false},
		{"convert-explicit-db", false, false, true},
		{"prepare-default-retention", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts, original := gzipRouteCapture(t, body, "../../false-capture.db")
			opts.KeepTraceDB = tc.keep
			opts.TraceStreamerPath = filepath.Join(t.TempDir(), "must-not-be-invoked")
			if tc.explicit {
				opts.TraceDBOutputPath = filepath.Join(filepath.Dir(opts.OutputPath), "retained.payload")
			}
			generation, err := filegeneration.FromPath(opts.InputPath)
			if err != nil {
				t.Fatal(err)
			}
			convert := ConvertFile
			if tc.prepare {
				convert = PrepareFile
			}
			result, err := convert(context.Background(), opts)
			if err != nil {
				t.Fatal(err)
			}
			outer, inner := result.GzipInputProvenance, result.ExistingTraceDBSource
			if outer == nil || inner == nil || outer.DecodedFormat != "sqlite" || outer.SourceBytes != int64(len(original)) || outer.SourceSHA256 != traceArchiveTestSHA(original) || outer.SourceGeneration != generation.CacheToken() || outer.DecodedBytes != int64(len(body)) || outer.DecodedSHA256 != traceArchiveTestSHA(body) || inner.Path != opts.InputPath || inner.Bytes != outer.DecodedBytes || inner.SHA256 != outer.DecodedSHA256 || inner.Generation != outer.DecodedGeneration || result.InputPath != opts.InputPath || result.InputBytes != int64(len(original)) {
				t.Fatalf("outer/decoded source binding mismatch: %+v %+v %+v", result, outer, inner)
			}
			if QueryReadySystracePath(result) != opts.OutputPath || result.TextTransport != nil || result.ArchiveProvenance != nil {
				t.Fatalf("closed DB did not enter existing semantic route: %+v", result)
			}
			idx, err := tracequery.BuildIndex(t.Context(), result.BundlePath)
			if err != nil {
				t.Fatal(err)
			}
			var switches []tracequery.Event
			for _, event := range idx.Events {
				if event.Type == tracequery.EventSchedSwitch {
					switches = append(switches, event)
				}
			}
			if len(switches) != 2 || switches[0].Ts != .001 || switches[1].Ts != .002 {
				t.Fatalf("scheduler projection changed: %+v", switches)
			}
			query := tracequery.Run(idx, tracequery.Query{View: "event_search", Pattern: "sched_switch", TimeStart: .0015, TimeEnd: .0025, Limit: 10})
			if len(query.Events) != 1 || query.Events[0].Ts != .002 || query.TimeStart != .0015 || query.TimeEnd != .0025 {
				t.Fatalf("explicit query scope changed: %+v", query)
			}
			text, err := os.ReadFile(result.OutputPath)
			if err != nil || !bytes.Equal(text, rawText) {
				t.Fatalf("gzip changed semantic export or full-table fidelity bytes: %v", err)
			}
			manifest, err := os.ReadFile(result.BundlePath)
			if err != nil {
				t.Fatal(err)
			}
			var metadata traceBundleMetadata
			if err := json.Unmarshal(manifest, &metadata); err != nil {
				t.Fatal(err)
			}
			if metadata.GzipInputProvenance == nil || *metadata.GzipInputProvenance != *outer || tracebundle.ValidateGzipInputProvenance(outer) != nil {
				t.Fatal("published bundle lost exact gzip receipt")
			}
			keep := tc.prepare || tc.keep || tc.explicit
			if hasArtifact(result.Artifacts, ArtifactTraceDB) != keep {
				t.Fatalf("retention option ignored: %+v", result.Artifacts)
			}
			for _, artifact := range result.Artifacts {
				if artifact.Type == ArtifactTraceDB {
					got, err := os.ReadFile(artifact.Path)
					if err != nil || !bytes.Equal(got, body) || artifact.SHA256 != outer.DecodedSHA256 {
						t.Fatalf("retained DB not exact payload: %+v %v", artifact, err)
					}
					if tc.explicit && artifact.Path != opts.TraceDBOutputPath {
						t.Fatal("explicit DB path ignored")
					}
				}
			}
			if got, err := os.ReadFile(opts.InputPath); err != nil || !bytes.Equal(got, original) {
				t.Fatalf("source changed: %v", err)
			}
			assertGzipRoutePrivateClean(t, opts)
		})
	}
}
