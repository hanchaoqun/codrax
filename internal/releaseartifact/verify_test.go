package releaseartifact

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const testReleaseTag = "releaseartifact_contract_test"

func TestVerifyAcceptsMatchingEmbeddedPayload(t *testing.T) {
	repo, artifact := buildTestArtifact(t, PayloadLinux)
	if err := Verify(testOptions(repo, artifact, PayloadLinux)); err != nil {
		t.Fatalf("matching artifact rejected: %v", err)
	}
}

func TestVerifyRejectsArtifactContractMismatches(t *testing.T) {
	repo, artifact := buildTestArtifact(t, PayloadLinux)
	tests := []struct {
		name   string
		mutate func(*Options)
		want   string
	}{
		{name: "opposite payload", mutate: func(o *Options) { o.Payload = PayloadWindows }, want: "linux-amd64 payload count=1 want=0"},
		{name: "wrong goos", mutate: func(o *Options) { o.GOOS = oppositeGOOS(runtime.GOOS) }, want: "artifact build setting GOOS"},
		{name: "wrong goarch", mutate: func(o *Options) { o.GOARCH = "not-an-architecture" }, want: "artifact build setting GOARCH"},
		{name: "wrong cgo", mutate: func(o *Options) { o.CGOEnabled = "1" }, want: "artifact build setting CGO_ENABLED"},
		{name: "missing required tag", mutate: func(o *Options) { o.RequiredTags = []string{"absent_release_tag"} }, want: "lack required tag"},
		{name: "forbidden tag", mutate: func(o *Options) { o.ForbiddenTags = []string{testReleaseTag} }, want: "contain forbidden tag"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := testOptions(repo, artifact, PayloadLinux)
			test.mutate(&options)
			err := Verify(options)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Verify error=%v, want containing %q", err, test.want)
			}
		})
	}
}

func TestVerifyRejectsRepositoryPayloadManifestDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string)
		want   string
	}{
		{
			name: "corrupt payload",
			mutate: func(t *testing.T, repo string) {
				path := payloadFile(repo, PayloadLinux)
				body := mustReadFile(t, path)
				body[len(body)/2] ^= 0xff
				mustWriteFile(t, path, body, 0o755)
			},
			want: "payload sha256=",
		},
		{
			name: "declared hash",
			mutate: func(t *testing.T, repo string) {
				mutateManifest(t, repo, PayloadLinux, func(m *streamerManifest) { m.Platforms[0].SHA256 = strings.Repeat("0", 64) })
			},
			want: "payload sha256=",
		},
		{
			name: "uppercase hash is noncanonical",
			mutate: func(t *testing.T, repo string) {
				mutateManifest(t, repo, PayloadLinux, func(m *streamerManifest) { m.Platforms[0].SHA256 = strings.ToUpper(m.Platforms[0].SHA256) })
			},
			want: "lowercase hexadecimal",
		},
		{
			name: "declared size",
			mutate: func(t *testing.T, repo string) {
				mutateManifest(t, repo, PayloadLinux, func(m *streamerManifest) { m.Platforms[0].SizeBytes++ })
			},
			want: "payload size=",
		},
		{
			name: "declared glibc baseline",
			mutate: func(t *testing.T, repo string) {
				mutateManifest(t, repo, PayloadLinux, func(m *streamerManifest) { m.Platforms[0].MinimumGlibc = "2.31" })
			},
			want: "minimum_glibc=\"2.31\" want=2.34",
		},
		{
			name: "tuple",
			mutate: func(t *testing.T, repo string) {
				mutateManifest(t, repo, PayloadLinux, func(m *streamerManifest) { m.Platforms[0].GOARCH = "arm64" })
			},
			want: "platform tuple=linux/arm64 want=linux/amd64",
		},
		{
			name: "asset path",
			mutate: func(t *testing.T, repo string) {
				mutateManifest(t, repo, PayloadLinux, func(m *streamerManifest) { m.Platforms[0].Path = "nested/trace_streamer" })
			},
			want: "want exact basename",
		},
		{
			name: "multiple platform records",
			mutate: func(t *testing.T, repo string) {
				mutateManifest(t, repo, PayloadLinux, func(m *streamerManifest) { m.Platforms = append(m.Platforms, m.Platforms[0]) })
			},
			want: "platform count=2 want=1",
		},
		{
			name: "floating source ref",
			mutate: func(t *testing.T, repo string) {
				mutateManifest(t, repo, PayloadLinux, func(m *streamerManifest) { m.SourceURL = "https://example.invalid/tree/main" })
			},
			want: "source_url must pin upstream_ref",
		},
		{
			name: "unknown manifest field",
			mutate: func(t *testing.T, repo string) {
				path := manifestFile(repo, PayloadLinux)
				body := bytes.TrimSpace(mustReadFile(t, path))
				body = append(body[:len(body)-1], []byte(",\n  \"unreviewed\": true\n}\n")...)
				mustWriteFile(t, path, body, 0o644)
			},
			want: "unknown field",
		},
		{
			name: "duplicate manifest field",
			mutate: func(t *testing.T, repo string) {
				path := manifestFile(repo, PayloadLinux)
				body := bytes.TrimSpace(mustReadFile(t, path))
				body = append(body[:len(body)-1], []byte(",\n  \"redistribution_status\": \"approved\"\n}\n")...)
				mustWriteFile(t, path, body, 0o644)
			},
			want: "duplicate JSON object key",
		},
		{
			name: "case-aliased duplicate manifest field",
			mutate: func(t *testing.T, repo string) {
				path := manifestFile(repo, PayloadLinux)
				body := bytes.TrimSpace(mustReadFile(t, path))
				body = append(body[:len(body)-1], []byte(",\n  \"Redistribution_Status\": \"approved\"\n}\n")...)
				mustWriteFile(t, path, body, 0o644)
			},
			want: "duplicate JSON object key",
		},
		{
			name: "case-aliased manifest field",
			mutate: func(t *testing.T, repo string) {
				path := manifestFile(repo, PayloadLinux)
				body := bytes.Replace(mustReadFile(t, path), []byte(`"source_url"`), []byte(`"Source_URL"`), 1)
				mustWriteFile(t, path, body, 0o644)
			},
			want: "does not exactly match the schema",
		},
		{
			name: "trailing json",
			mutate: func(t *testing.T, repo string) {
				path := manifestFile(repo, PayloadLinux)
				body := append(mustReadFile(t, path), []byte("\n{}\n")...)
				mustWriteFile(t, path, body, 0o644)
			},
			want: "multiple JSON values",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, artifact := buildTestArtifact(t, PayloadLinux)
			test.mutate(t, repo)
			err := Verify(testOptions(repo, artifact, PayloadLinux))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Verify error=%v, want containing %q", err, test.want)
			}
		})
	}
}

func TestLinuxRuntimeContractRejectsMuslStandardAndDynamicStaticArtifacts(t *testing.T) {
	repo := releaseArtifactRepositoryRoot(t)
	glibcPayload := mustReadFile(t, filepath.Join(repo, "internal", "hitraceconv", "embedded_trace_streamer", "linux-amd64", "trace_streamer"))
	if err := verifyLinuxRuntime(glibcPayload, "amd64", LinuxRuntimeGlibc); err != nil {
		t.Fatalf("pinned upstream glibc ELF rejected: %v", err)
	}
	if baseline, err := minimumRequiredGlibc(glibcPayload); err != nil || baseline != "2.34" {
		t.Fatalf("pinned upstream glibc baseline=%q err=%v, want 2.34", baseline, err)
	}
	if err := verifyLinuxRuntime(glibcPayload, "amd64", LinuxRuntimeStatic); err == nil || !strings.Contains(err.Error(), "Linux static artifact has interpreter") {
		t.Fatalf("dynamic ELF passed static runtime contract: %v", err)
	}

	glibcInterpreter := []byte("/lib64/ld-linux-x86-64.so.2")
	muslInterpreter := append([]byte("/lib/ld-musl-x86_64.so.1"), 0, 0, 0)
	muslPayload := bytes.Replace(glibcPayload, glibcInterpreter, muslInterpreter, 1)
	if bytes.Equal(muslPayload, glibcPayload) {
		t.Fatal("test fixture does not contain the pinned glibc interpreter")
	}
	if err := verifyLinuxRuntime(muslPayload, "amd64", LinuxRuntimeGlibc); err == nil || !strings.Contains(err.Error(), "want portable glibc interpreter") {
		t.Fatalf("musl interpreter passed glibc runtime contract: %v", err)
	}

	staticArtifact := buildLinuxStaticTestArtifact(t)
	staticBody := mustReadFile(t, staticArtifact)
	if err := verifyLinuxRuntime(staticBody, "amd64", LinuxRuntimeStatic); err != nil {
		t.Fatalf("CGO-disabled static Linux ELF rejected: %v", err)
	}
	if err := verifyLinuxRuntime(staticBody, "amd64", LinuxRuntimeGlibc); err == nil || !strings.Contains(err.Error(), "want portable glibc interpreter") {
		t.Fatalf("static Linux ELF passed glibc runtime contract: %v", err)
	}
}

func buildLinuxStaticTestArtifact(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	mustWriteFile(t, filepath.Join(repo, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644)
	mustWriteFile(t, filepath.Join(repo, "go.mod"), []byte("module linuxstatictest\n\ngo 1.22\n"), 0o644)
	artifact := filepath.Join(repo, "artifact")
	command := exec.Command("go", "build", "-trimpath", "-o", artifact, ".")
	command.Dir = repo
	command.Env = append(withoutGoBuildEnv(os.Environ()),
		"CGO_ENABLED=0",
		"GOARCH=amd64",
		"GOENV=off",
		"GOFLAGS=",
		"GOOS=linux",
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build static Linux test artifact: %v\n%s", err, output)
	}
	return artifact
}

func TestVerifyRejectsSymlinkArtifact(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not generally available to unprivileged Windows builds")
	}
	repo, artifact := buildTestArtifact(t, PayloadLinux)
	link := filepath.Join(t.TempDir(), "artifact-link")
	if err := os.Symlink(artifact, link); err != nil {
		t.Fatal(err)
	}
	err := Verify(testOptions(repo, link, PayloadLinux))
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("symlink artifact error=%v, want regular-file rejection", err)
	}
}

func TestVerifyCommercialArtifactBindsFinalBinaryToApprovedPayloads(t *testing.T) {
	repo, _ := commercialTestRepository(t, true)
	artifact := buildTestArtifactInRepository(t, repo, PayloadLinux)
	options := testOptions(repo, artifact, PayloadLinux)
	if err := VerifyCommercialArtifact(options); err != nil {
		t.Fatalf("approved commercial artifact rejected: %v", err)
	}

	body := mustReadFile(t, payloadFile(repo, PayloadLinux))
	body[len(body)/2] ^= 0xff
	mustWriteFile(t, payloadFile(repo, PayloadLinux), body, 0o755)
	if err := VerifyCommercialArtifact(options); err == nil || !strings.Contains(err.Error(), "payload sha256=") {
		t.Fatalf("commercial artifact accepted post-approval payload drift: %v", err)
	}
}

func buildTestArtifact(t *testing.T, payload PayloadExpectation) (string, string) {
	t.Helper()
	repo := t.TempDir()
	writeTestRepositoryPayloads(t, repo)
	return repo, buildTestArtifactInRepository(t, repo, payload)
}

func buildTestArtifactInRepository(t *testing.T, repo string, payload PayloadExpectation) string {
	t.Helper()
	directory, binary := payloadLocation(payload)
	source := fmt.Sprintf(`package main

import (
	"embed"
	"fmt"
)

//go:embed internal/hitraceconv/embedded_trace_streamer/%s
var assets embed.FS

func main() {
	binary, err := assets.ReadFile("internal/hitraceconv/embedded_trace_streamer/%s/%s")
	if err != nil { panic(err) }
	manifest, err := assets.ReadFile("internal/hitraceconv/embedded_trace_streamer/%s/manifest.json")
	if err != nil { panic(err) }
	fmt.Print(len(binary) + len(manifest))
}
`, directory, directory, binary, directory)
	mustWriteFile(t, filepath.Join(repo, "main.go"), []byte(source), 0o644)
	mustWriteFile(t, filepath.Join(repo, "go.mod"), []byte("module releaseartifacttest\n\ngo 1.22\n"), 0o644)
	artifact := filepath.Join(repo, "artifact")
	if runtime.GOOS == "windows" {
		artifact += ".exe"
	}
	cmd := exec.Command("go", "build", "-trimpath", "-tags", testReleaseTag, "-o", artifact, ".")
	cmd.Dir = repo
	cmd.Env = append(withoutGoBuildEnv(os.Environ()),
		"CGO_ENABLED=0",
		"GOARCH="+runtime.GOARCH,
		"GOENV=off",
		"GOFLAGS=",
		"GOOS="+runtime.GOOS,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build test artifact: %v\n%s", err, output)
	}
	return artifact
}

func writeTestRepositoryPayloads(t *testing.T, repo string) {
	t.Helper()
	for _, contract := range platformContracts {
		seed := []byte("codrax-release-verifier-" + string(contract.name) + "-")
		payload := bytes.Repeat(seed, 257)
		sum := sha256.Sum256(payload)
		minimumGlibc := ""
		if contract.goos == "linux" {
			minimumGlibc = "2.34"
		}
		manifest := streamerManifest{
			SourceURL:              "https://example.invalid/tree/0123456789abcdef0123456789abcdef01234567/assets/trace_streamer/" + contract.directory,
			UpstreamRef:            "0123456789abcdef0123456789abcdef01234567",
			Version:                "test",
			AcquisitionRepoLicense: "Apache-2.0",
			LicenseConcluded:       "NOASSERTION",
			RedistributionStatus:   "blocked",
			ProductApprovalRef:     "test-only",
			Platforms: []streamerPlatform{{
				GOOS:         contract.goos,
				GOARCH:       contract.goarch,
				Path:         contract.binaryName,
				SHA256:       hex.EncodeToString(sum[:]),
				SizeBytes:    int64(len(payload)),
				ActualFormat: "test fixture",
				MinimumGlibc: minimumGlibc,
			}},
		}
		body, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		body = append(body, '\n')
		dir := filepath.Join(repo, "internal", "hitraceconv", "embedded_trace_streamer", contract.directory)
		mustWriteFile(t, filepath.Join(dir, "manifest.json"), body, 0o644)
		mustWriteFile(t, filepath.Join(dir, contract.binaryName), payload, 0o755)
	}
}

func mutateManifest(t *testing.T, repo string, payload PayloadExpectation, mutate func(*streamerManifest)) {
	t.Helper()
	path := manifestFile(repo, payload)
	var manifest streamerManifest
	if err := json.Unmarshal(mustReadFile(t, path), &manifest); err != nil {
		t.Fatal(err)
	}
	mutate(&manifest)
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, path, append(body, '\n'), 0o644)
}

func testOptions(repo, artifact string, payload PayloadExpectation) Options {
	return Options{
		ArtifactPath: artifact,
		RepoRoot:     repo,
		GOOS:         runtime.GOOS,
		GOARCH:       runtime.GOARCH,
		CGOEnabled:   "0",
		Payload:      payload,
		RequiredTags: []string{testReleaseTag},
	}
}

func payloadLocation(payload PayloadExpectation) (string, string) {
	for _, contract := range platformContracts {
		if contract.name == payload {
			return contract.directory, contract.binaryName
		}
	}
	panic("unsupported test payload: " + payload)
}

func manifestFile(repo string, payload PayloadExpectation) string {
	directory, _ := payloadLocation(payload)
	return filepath.Join(repo, "internal", "hitraceconv", "embedded_trace_streamer", directory, "manifest.json")
}

func payloadFile(repo string, payload PayloadExpectation) string {
	directory, binary := payloadLocation(payload)
	return filepath.Join(repo, "internal", "hitraceconv", "embedded_trace_streamer", directory, binary)
}

func withoutGoBuildEnv(base []string) []string {
	blocked := map[string]bool{"CGO_ENABLED": true, "GOARCH": true, "GOENV": true, "GOFLAGS": true, "GOOS": true}
	out := make([]string, 0, len(base))
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if !ok || !blocked[strings.ToUpper(key)] {
			out = append(out, item)
		}
	}
	return out
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func mustWriteFile(t *testing.T, path string, body []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, mode); err != nil {
		t.Fatal(err)
	}
}

func oppositeGOOS(goos string) string {
	if goos == "windows" {
		return "linux"
	}
	return "windows"
}
