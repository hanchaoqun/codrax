package hitraceconv

import (
	"bufio"
	"go/build/constraint"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestEmbeddedTraceStreamerBuildTagMatrix pins the distribution contract
// without depending on the host platform: default and unrelated-tag builds
// select exactly one payload-or-gap stub, while slim_streamer selects none.
// The legacy embed_streamer tag is deliberately inert; when combined with
// slim_streamer, the explicit slim opt-out wins deterministically.
func TestEmbeddedTraceStreamerBuildTagMatrix(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate build-tag matrix test")
	}
	dir := filepath.Dir(currentFile)
	expressions := map[string]constraint.Expr{}
	for _, name := range []string{
		"embedded_trace_streamer_payload_linux_amd64.go",
		"embedded_trace_streamer_payload_windows_amd64.go",
		"embedded_trace_streamer_payload_unbundled.go",
	} {
		expressions[name] = readGoBuildConstraint(t, filepath.Join(dir, name))
	}

	type platform struct{ goos, goarch string }
	platforms := []platform{
		{goos: "linux", goarch: "amd64"},
		{goos: "windows", goarch: "amd64"},
		{goos: "darwin", goarch: "arm64"},
		{goos: "linux", goarch: "arm64"},
	}
	tagSets := []struct {
		name      string
		tags      map[string]bool
		wantStubs int
	}{
		{name: "default", tags: nil, wantStubs: 1},
		{name: "unrelated tag", tags: map[string]bool{"customer_release": true}, wantStubs: 1},
		{name: "legacy embed tag", tags: map[string]bool{"embed_streamer": true}, wantStubs: 1},
		{name: "explicit slim", tags: map[string]bool{"slim_streamer": true}, wantStubs: 0},
		{name: "slim wins over legacy", tags: map[string]bool{"embed_streamer": true, "slim_streamer": true}, wantStubs: 0},
	}
	for _, target := range platforms {
		for _, tagSet := range tagSets {
			t.Run(target.goos+"-"+target.goarch+"/"+tagSet.name, func(t *testing.T) {
				selected := 0
				for _, expression := range expressions {
					if expression.Eval(func(tag string) bool {
						return tag == target.goos || tag == target.goarch || tagSet.tags[tag]
					}) {
						selected++
					}
				}
				if selected != tagSet.wantStubs {
					t.Fatalf("selected payload/gap stubs=%d, want %d", selected, tagSet.wantStubs)
				}
			})
		}
	}

	slimPin := readGoBuildConstraint(t, filepath.Join(dir, "embedded_trace_streamer_slim_pin_test.go"))
	if !slimPin.Eval(func(tag string) bool { return tag == "slim_streamer" }) ||
		slimPin.Eval(func(tag string) bool { return tag == "embed_streamer" }) {
		t.Fatalf("slim compile pin is not exclusively controlled by slim_streamer: %s", slimPin)
	}
}

func readGoBuildConstraint(t *testing.T, filePath string) constraint.Expr {
	t.Helper()
	file, err := os.Open(filePath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "//go:build ") {
			continue
		}
		expression, err := constraint.Parse(line)
		if err != nil {
			t.Fatalf("parse %s constraint %q: %v", filePath, line, err)
		}
		return expression
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	t.Fatalf("%s has no //go:build constraint", filePath)
	return nil
}
