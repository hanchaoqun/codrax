package repl

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/outputdump"
)

type namedInputRoutingContextKey struct{}

// WithNamedInputRoutingContext supplies first-turn content navigation to both
// classifier lanes. It does not attach files, retain query authority, modify
// the current request, or override typed policy. Its short local probe budget
// never becomes the later LLM/stream lifetime budget.
func WithNamedInputRoutingContext(ctx context.Context, request, repoRoot, runtimeAnchor string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithValue(ctx, namedInputRoutingContextKey{}, "")
	paths := outputdump.NamedInputPathsFromRequest(request, repoRoot, 8)
	if len(paths) == 0 {
		return ctx
	}
	if runtimeAnchor == "" {
		cwd, err := os.Getwd()
		if err == nil {
			runtimeAnchor = filepath.Join(cwd, ".codrax")
		}
	}
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	profile := make([]hitraceconv.InputRoutingCapability, 0, len(paths))
	for _, path := range paths {
		profile = append(profile, hitraceconv.ProbeInputRoutingCapability(probeCtx, path, runtimeAnchor))
	}
	encoded, err := json.Marshal(profile)
	if err != nil {
		return ctx
	}
	return context.WithValue(ctx, namedInputRoutingContextKey{}, string(encoded))
}

func namedInputRoutingContext(ctx context.Context) string {
	profile, _ := ctx.Value(namedInputRoutingContextKey{}).(string)
	if profile == "" {
		return ""
	}
	return "## current_named_inputs (navigation only; bounded to 8 files, 2 seconds and 32 MiB per SQLite snapshot)\n" + profile + "\n\n"
}
