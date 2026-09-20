package traceinput

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

func TestGzipPreparationRequiresExactlyOneTransportReceipt(t *testing.T) {
	for _, family := range []string{"text", "binary"} {
		for _, corruption := range []string{"missing", "mixed"} {
			t.Run(family+"/"+corruption, func(t *testing.T) {
				dir := t.TempDir()
				sourceDir := filepath.Join(dir, "original")
				anchor := filepath.Join(dir, "runtime")
				for _, path := range []string{sourceDir, anchor} {
					if err := os.Mkdir(path, 0o700); err != nil {
						t.Fatal(err)
					}
				}
				input := filepath.Join(sourceDir, "capture.arbitrary")
				decoded := gzipPreparationBody()
				if family == "binary" {
					decoded = simpleperfFixture()
				}
				original := gzipPreparationBytes(t, decoded)
				writeTestFile(t, input, original)
				prior := filepath.Join(anchor, "prior-success.systrace")
				priorBody := []byte("previous publication must survive\n")
				writeTestFile(t, prior, priorBody)
				t.Setenv("CODRAX_TRACE_STREAMER", filepath.Join(dir, "no-trace-streamer"))
				calls := 0
				var managed string
				var publications []string
				material, err := prepare(context.Background(), Options{InputPath: input, RuntimeAnchor: anchor}, func(ctx context.Context, opts hitraceconv.Options) (hitraceconv.Result, error) {
					calls++
					result, err := hitraceconv.PrepareFile(ctx, opts)
					if err != nil {
						t.Fatalf("real converter fixture failed before receipt corruption: %v", err)
					}
					managed = filepath.Dir(opts.OutputPath)
					if result.OutputPath != "" {
						publications = append(publications, result.OutputPath)
					}
					for _, artifact := range result.Artifacts {
						publications = append(publications, artifact.Path)
					}
					if len(publications) == 0 {
						t.Fatal("real conversion did not exercise publication cleanup")
					}
					for _, path := range publications {
						if _, err := os.Stat(path); err != nil {
							t.Fatalf("real converter output is absent before rejection: %q: %v", path, err)
						}
					}
					if family == "text" {
						text := result.TextTransport
						if text == nil || result.GzipInputProvenance != nil || result.OutputPath == "" || result.BundlePath != "" {
							t.Fatalf("text fixture did not produce its exclusive real transport receipt: %+v", result)
						}
						if corruption == "missing" {
							result.TextTransport = nil
						} else {
							result.GzipInputProvenance = &tracebundle.GzipInputProvenance{
								Profile:     tracebundle.GzipInputProfileV1,
								SourceBytes: text.SourceBytes, SourceSHA256: text.SourceSHA256, SourceGeneration: text.SourceGeneration,
								DecodedFormat: "linux_perf_data", DecodedBytes: text.DecodedBytes,
								DecodedSHA256: text.DecodedSHA256, DecodedGeneration: text.DecodedGeneration,
							}
							if err := tracebundle.ValidateGzipInputProvenance(result.GzipInputProvenance); err != nil {
								t.Fatalf("mixed fixture must have a syntactically valid extra receipt: %v", err)
							}
						}
					} else {
						binary := result.GzipInputProvenance
						if binary == nil || result.TextTransport != nil || result.BundlePath == "" || hitraceconv.QueryReadyPerfTracePath(result.Artifacts) == "" {
							t.Fatalf("binary fixture did not produce an exclusive transport receipt and queryable sample: %+v", result)
						}
						if corruption == "missing" {
							result.GzipInputProvenance = nil
						} else {
							result.TextTransport = &hitraceconv.GzipTextTransportResult{
								Profile: hitraceconv.GzipTextTransportProfile, SourcePath: input,
								SourceBytes: binary.SourceBytes, SourceSHA256: binary.SourceSHA256, SourceGeneration: binary.SourceGeneration,
								DecodedPath: opts.OutputPath, DecodedBytes: binary.DecodedBytes,
								DecodedSHA256: binary.DecodedSHA256, DecodedGeneration: binary.DecodedGeneration,
							}
						}
					}
					return result, nil
				})
				if material != nil || err == nil || !strings.Contains(err.Error(), "requires exactly one text or binary transport receipt") || calls != 1 {
					t.Fatalf("invalid transport receipt set escaped the contract gate: material=%+v calls=%d err=%v", material, calls, err)
				}
				for _, path := range append(publications, managed) {
					if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Fatalf("rejected preparation retained a publication: %q: %v", path, err)
					}
				}
				if entries, err := os.ReadDir(anchor); err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(prior) {
					t.Fatalf("rejected preparation leaked managed or private staging: entries=%v err=%v", entries, err)
				}
				if got, err := os.ReadFile(prior); err != nil || !bytes.Equal(got, priorBody) {
					t.Fatalf("cleanup changed an existing publication: %v", err)
				}
				if got, err := os.ReadFile(input); err != nil || !bytes.Equal(got, original) {
					t.Fatalf("rejected receipt changed the compressed original: %v", err)
				}
				if entries, err := os.ReadDir(sourceDir); err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(input) {
					t.Fatalf("rejected preparation wrote beside the original: entries=%v err=%v", entries, err)
				}
			})
		}
	}
}
