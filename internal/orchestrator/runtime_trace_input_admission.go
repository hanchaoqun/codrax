package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const maxTypedNamedTraceAdmissionPaths = 32

// validateRuntimeTraceInputsBeforeInvestigation is the run-entry defense for
// callers that bypass cmd/repl attachment loaders. Raw request path-shape is
// deliberately not treated as trace-analysis intent: code-analysis/write
// requests must remain able to name binary converter fixtures. Explicit path
// calls enter the separate trace_query gate before any physical trace parser.
// No pipeline event, pre-stage, agent, or tool is allowed to run until the
// explicit attached trace passes this gate.
func validateRuntimeTraceInputsBeforeInvestigation(ctx context.Context, attachedTrace string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(attachedTrace) != "" {
		// Frontend loaders publish complete UTF-8 even when their byte cap cuts a
		// source. Direct setters carry no truncation provenance, so this backstop
		// must validate the immutable string as complete input.
		if err := attachment.ValidateTextString(attachment.KindTrace, "attached trace", attachedTrace, false); err != nil {
			return err
		}
		if err := attachment.ValidateSingleTraceAttachmentProvenance(attachedTrace); err != nil {
			return err
		}
	}
	return ctx.Err()
}

// validateTypedNamedTraceInputsBeforeExploration closes the natural-language
// path lane without guessing user intent from raw prose. The analyzer is
// allowed to classify the turn, but no explore/extract/finalize task and no
// physical trace parser may run until every request-path trace in the
// deterministic preflight profile passes the shared held-file admission gate.
//
// The blocking decision consumes only typed analyzer policy/scope plus a
// deterministic existing-file profile. Path shape alone remains identity
// guidance and can never block converter/parser source-code work.
func validateTypedNamedTraceInputsBeforeExploration(ctx context.Context, bus *types.BusContext, request string) error {
	return validateTypedNamedTraceInputsWithAnchor(ctx, bus, request, absoluteTraceRuntimeAnchor(""))
}

func (o *Orchestrator) validateTypedNamedTraceInputsBeforeExploration(ctx context.Context, bus *types.BusContext, request string) error {
	return validateTypedNamedTraceInputsWithAnchor(ctx, bus, request, absoluteTraceRuntimeAnchor(o.traceRuntimeAnchor))
}

func validateTypedNamedTraceInputsWithAnchor(ctx context.Context, bus *types.BusContext, request, runtimeAnchor string) error {
	if bus == nil || bus.AnalysisIR == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	rm := bus.AnalysisIR.RequestModel
	profile := types.NormalizeRuntimeArtifactPreflightProfile(bus.RuntimeArtifactPreflight)
	// The current typed intent, not the file suffix or prose, permits native
	// content discovery. The new candidates remain local until real preparation
	// and complete input admission succeed below.
	profile = namedTraceContentAdmissionProfile(ctx, bus, request, runtimeAnchor, profile)
	profile = comparisonNamedTraceProfile(rm, profile)
	if err := ctx.Err(); err != nil {
		return err
	}
	if !typedNamedTraceAdmissionEnabled(rm, profile, request) {
		return nil
	}

	paths, err := typedNamedTraceAdmissionPaths(profile, rm, request, bus.RepoRoot)
	if err != nil {
		return err
	}
	catalog, err := namedTraceComparisonCatalog(ctx, bus, paths)
	if err != nil {
		return err
	}
	var failures []error
	ready := 0
	for index, path := range paths {
		err := prepareAndAdmitNamedTracePath(ctx, bus, path)
		if err == nil {
			ready++
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		wrapped := fmt.Errorf("named trace input %d/%d %q: %w", index+1, len(paths), path, err)
		if namedTraceArchiveSelectionPending(err) {
			markNamedTraceArchiveSelectionPending(&profile, path, bus.RepoRoot)
			if catalog != nil {
				catalog.RecordPreparationFailure(path, wrapped)
			}
			continue
		}
		if catalog == nil {
			return wrapped
		}
		catalog.RecordPreparationFailure(path, wrapped)
		failures = append(failures, wrapped)
	}
	// A pending choice is navigation, not a successful side. Preserve the
	// comparison's existing ready+failed isolation without letting pending
	// inputs hide a fatal set in which no side was actually admitted.
	if len(failures) != 0 && ready == 0 {
		return errors.Join(failures...)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// This remains navigation. A failed side has a catalog issue, not ready
	// data; actual queries still prepare and validate each selected input.
	if catalog != nil {
		if err := bus.Mutable.StageTraceCatalogForInvestigation(catalog); err != nil {
			return err
		}
	}
	bus.RuntimeArtifactPreflight = profile
	return nil
}

func prepareAndAdmitNamedTracePath(ctx context.Context, bus *types.BusContext, path string) error {
	originalPath := path
	var preparedMaterial *attachment.TraceMaterial
	var preparedPreview string
	var namedPrepared bool
	if m := bus.AttachedTraceMaterial; m != nil && typedNamedTraceAdmissionPathKey(path) == typedNamedTraceAdmissionPathKey(resolveTypedNamedTraceSource(m.SourcePath(), bus.RepoRoot)) {
		if err := m.Validate(ctx, bus.AttachedHitrace); err != nil {
			return err
		}
		path = m.QueryPath()
		preparedMaterial, preparedPreview = m, bus.AttachedHitrace
	} else if bus.TraceInputPreparer != nil {
		material, err := bus.TraceInputPreparer.Prepare(ctx, path)
		if err != nil {
			return err
		}
		if material == nil {
			return fmt.Errorf("preparation returned no complete material")
		}
		path = material.QueryPath()
		namedPrepared = true
	}
	if err := tracequery.ValidateTraceInputPath(ctx, path); err != nil {
		return err
	}
	// The complete-query admission read must remain in the same original,
	// derived-output and bundle-member generation as preparation itself.
	if namedPrepared {
		if _, err := bus.TraceInputPreparer.Prepare(ctx, originalPath); err != nil {
			return err
		}
	} else if preparedMaterial != nil {
		if err := preparedMaterial.Validate(ctx, preparedPreview); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func typedNamedTraceAdmissionEnabled(rm types.RequestModel, profile types.RuntimeArtifactPreflightProfile, request string) bool {
	// A source-owned comparison is precise runtime-acquisition intent even
	// when source-code analysis remains allowed. Neither changes the other.
	if hasNamedTraceComparison(rm) {
		return true
	}
	// Legacy inputs still require the settled external-observation policy.
	// Raw request path shape alone can never arm this gate.
	policy := rm.ExternalObservationPolicy
	if policy == nil || (!policy.ExcludesCurrentSource() && !policy.ArtifactCitationsExternalOnly()) {
		return false
	}
	for _, artifact := range profile.Artifacts {
		if artifact.Carrier == "request_path" && artifact.RuntimeArtifactKind() == "trace" {
			return true
		}
	}
	// A generic .sys suffix is not typed trace identity. It can enter this
	// gate only when the analyzer's closed scenario enum independently says
	// this is a performance-bottleneck investigation; external log/report
	// turns with a converter fixture.sys therefore remain source work.
	if rm.Scenario != types.ScenarioPerformanceBottleneck {
		return false
	}
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		if strings.EqualFold(typedNamedTraceCandidateExtension(hint.Path), ".sys") {
			return true
		}
	}
	return len(typedNamedSysTraceSourcesInRequest(request)) > 0
}

func typedNamedTraceAdmissionPaths(
	profile types.RuntimeArtifactPreflightProfile,
	rm types.RequestModel,
	request string,
	repoRoot string,
) ([]string, error) {
	profile = types.NormalizeRuntimeArtifactPreflightProfile(profile)
	seen := map[string]bool{}
	var paths []string
	add := func(source string) error {
		path := resolveTypedNamedTraceSource(source, repoRoot)
		if scope := rm.RuntimeArtifactScopeProfile; path == "" && scope != nil && scope.Comparison.Valid() {
			for _, side := range scope.Comparison.Sides() {
				if side.Path == source {
					path = resolveTypedComparisonTraceSource(source, repoRoot)
				}
			}
		}
		if path == "" {
			return nil
		}
		key := typedNamedTraceAdmissionPathKey(path)
		if seen[key] {
			return nil
		}
		seen[key] = true
		paths = append(paths, path)
		if len(paths) > maxTypedNamedTraceAdmissionPaths {
			return fmt.Errorf("trace input admission: typed request names more than %d existing trace paths; split the comparison into smaller batches", maxTypedNamedTraceAdmissionPaths)
		}
		return nil
	}
	for _, artifact := range profile.Artifacts {
		if artifact.Carrier != "request_path" || artifact.RuntimeArtifactKind() != "trace" {
			continue
		}
		if err := add(artifact.Source); err != nil {
			return nil, err
		}
	}

	// Required-file hints are schema-validated typed path carriers. They close
	// the customer .sys lane even when the producer's binary magic is newer
	// than Codrax and therefore could not be content-sniffed at run entry.
	// .sys stays local to this typed trace gate; globally classifying that
	// generic extension as a runtime artifact would corrupt code-analysis and
	// converter-fixture turns.
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		if !typedNamedTraceCandidateSource(hint.Path) {
			continue
		}
		if err := add(hint.Path); err != nil {
			return nil, err
		}
	}
	for _, source := range typedNamedSysTraceSourcesInRequest(request) {
		if err := add(source); err != nil {
			return nil, err
		}
	}
	return paths, nil
}

func typedNamedTraceCandidateSource(source string) bool {
	source = strings.TrimSpace(strings.Trim(source, "\"'`<>"))
	if types.RuntimeArtifactPathKind(source) == "trace" {
		return true
	}
	if strings.HasPrefix(strings.ToLower(source), "file://") {
		parsed, err := url.Parse(source)
		if err != nil || parsed.Scheme != "file" || parsed.Host != "" {
			return false
		}
		source = parsed.Path
	}
	return strings.EqualFold(filepath.Ext(source), ".sys")
}

// typedNamedSysTraceSourcesInRequest enumerates generic .sys paths only after
// typedNamedTraceAdmissionEnabled has accepted the analyzer's structured
// external-observation policy. It is deliberately not a user-intent detector.
// Quoted paths preserve spaces; ordinary tokens support absolute, relative,
// tilde and file:// forms. Every candidate must still resolve to an existing
// local file and pass the full content admission gate before investigation.
func typedNamedSysTraceSourcesInRequest(request string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(raw string) {
		raw = strings.TrimSpace(strings.Trim(raw, "\"'`<>[]{}()，。；;、"))
		if !typedNamedTraceCandidateSource(raw) || !strings.EqualFold(typedNamedTraceCandidateExtension(raw), ".sys") {
			return
		}
		key := raw
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, raw)
	}
	for _, quote := range []byte{'"', '\'', '`'} {
		for start := 0; start < len(request); {
			left := strings.IndexByte(request[start:], quote)
			if left < 0 {
				break
			}
			left += start
			right := strings.IndexByte(request[left+1:], quote)
			if right < 0 {
				break
			}
			right += left + 1
			add(request[left+1 : right])
			start = right + 1
		}
	}
	for _, field := range strings.FieldsFunc(request, func(r rune) bool {
		return r == '"' || r == '\'' || r == '`' || r == '<' || r == '>' ||
			r == '(' || r == ')' || r == '[' || r == ']' || r == '{' || r == '}' ||
			r == '，' || r == '。' || r == '；' || r == ';' || r == '、' ||
			r == ',' || r == '|' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	}) {
		add(field)
	}
	return out
}

func typedNamedTraceCandidateExtension(source string) string {
	source = strings.TrimSpace(strings.Trim(source, "\"'`<>"))
	if strings.HasPrefix(strings.ToLower(source), "file://") {
		parsed, err := url.Parse(source)
		if err != nil || parsed.Scheme != "file" || parsed.Host != "" {
			return ""
		}
		source = parsed.Path
	}
	return filepath.Ext(source)
}

func resolveTypedNamedTraceSource(source, repoRoot string) string {
	return resolveTypedNamedTraceSourceWithAbsent(source, repoRoot, false)
}

func resolveTypedComparisonTraceSource(source, repoRoot string) string {
	return resolveTypedNamedTraceSourceWithAbsent(source, repoRoot, true)
}

func resolveTypedNamedTraceSourceWithAbsent(source, repoRoot string, retainAbsent bool) string {
	source = strings.TrimSpace(strings.Trim(source, "\"'`<>"))
	if source == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(source), "file://") {
		parsed, err := url.Parse(source)
		if err != nil || parsed.Scheme != "file" || parsed.Host != "" {
			return ""
		}
		source = parsed.Path
	}
	if strings.HasPrefix(source, "~/") {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
		source = filepath.Join(home, strings.TrimPrefix(source, "~/"))
	}
	if filegeneration.IsWindowsNamedPipePath(source) {
		// Preserve the lexical candidate so tracequery's platform-safe opener
		// publishes the explicit non-regular-source verdict. Silently dropping it
		// here would bypass the atomic set gate.
		return filepath.Clean(source)
	}
	var candidates []string
	if filepath.IsAbs(source) {
		candidates = []string{source}
	} else {
		if strings.TrimSpace(repoRoot) != "" {
			candidates = append(candidates, filepath.Join(repoRoot, source))
		}
		if cwd, err := os.Getwd(); err == nil && cwd != "" {
			candidates = append(candidates, filepath.Join(cwd, source))
		}
	}
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		info, err := os.Lstat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		if absolute, err := filepath.Abs(candidate); err == nil {
			candidate = filepath.Clean(absolute)
		}
		if canonical, err := filepath.EvalSymlinks(candidate); err == nil {
			candidate = filepath.Clean(canonical)
		}
		return candidate
	}
	if retainAbsent && len(candidates) > 0 {
		return filepath.Clean(candidates[0])
	}
	return ""
}

func typedNamedTraceAdmissionPathKey(path string) string {
	key := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(key)
	}
	return key
}
