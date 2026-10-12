package repl

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/outputdump"
)

const dataTaskNativeReaderStatus = "needs_native_reader"
const dataTaskNativeReaderTeaching = "On the initial plan only, if the task needs native runtime observations instead of generic dataset processing, emit status=needs_native_reader with input_paths naming the current request's exact files and no actions or script. This delegates the unchanged request to the read-only native analysis pipeline; reader candidates are navigation, not prepared inputs or consumed content. Ordinary datasets remain in this workflow."

type nativeDataHandoff struct{ paths []string }

func (h *nativeDataHandoff) Error() string {
	return "data task delegated to read-only native observation pipeline"
}
func (h *nativeDataHandoff) NativeObservationInputs() []string {
	return append([]string(nil), h.paths...)
}

// NativeDataHandoffPolicy changes only the execution lane after an explicit
// initial planner decision. Actual source admission and preparation still run.
func NativeDataHandoffPolicy(policy TurnPolicy, err error) (TurnPolicy, bool) {
	var handoff *nativeDataHandoff
	if policy.Route != RouteData || policy.WriteIntent == WriteIntentExplicitChange || policy.NeedsOperationAccess || !errors.As(err, &handoff) || handoff == nil || len(handoff.paths) == 0 {
		return policy, false
	}
	policy.Route = RouteRepo
	policy.NeedsRepoAccess = true
	policy.NeedsDataAccess = false
	policy.Operation = "investigate"
	policy.DataTaskKind = ""
	policy.OperationKind = ""
	policy.WriteIntent = WriteIntentAnalysisOnly
	policy.Source = "artifact"
	return policy, true
}

func initialDataNativeHandoff(ctx context.Context, request, repoRoot string, plan dataquery.TaskPlan) error {
	if plan.Status != dataTaskNativeReaderStatus {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(plan.InputPaths) == 0 || len(plan.Actions) > 0 || strings.TrimSpace(plan.Script) != "" || plan.ContinueAfter {
		return fmt.Errorf("needs_native_reader requires current named input_paths and no actions, script or continuation")
	}
	available := map[string]bool{}
	for _, p := range outputdump.NamedInputPathsFromRequest(request, repoRoot, 8) {
		available[p] = true
	}
	var paths []string
	for _, p := range plan.InputPaths {
		abs := strings.TrimSpace(p)
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(repoRoot, abs)
		}
		abs, err := filepath.Abs(abs)
		if err != nil || !available[abs] {
			return fmt.Errorf("native reader input %q is not an exact current named regular file", p)
		}
		paths = append(paths, abs)
	}
	return &nativeDataHandoff{paths: paths}
}

func discoverCurrentDataInputs(ctx context.Context, request, repoRoot, anchor string) ([]dataquery.CandidateFile, *dataquery.NamedInputs, error) {
	paths := outputdump.NamedInputPathsFromRequest(request, repoRoot, 8)
	candidates, named, err := dataquery.DiscoverCandidateFilesWithNamed(repoRoot, paths, 240)
	if err != nil {
		return nil, nil, err
	}
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	namedPaths := make(map[string]bool, len(paths))
	for _, path := range paths {
		namedPaths[path] = true
	}
	for i := range candidates {
		if !namedPaths[candidates[i].Path] {
			continue
		}
		capability := hitraceconv.ProbeInputRoutingCapability(probeCtx, candidates[i].Path, anchor)
		candidates[i].NativeReaderCandidate = capability.NativeReader
		candidates[i].NativeViewCandidates = append([]string(nil), capability.CandidateViews...)
	}
	return candidates, named, nil
}
