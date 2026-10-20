package traceinput

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func hmc271TextArchive(t *testing.T, members ...string) (Options, string, []byte) {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, name := range members {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		body := "# tracer: nop\n worker-42 (42) [000] .... 1.000000: tracing_mark_write: B|42|early\n" +
			strings.Repeat("# complete member\n", 5000) +
			" worker-42 (42) [000] .... 2.000000: tracing_mark_write: B|42|" + name + "\n"
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "ordinary.data")
	writeTestFile(t, path, buf.Bytes())
	return Options{RuntimeAnchor: filepath.Join(dir, "runtime"), PreviewBytes: 640}, path, buf.Bytes()
}

func TestHMC271ZIPTextSharedPreparationBindsCompleteSelectedMember(t *testing.T) {
	opts, path, original := hmc271TextArchive(t, "session/A.data", "session/B.data")
	c := NewCoordinator(opts)
	if _, err := c.Prepare(t.Context(), path); ClassifySelectionFailure(err) != SelectionFailureRequired {
		t.Fatalf("implicit ambiguity was not recoverable: %v", err)
	}
	if _, err := c.PrepareSelection(t.Context(), attachment.TraceSelection{Path: path, ArchiveMember: "missing.data"}); ClassifySelectionFailure(err) != SelectionFailureInvalid {
		t.Fatalf("wrong member was not recoverable: %v", err)
	}
	var prior *attachment.TraceMaterial
	for _, member := range []string{"session/A.data", "session/B.data"} {
		m := hmc263Select(t, c, path, member)
		if m.ArchiveMember() != member || !m.ExplicitArchiveSelection() || !m.MatchesSelection(path, member) {
			t.Fatalf("wrong member binding: %s", m.ArchiveMember())
		}
		if prior != nil && (m == prior || m.QueryPath() == prior.QueryPath()) {
			t.Fatal("independent members merged")
		}
		prior = m
		query, err := c.Prepare(t.Context(), m.QueryPath())
		if err != nil || query != m || hmc263Select(t, c, path, member) != m {
			t.Fatal("warm source/query changed receipt", err)
		}
		idx, err := tracequery.BuildIndex(t.Context(), m.QueryPath())
		if err != nil || len(idx.Events) != 2 || idx.Events[1].SpanName != member || idx.Events[1].Line < 5000 {
			t.Fatalf("preview replaced complete member: %+v %v", idx, err)
		}
		raw, err := os.ReadFile(filepath.Join(filepath.Dir(m.QueryPath()), "preparation.json"))
		if err != nil {
			t.Fatal(err)
		}
		var receipt preparationReceipt
		if err := json.Unmarshal(raw, &receipt); err != nil || receipt.ArchiveTransport == nil || receipt.Transport != nil || receipt.Conversion != nil ||
			receipt.ArchiveTransport.Archive.Member != member || receipt.ArchiveTransport.SourceGeneration != receipt.SourceGeneration ||
			receipt.ArchiveTransport.Archive.ArchiveSHA256 != receipt.SourceSHA256 {
			t.Fatalf("persistent source/member receipt lost: %+v %v", receipt, err)
		}
	}
	if _, err := c.Prepare(t.Context(), path); ClassifySelectionFailure(err) != SelectionFailureRequired {
		t.Fatal("prior success selected ambiguous source implicitly", err)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, original) {
		t.Fatal("original changed", err)
	}
}

func TestHMC271ZIPTextPreparedGenerationsRemainIndependent(t *testing.T) {
	for _, mutation := range []string{"outer_same_bytes", "derived"} {
		t.Run(mutation, func(t *testing.T) {
			opts, path, original := hmc271TextArchive(t, "A.data", "B.data")
			c := NewCoordinator(opts)
			a := hmc263Select(t, c, path, "A.data")
			if mutation == "derived" {
				writeTestFile(t, a.QueryPath(), []byte("different generation\n"))
				if _, err := c.PrepareSelection(t.Context(), attachment.TraceSelection{Path: path, ArchiveMember: "A.data"}); err == nil {
					t.Fatal("changed query material retained authority")
				}
				_ = hmc263Select(t, c, path, "B.data")
			} else {
				writeTestFile(t, path+".new", original)
				if err := os.Rename(path+".new", path); err != nil {
					t.Fatal(err)
				}
				for _, selected := range []attachment.TraceSelection{{Path: path, ArchiveMember: "A.data"}, {Path: path, ArchiveMember: "B.data"}, {Path: a.QueryPath()}} {
					if _, err := c.PrepareSelection(t.Context(), selected); err == nil {
						t.Fatalf("new original bypassed receipt via %+v", selected)
					}
				}
			}
		})
	}
}

func TestHMC271ZIPTextReceiptRejectsWrongSourceMemberOrPublication(t *testing.T) {
	for _, corruption := range []string{"missing", "profile", "source_generation", "member", "member_sha", "member_bytes", "output", "decoded_generation", "semantic_artifact", "gzip"} {
		t.Run(corruption, func(t *testing.T) {
			opts, path, original := hmc271TextArchive(t, "session/capture.data")
			opts.InputPath = path
			var publication string
			m, err := prepare(context.Background(), opts, func(ctx context.Context, o hitraceconv.Options) (hitraceconv.Result, error) {
				result, err := hitraceconv.PrepareFile(ctx, o)
				if err != nil || result.ArchiveTextTransport == nil {
					t.Fatalf("real preparation failed before corruption: %+v %v", result, err)
				}
				publication = result.OutputPath
				p := result.ArchiveTextTransport
				switch corruption {
				case "missing":
					result.ArchiveTextTransport = nil
				case "profile":
					p.Profile = hitraceconv.GzipTextTransportProfile
				case "source_generation":
					p.SourceGeneration = "wrong"
				case "member":
					p.Archive.Member = "other.data"
				case "member_sha":
					p.Archive.MemberSHA256 = strings.Repeat("a", 64)
				case "member_bytes":
					p.Archive.MemberBytes++
				case "output":
					p.DecodedPath += ".other"
				case "decoded_generation":
					p.DecodedGeneration = "wrong"
				case "semantic_artifact":
					result.Artifacts = []hitraceconv.Artifact{{Path: p.DecodedPath}}
				case "gzip":
					result.TextTransport = &hitraceconv.GzipTextTransportResult{Profile: hitraceconv.GzipTextTransportProfile}
				}
				return result, nil
			})
			if err == nil || m != nil || publication == "" {
				t.Fatalf("forged transport acquired material: %+v %v", m, err)
			}
			if _, err := os.Stat(publication); !os.IsNotExist(err) {
				t.Fatal("rejected publication retained", err)
			}
			if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, original) {
				t.Fatal("rejection changed original", err)
			}
		})
	}
}
