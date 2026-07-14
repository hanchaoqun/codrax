package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/hanchaoqun/codrax/internal/releaseartifact"
)

func main() {
	var options releaseartifact.Options
	var payload string
	var linuxRuntime string
	var requiredTags string
	var forbiddenTags string
	var commercialRelease bool
	flag.StringVar(&options.ArtifactPath, "artifact", "", "release artifact to verify")
	flag.StringVar(&options.RepoRoot, "repo", ".", "Codrax repository root")
	flag.StringVar(&options.GOOS, "goos", "", "required artifact GOOS")
	flag.StringVar(&options.GOARCH, "goarch", "", "required artifact GOARCH")
	flag.StringVar(&options.CGOEnabled, "cgo", "", "required artifact CGO_ENABLED build setting")
	flag.StringVar(&payload, "payload", "", "required payload: linux-amd64, windows-amd64, or none")
	flag.StringVar(&linuxRuntime, "linux-runtime", "", "required Linux runtime contract: glibc or static")
	flag.StringVar(&requiredTags, "require-tags", "", "comma-separated required build tags")
	flag.StringVar(&forbiddenTags, "forbid-tags", "", "comma-separated forbidden build tags")
	flag.BoolVar(&commercialRelease, "commercial-release", false, "bind the final artifact to the approved commercial evidence set")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "positional arguments are not accepted")
		os.Exit(2)
	}
	options.Payload = releaseartifact.PayloadExpectation(payload)
	options.LinuxRuntime = releaseartifact.LinuxRuntimeExpectation(linuxRuntime)
	options.RequiredTags = splitTags(requiredTags)
	options.ForbiddenTags = splitTags(forbiddenTags)
	verify := releaseartifact.Verify
	if commercialRelease {
		verify = releaseartifact.VerifyCommercialArtifact
	}
	if err := verify(options); err != nil {
		fmt.Fprintf(os.Stderr, "release artifact verification failed: %v\n", err)
		os.Exit(1)
	}
}

func splitTags(value string) []string {
	var out []string
	for _, tag := range strings.Split(value, ",") {
		if tag = strings.TrimSpace(tag); tag != "" {
			out = append(out, tag)
		}
	}
	return out
}
