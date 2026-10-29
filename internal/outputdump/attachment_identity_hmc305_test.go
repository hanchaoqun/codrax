package outputdump_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/outputdump"
)

// The public receipt constructor represents already completed preparation;
// these renderer tests do not claim converter or native-query acceptance.
func hmc305DisplayReceipt(t *testing.T, source, query, preview, format string, members ...string) *attachment.TraceMaterial {
	t.Helper()
	bindings := map[string]filegeneration.Identity{}
	for _, path := range append([]string{source, query}, members...) {
		id, err := filegeneration.FromPath(path)
		if err != nil {
			t.Fatal(err)
		}
		bindings[path] = id
	}
	m, err := attachment.BindTraceMaterial(source, query, preview, bindings)
	if err != nil {
		t.Fatal(err)
	}
	m, err = m.WithDisplayFormats(context.Background(), format, "text")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestHMC305PreparedOriginalRootMergesAcrossDerivedKinds(t *testing.T) {
	for _, tc := range []struct{ name, sourceName, sourceBody, queryName, queryBody string }{
		{"linux_perf_data", "perf.data", "PERFILE2\x00payload", "query.perftrace", "app-1 [000] 1.000: perf_sample: pid=1 tid=1 period=1\n"},
		{"zip", "archive.trace", "PK\x03\x04payload", "query.tracebundle.json", `{"version":"hitraceconv-v1","artifacts":[{"type":"perftrace","path":"member.perftrace","bytes":5}]}`},
		{"sqlite", "database.trace", "SQLite format 3\x00payload", "query.tracebundle.json", `{"version":"hitraceconv-v1","artifacts":[{"type":"perftrace","path":"member.perftrace","bytes":5}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			source, query := filepath.Join(dir, tc.sourceName), filepath.Join(dir, tc.queryName)
			for path, body := range map[string]string{source: tc.sourceBody, query: tc.queryBody} {
				if err := os.WriteFile(path, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			preview := "# codrax-source: " + query + "\n" + tc.queryBody
			m := hmc305DisplayReceipt(t, source, query, preview, tc.name)
			request := outputdump.RuntimeArtifactsFromRequest(source)
			prepared := outputdump.RuntimeArtifactsFromAttachment("trace", preview, m)
			if len(request) != 1 || len(prepared) == 0 || request[0].Kind == prepared[0].Kind {
				t.Fatalf("fixture did not reach cross-kind roots: request=%+v prepared=%+v", request, prepared)
			}
			for _, groups := range [][][]outputdump.RuntimeArtifact{{request, prepared}, {prepared, request}} {
				rows := outputdump.MergeRuntimeArtifacts(groups...)
				roots := 0
				for _, row := range rows {
					if row.Source != source {
						continue
					}
					roots++
					if row.Kind != prepared[0].Kind || row.Carrier != prepared[0].Carrier || !strings.Contains(row.DisplayDetail("en"), "input channel: trace;") || !strings.Contains(row.DisplayDetail("en"), query) {
						t.Errorf("display lost query/channel or rewrote legacy authority: %+v detail=%s", row, row.DisplayDetail("en"))
					}
				}
				if roots != 1 {
					t.Errorf("same prepared original was counted %d times: %+v", roots, rows)
				}
			}
			child := append([]outputdump.RuntimeArtifact(nil), request...)
			child[0].Carrier = outputdump.RuntimeArtifactCarrierBundleChild
			if rows := outputdump.MergeRuntimeArtifacts(child, prepared); len(rows) != len(child)+len(prepared) {
				t.Errorf("bundle child became an original root: %+v", rows)
			}
			other := filepath.Join(dir, "other.trace")
			if err := os.WriteFile(other, []byte(tc.sourceBody), 0600); err != nil {
				t.Fatal(err)
			}
			otherRequest := outputdump.RuntimeArtifactsFromRequest(other)
			if rows := outputdump.MergeRuntimeArtifacts(otherRequest, prepared); len(rows) != len(otherRequest)+len(prepared) {
				t.Errorf("different original merged by equal bytes: %+v", rows)
			}
			// Same source and query pathname do not identify the same prepared
			// result after the query file has been replaced.
			if err := os.WriteFile(query, []byte(tc.queryBody+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			newMaterial := hmc305DisplayReceipt(t, source, query, preview, tc.name)
			newPrepared := outputdump.RuntimeArtifactsFromAttachment("trace", preview, newMaterial)
			roots := 0
			for _, row := range outputdump.MergeRuntimeArtifacts(prepared, newPrepared) {
				if row.Source == source {
					roots++
				}
			}
			if roots != 2 {
				t.Errorf("same-path query generations collapsed: roots=%d", roots)
			}
			// A replaced original must not join a newer request observation to
			// the older, already captured presentation receipt.
			if err := os.WriteFile(source, []byte(tc.sourceBody+"new generation"), 0600); err != nil {
				t.Fatal(err)
			}
			newRequest := outputdump.RuntimeArtifactsFromRequest(source)
			rows := outputdump.MergeRuntimeArtifacts(newRequest, prepared)
			if len(rows) != len(newRequest)+len(prepared) {
				t.Errorf("different generations merged: %+v", rows)
			}
		})
	}
}

func TestHMC305BundleMemberKnownZeroRequiresPhysicalObservation(t *testing.T) {
	dir := t.TempDir()
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	empty := filepath.Join(dir, "empty.perftrace")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"version":"hitraceconv-v1","artifacts":[{"type":"perftrace","path":%q},{"type":"perftrace","path":"missing.perftrace"},{"type":"perftrace","path":"claimed-zero.perftrace","bytes":0}]}`, filepath.Base(empty))
	if err := os.WriteFile(bundle, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	preview := "# codrax-source: " + bundle + "\n" + body
	m := hmc305DisplayReceipt(t, bundle, bundle, preview, "tracebundle", empty)
	for _, rows := range [][]outputdump.RuntimeArtifact{
		outputdump.RuntimeArtifactsFromAttachment("trace", preview, m),
		outputdump.RuntimeArtifactsFromRequest(bundle),
	} {
		if len(rows) != 4 {
			t.Fatalf("wrong bundle members: %+v", rows)
		}
		for i, want := range []string{"0 B", "unknown", "unknown"} {
			if got := rows[i+1].DisplaySize("en"); got != want {
				t.Errorf("member %s got %q want %q", rows[i+1].Source, got, want)
			}
		}
	}
}
