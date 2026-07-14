package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hanchaoqun/codrax/internal/releaseartifact"
)

func main() {
	repoRoot := flag.String("repo", ".", "repository root")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "commercial trace_streamer release verification does not accept positional arguments")
		os.Exit(2)
	}
	if err := releaseartifact.VerifyCommercialTraceStreamerRelease(*repoRoot); err != nil {
		fmt.Fprintf(os.Stderr, "commercial trace_streamer release verification failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("commercial trace_streamer release verification passed")
}
