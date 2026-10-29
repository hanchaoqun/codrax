package outputdump_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/outputdump"
	"github.com/hanchaoqun/codrax/internal/traceinput"
)

func TestHMC305AttachmentStringDoesNotClaimOriginalBytes(t *testing.T) {
	for _, kind := range []string{"trace", "log"} {
		t.Run(kind, func(t *testing.T) {
			preview := "# codrax-source: /missing/original.capture\n" + strings.Repeat("x", 462)
			body := outputdump.BuildBody(outputdump.Args{Language: "en", Request: "inspect", Answer: "answer",
				RuntimeArtifacts: outputdump.RuntimeArtifactsFromAttachment(kind, preview)})
			if strings.Contains(body, "| 462 B |") || !strings.Contains(body, "original size unknown") || !strings.Contains(body, "preview 462 B") {
				t.Fatalf("unbound preview was presented as original size:\n%s", body)
			}
		})
	}
}

func TestHMC305PreparedAttachmentSeparatesOriginalQueryAndPreview(t *testing.T) {
	text := []byte(strings.Repeat("worker-42 ( 42) [000] d..2 9.000001: sched_wakeup: comm=tail pid=99 prio=120 target_cpu=000\n", 100))
	var compressed bytes.Buffer
	w := gzip.NewWriter(&compressed)
	if _, err := w.Write(text); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	// One valid large packet reproduces the original-vs-preview scale without
	// requiring an external repository, model, or packet query.
	pcap := make([]byte, 3499182)
	binary.LittleEndian.PutUint32(pcap[0:], 0xa1b2c3d4)
	binary.LittleEndian.PutUint16(pcap[4:], 2)
	binary.LittleEndian.PutUint16(pcap[6:], 4)
	binary.LittleEndian.PutUint32(pcap[16:], 4000000)
	binary.LittleEndian.PutUint32(pcap[20:], 147)
	binary.LittleEndian.PutUint32(pcap[24:], 1)
	binary.LittleEndian.PutUint32(pcap[32:], uint32(len(pcap)-40))
	binary.LittleEndian.PutUint32(pcap[36:], uint32(len(pcap)-40))
	for _, tc := range []struct {
		name, format, queryFormat string
		source                    []byte
		derived                   bool
	}{
		{"packet", "pcap", "pcap", pcap, false},
		{"text", "text", "text", text, false},
		{"compressed", "gzip", "text", compressed.Bytes(), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "normal.capture")
			if err := os.WriteFile(path, tc.source, 0600); err != nil {
				t.Fatal(err)
			}
			m, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: path, RuntimeAnchor: filepath.Join(dir, "runtime"), PreviewBytes: 640})
			if err != nil {
				t.Fatal(err)
			}
			queryPath := m.QueryPath()
			metadata, err := m.DisplayMetadata(context.Background(), m.Preview())
			if err != nil {
				t.Fatal(err)
			}
			queryInfo, err := os.Stat(queryPath)
			if err != nil {
				t.Fatal(err)
			}
			if metadata.SourcePath != path || metadata.QueryPath != queryPath || metadata.SourceBytes != int64(len(tc.source)) || metadata.QueryBytes != queryInfo.Size() || metadata.PreviewBytes != int64(len(m.Preview())) || metadata.SourceFormat != tc.format || metadata.QueryFormat != tc.queryFormat {
				t.Fatalf("material layers lost their provenance: %+v", metadata)
			}
			if (queryPath != path) != tc.derived {
				t.Fatalf("unexpected source/query identity: %+v", metadata)
			}
			artifacts := outputdump.RuntimeArtifactsFromAttachment("trace", m.Preview(), m)
			if len(artifacts) != 1 || artifacts[0].Source != path {
				t.Fatalf("wrong original identity: %+v", artifacts)
			}
			// A hand-built same-path card without a physical observation cannot
			// claim the current receipt's identity; real request observations are
			// exercised by the public cross-kind merge tests.
			sameSource := outputdump.MergeRuntimeArtifacts([]outputdump.RuntimeArtifact{{Kind: "trace", Source: path, Bytes: 462}}, artifacts)
			if len(sameSource) != 2 || strings.Contains(sameSource[0].DisplayDetail("en"), "preview envelope:") {
				t.Fatalf("unobserved same-path card borrowed prepared receipt: %+v", sameSource)
			}
			otherSource := outputdump.MergeRuntimeArtifacts([]outputdump.RuntimeArtifact{{Kind: "trace", Source: path + ".other", Bytes: 462}}, artifacts)
			if len(otherSource) != 2 || strings.Contains(otherSource[0].DisplayDetail("en"), "preview envelope:") {
				t.Fatalf("different source borrowed prepared receipt: %+v", otherSource)
			}
			merged := outputdump.MergeRuntimeArtifacts(outputdump.RuntimeArtifactsFromRequest(path), artifacts)
			args := outputdump.Args{Language: "en", Request: "inspect", Answer: "answer", RuntimeArtifacts: merged, Dir: filepath.Join(dir, "report")}
			body := outputdump.BuildBody(args)
			for _, want := range []string{fmt.Sprintf("| %d B |", len(tc.source)), "preview envelope: " + fmt.Sprint(len(m.Preview())) + " B", "query material:", "original format:"} {
				if !strings.Contains(body, want) {
					t.Fatalf("missing %q:\n%s", want, body)
				}
			}
			if tc.derived && (!strings.Contains(body, queryPath) || !strings.Contains(body, fmt.Sprintf("(%d B, text)", queryInfo.Size()))) {
				t.Fatalf("derived material lost: %s", body)
			}
			if tc.name == "packet" && !strings.Contains(body, "| PCAP |") {
				t.Fatalf("input channel replaced physical format: %s", body)
			}
			result := outputdump.WriteResult(args)
			html, err := os.ReadFile(result.HTMLPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(html, []byte(fmt.Sprintf("%d B", len(tc.source)))) || !bytes.Contains(html, []byte("preview envelope")) {
				t.Fatal("HTML did not reuse separated source metadata")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(after, tc.source) || m.QueryPath() != queryPath {
				t.Fatal("display mutated original/query identity")
			}
		})
	}
}

func TestHMC305StaleOrMismatchedReceiptCannotRenewFileMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plain.systrace")
	source := "worker-42 ( 42) [000] d..2 9.000001: sched_wakeup: comm=tail pid=99 prio=120 target_cpu=000\n"
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: path, PreviewBytes: 640})
	if err != nil {
		t.Fatal(err)
	}
	for _, preview := range []string{m.Preview() + "changed", m.Preview()} {
		if preview == m.Preview() {
			if err := os.WriteFile(path, []byte(source+source), 0600); err != nil {
				t.Fatal(err)
			}
		}
		rows := outputdump.RuntimeArtifactsFromAttachment("trace", preview, m)
		body := outputdump.BuildBody(outputdump.Args{Language: "en", RuntimeArtifacts: rows})
		if !strings.Contains(body, "original size unknown; preview") || strings.Contains(body, "preview envelope") {
			t.Fatalf("invalid receipt was rebound to a later stat: %s", body)
		}
	}
}

func TestHMC305UnknownFileSizeIsNotKnownZero(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.systrace")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, size string }{{empty, "0 B"}, {filepath.Join(dir, "missing.systrace"), "unknown"}} {
		rows := outputdump.RuntimeArtifactsFromRequest(tc.path)
		if len(rows) != 1 || rows[0].DisplaySize("en") != tc.size {
			t.Fatalf("unknown/known zero collapsed: %+v", rows)
		}
	}
}
