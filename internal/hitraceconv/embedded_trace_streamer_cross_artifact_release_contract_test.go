package hitraceconv

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/releaseartifact"
)

func TestCrossTargetEmbedProbeCarriesExactlyTargetTraceStreamer(t *testing.T) {
	const standardTag = "codrax_embedded_streamer_release"
	tests := []struct {
		name    string
		goos    string
		goarch  string
		tags    string
		poison  string
		payload releaseartifact.PayloadExpectation
		runtime releaseartifact.LinuxRuntimeExpectation
	}{
		{name: "linux-amd64-static-embedded", goos: "linux", goarch: "amd64", tags: standardTag, poison: "slim_streamer", payload: releaseartifact.PayloadLinux, runtime: releaseartifact.LinuxRuntimeStatic},
		{name: "windows-amd64-default", goos: "windows", goarch: "amd64", tags: standardTag, poison: "slim_streamer", payload: releaseartifact.PayloadWindows},
		{name: "linux-amd64-static-slim", goos: "linux", goarch: "amd64", tags: "slim_streamer", poison: standardTag, payload: releaseartifact.PayloadNone, runtime: releaseartifact.LinuxRuntimeStatic},
		{name: "windows-amd64-slim", goos: "windows", goarch: "amd64", tags: "slim_streamer", poison: standardTag, payload: releaseartifact.PayloadNone},
		{name: "linux-arm64-platform-gap", goos: "linux", goarch: "arm64", tags: standardTag, poison: "slim_streamer", payload: releaseartifact.PayloadNone, runtime: releaseartifact.LinuxRuntimeStatic},
		{name: "windows-arm64-platform-gap", goos: "windows", goarch: "arm64", tags: standardTag, poison: "slim_streamer", payload: releaseartifact.PayloadNone},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "embedprobe")
			if test.goos == "windows" {
				output += ".exe"
			}
			args := []string{"build", "-trimpath", "-o", output}
			args = append(args, "-tags="+test.poison)
			if test.tags != "" {
				args = append(args, "-tags", test.tags)
			}
			args = append(args, "./testdata/embedprobe")
			cmd := exec.Command("go", args...)
			cmd.Env = releaseBuildEnv(os.Environ(), map[string]string{
				"CGO_ENABLED": "0",
				"GOARCH":      test.goarch,
				"GOENV":       "off",
				"GOFLAGS":     "-tags=" + test.poison,
				"GOOS":        test.goos,
			})
			if body, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("cross-build %s: %v\n%s", test.name, err, body)
			}

			forbidden := []string(nil)
			if test.tags == standardTag {
				forbidden = []string{"slim_streamer"}
			}
			options := releaseartifact.Options{
				ArtifactPath:  output,
				RepoRoot:      filepath.Join("..", ".."),
				GOOS:          test.goos,
				GOARCH:        test.goarch,
				CGOEnabled:    "0",
				Payload:       test.payload,
				LinuxRuntime:  test.runtime,
				RequiredTags:  []string{test.tags},
				ForbiddenTags: forbidden,
			}
			if err := releaseartifact.Verify(options); err != nil {
				t.Fatalf("verify %s: %v", test.name, err)
			}
			if test.tags == "slim_streamer" && test.goarch == "amd64" {
				if test.goos == "linux" {
					options.Payload = releaseartifact.PayloadLinux
				} else {
					options.Payload = releaseartifact.PayloadWindows
				}
				options.RequiredTags = nil
				if err := releaseartifact.Verify(options); err == nil {
					t.Fatal("slim artifact passed the corresponding embedded payload contract")
				}
			}
			if test.name == "linux-amd64-static-embedded" {
				wrong := options
				wrong.Payload = releaseartifact.PayloadNone
				if err := releaseartifact.Verify(wrong); err == nil {
					t.Fatal("static embedded artifact passed the zero-payload contract")
				}
				wrong = options
				wrong.RequiredTags = []string{"slim_streamer"}
				if err := releaseartifact.Verify(wrong); err == nil {
					t.Fatal("static embedded artifact passed the slim identity contract")
				}
			}
		})
	}
}

func releaseBuildEnv(base []string, values map[string]string) []string {
	out := make([]string, 0, len(base)+len(values))
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if ok {
			if _, replace := values[strings.ToUpper(key)]; replace {
				continue
			}
		}
		out = append(out, item)
	}
	for _, key := range []string{"CGO_ENABLED", "GOARCH", "GOENV", "GOFLAGS", "GOOS"} {
		out = append(out, fmt.Sprintf("%s=%s", key, values[key]))
	}
	return out
}

func TestReleaseBuildEnvRejectsPersistentGoEnvironmentPoison(t *testing.T) {
	values := map[string]string{
		"CGO_ENABLED": "0",
		"GOARCH":      runtime.GOARCH,
		"GOENV":       "off",
		"GOFLAGS":     "",
		"GOOS":        runtime.GOOS,
	}
	env := releaseBuildEnv([]string{
		"GOENV=/tmp/poisoned-go-env",
		"GOFLAGS=-tags=slim_streamer",
		"GOOS=windows",
		"GOARCH=arm64",
		"CGO_ENABLED=1",
	}, values)
	for key, want := range values {
		prefix := key + "="
		count := 0
		for _, item := range env {
			if strings.HasPrefix(strings.ToUpper(item), prefix) {
				count++
				if item != prefix+want {
					t.Fatalf("%s environment=%q want=%q", key, item, prefix+want)
				}
			}
		}
		if count != 1 {
			t.Fatalf("%s environment count=%d want=1: %v", key, count, env)
		}
	}
}
