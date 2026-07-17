package tracequery

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

// traceSourceUniverseEntry is the immutable generation ledger for one
// physical member selected into a query build. Content readers still open
// their own descriptor, but must prove that descriptor matches this identity
// before consuming a byte.
type traceSourceUniverseEntry struct {
	path     string
	identity traceFileIdentity
}

type traceSourceUniverse struct {
	entries    []traceSourceUniverseEntry
	totalBytes int64
	cacheToken string
}

func (u traceSourceUniverse) entry(path string) (traceSourceUniverseEntry, bool) {
	path = canonicalTraceIndexPath(path)
	for _, entry := range u.entries {
		if entry.path == path {
			return entry, true
		}
	}
	return traceSourceUniverseEntry{}, false
}

func (u traceSourceUniverse) validate(ctx context.Context, manifest *tracebundle.Snapshot) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if manifest != nil {
		if err := manifest.Validate(); err != nil {
			return fmt.Errorf("revalidate trace bundle manifest: %w", err)
		}
	}
	for _, entry := range u.entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if manifest != nil && entry.path == manifest.Path() {
			continue
		}
		current, err := filegeneration.FromPath(entry.path)
		if err != nil {
			return fmt.Errorf("revalidate trace source artifact %s: %w", entry.path, err)
		}
		if !entry.identity.SameVersion(current) {
			return fmt.Errorf("trace source artifact changed while the index was being built: %s", entry.path)
		}
	}
	return nil
}

// traceIndexSelection freezes every pathname decision made for one build.
// In particular, a sibling manifest is decoded once and the original
// requested systrace remains attached until final universe validation.
type traceIndexSelection struct {
	requestedPath   string
	indexPath       string
	promoted        bool
	manifest        *tracebundle.Snapshot
	bundle          traceBundleFile
	bundleSet       bool
	artifactSpecs   []traceArtifactSpec
	caveats         []string
	universe        traceSourceUniverse
	indexIdentity   traceFileIdentity
	allowDigestScan bool
}

func (s *traceIndexSelection) close() error {
	if s == nil || s.manifest == nil {
		return nil
	}
	err := s.manifest.Close()
	s.manifest = nil
	return err
}

func (s *traceIndexSelection) closeAfter(prior error) error {
	closeErr := s.close()
	if prior == nil {
		return closeErr
	}
	if closeErr == nil {
		return prior
	}
	return errors.Join(prior, closeErr)
}

func (s *traceIndexSelection) validate(ctx context.Context) error {
	if s == nil || s.indexPath == "" || !s.indexIdentity.Initialized() {
		return fmt.Errorf("trace index selection is uninitialized")
	}
	if s.promoted && !traceSelectionContainsRequestedSystrace(s) {
		return fmt.Errorf("trace bundle no longer contains the originally requested systrace")
	}
	return s.universe.validate(ctx, s.manifest)
}

func (s *traceIndexSelection) finish(ctx context.Context, idx *Index) error {
	return s.closeAfter(errors.Join(s.validateIndex(idx), s.validate(ctx)))
}

func (s *traceIndexSelection) validateIndex(idx *Index) error {
	if s == nil || idx == nil {
		return fmt.Errorf("trace index selection produced no index")
	}
	if canonicalTraceIndexPath(idx.Path) != s.indexPath {
		return fmt.Errorf("trace index path differs from its frozen selection")
	}
	if idx.Size != s.universe.totalBytes {
		return fmt.Errorf("trace index source bytes differ from its frozen universe: got=%d want=%d", idx.Size, s.universe.totalBytes)
	}
	for _, entry := range s.universe.entries {
		if s.manifest != nil && entry.path == s.manifest.Path() {
			continue
		}
		source, ok := traceArtifactSourceForPath(idx.TraceArtifacts, entry.path)
		if !ok {
			return fmt.Errorf("trace index omitted frozen artifact %s", entry.path)
		}
		if !source.sourceIdentity.Initialized() || !source.sourceIdentity.SameVersion(entry.identity) {
			return fmt.Errorf("trace index artifact generation differs from frozen source %s", entry.path)
		}
	}
	if s.promoted {
		source, ok := traceArtifactSourceForPath(idx.TraceArtifacts, s.requestedPath)
		if !ok || source.Kind != "systrace" || !source.CausalCompatible {
			return fmt.Errorf("trace index did not preserve the originally requested causal systrace")
		}
	}
	return nil
}

func traceSelectionContainsRequestedSystrace(selection *traceIndexSelection) bool {
	if selection == nil || !selection.promoted || selection.requestedPath == "" {
		return selection != nil
	}
	for _, spec := range selection.artifactSpecs {
		if spec.source.Kind == "systrace" {
			return spec.source.CausalCompatible && canonicalTraceIndexPath(spec.source.SourcePath) == selection.requestedPath
		}
	}
	return false
}

func resolveTraceIndexSelection(ctx context.Context, requested string) (*traceIndexSelection, error) {
	return resolveTraceIndexSelectionWithPolicy(ctx, requested, true)
}

func resolveTraceIndexSelectionWithPolicy(ctx context.Context, requested string, allowDigestScan bool) (*traceIndexSelection, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	requested = canonicalTraceIndexPath(strings.TrimSpace(requested))
	if requested == "" {
		return nil, fmt.Errorf("trace path is empty")
	}
	if traceSourcePathIsBlockingNamespace(requested) {
		return nil, fmt.Errorf("trace source is not a regular file: named-pipe path=%q", requested)
	}
	selection := &traceIndexSelection{requestedPath: requested, indexPath: requested, allowDigestScan: allowDigestScan}
	explicitBundle := traceBundlePath(requested)
	if explicitBundle {
		// A tracebundle is a bounded, strongly generation-bound JSON control
		// document, not event text. Preserve its dedicated size/UTF-8/schema
		// verdicts instead of misclassifying a malformed manifest as a binary
		// capture that trace convert could repair.
		manifest, bundle, err := openTraceBundleSnapshot(ctx, requested)
		if err != nil {
			return nil, err
		}
		selection.manifest = manifest
		selection.bundleSet = true
		if bundle.schemaMode == traceBundleSchemaV2 {
			selection.bundle = bundle
			selection.artifactSpecs = traceBundleArtifactSpecs(requested, bundle)
		} else {
			selection.artifactSpecs, err = legacySingleTraceBundleSpecs(requested, bundle)
			if err != nil {
				return nil, selection.closeAfter(err)
			}
			// Legacy metadata was never bound to the selected child generation.
			// Keep only the physical single-file view and one explicit disclosure.
			selection.bundle = traceBundleFile{schemaMode: traceBundleSchemaLegacy}
			selection.caveats = append(selection.caveats, "tracebundle_legacy_unbound=true; only the explicit bundle-local systrace was read; legacy provider, coverage, clock, capability, and caveat metadata was not trusted")
		}
	} else {
		// Admit the explicitly requested event/perf text before any optional
		// sibling discovery. A bad sibling must never mask the requested file's
		// content verdict.
		requestedFile, _, err := openTraceSourceRegularContextPolicy(ctx, requested, allowDigestScan)
		if err != nil {
			return nil, err
		}
		if err := requestedFile.Close(); err != nil {
			return nil, fmt.Errorf("close preflighted requested trace source %s: %w", requested, err)
		}
	}

	if !explicitBundle && !strings.HasSuffix(strings.ToLower(requested), ".perftrace") {
		candidate := traceSiblingBundleCandidate(requested)
		if candidate != "" {
			manifest, bundle, err := openTraceBundleSnapshot(ctx, candidate)
			switch {
			case err == nil:
				if bundle.schemaMode != traceBundleSchemaV2 {
					if closeErr := manifest.Close(); closeErr != nil {
						return nil, closeErr
					}
					break
				}
				specs := traceBundleArtifactSpecs(candidate, bundle)
				if traceSpecsContainRequestedSystrace(specs, requested) {
					selection.indexPath = candidate
					selection.promoted = true
					selection.manifest = manifest
					selection.bundle = bundle
					selection.bundleSet = true
					selection.artifactSpecs = specs
				} else {
					if closeErr := manifest.Close(); closeErr != nil {
						return nil, closeErr
					}
				}
			case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
				return nil, err
			default:
				// A sibling bundle is optional metadata. Any intake, shape, or
				// generation failure must leave the explicitly requested trace
				// usable instead of allowing stale metadata to hijack it. This is
				// a per-call frozen decision, not a negative cache entry: every
				// later call reruns this resolver, and a newly valid manifest selects
				// a different source universe/cache token before any cache lookup.
			}
		}
	}

	if !selection.bundleSet {
		// A same-basename physical sibling is not a capture identity. Without a
		// V2 manifest the user's explicit file is the complete source universe.
		selection.artifactSpecs = nil
	}

	universe, err := captureTraceSourceUniverse(ctx, selection)
	if err != nil {
		// An implicitly selected sibling is never allowed to make the direct
		// request fail. Close it and retry the physical request without that
		// manifest. Explicit bundle failures remain hard errors.
		if selection.promoted {
			if closeErr := selection.close(); closeErr != nil {
				return nil, errors.Join(err, closeErr)
			}
			selection = &traceIndexSelection{requestedPath: requested, indexPath: requested, allowDigestScan: allowDigestScan}
			universe, err = captureTraceSourceUniverse(ctx, selection)
		}
		if err != nil {
			return nil, selection.closeAfter(err)
		}
	}
	selection.universe = universe
	entry, ok := universe.entry(selection.indexPath)
	if !ok {
		return nil, selection.closeAfter(fmt.Errorf("trace index selection omitted its entry artifact"))
	}
	selection.indexIdentity = entry.identity
	if err := selection.validate(ctx); err != nil {
		return nil, selection.closeAfter(err)
	}
	return selection, nil
}

func openTraceBundleSnapshot(ctx context.Context, path string) (*tracebundle.Snapshot, traceBundleFile, error) {
	snapshot, err := tracebundle.Open(ctx, path)
	if err != nil {
		return nil, traceBundleFile{}, err
	}
	var bundle traceBundleFile
	if err := snapshot.Decode(&bundle); err != nil {
		parseErr := fmt.Errorf("parse trace bundle %s: %w", path, err)
		return nil, traceBundleFile{}, errors.Join(parseErr, snapshot.Close())
	}
	if err := classifyTraceBundleSchema(path, &bundle); err != nil {
		return nil, traceBundleFile{}, errors.Join(err, snapshot.Close())
	}
	return snapshot, bundle, nil
}

func traceSiblingBundleCandidate(path string) string {
	base := traceArtifactBase(path)
	if base == "" {
		return ""
	}
	return canonicalTraceIndexPath(base + ".tracebundle.json")
}

func traceSpecsContainRequestedSystrace(specs []traceArtifactSpec, requested string) bool {
	requested = canonicalTraceIndexPath(requested)
	for _, spec := range specs {
		if spec.source.Kind == "systrace" {
			return spec.source.CausalCompatible && canonicalTraceIndexPath(spec.source.SourcePath) == requested
		}
	}
	return false
}

func captureTraceSourceUniverse(ctx context.Context, selection *traceIndexSelection) (traceSourceUniverse, error) {
	if selection == nil {
		return traceSourceUniverse{}, fmt.Errorf("trace index selection is nil")
	}
	paths := make([]string, 0, len(selection.artifactSpecs)+1)
	if selection.manifest != nil {
		paths = append(paths, selection.manifest.Path())
	}
	for _, spec := range selection.artifactSpecs {
		paths = append(paths, spec.source.SourcePath)
	}
	if len(paths) == 0 {
		paths = append(paths, selection.indexPath)
	}

	strongRequired := selection.manifest != nil || len(paths) > 1
	seen := make(map[string]struct{}, len(paths))
	physicalSeen := make(map[string]string, len(paths))
	universe := traceSourceUniverse{entries: make([]traceSourceUniverseEntry, 0, len(paths))}
	var key strings.Builder
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return traceSourceUniverse{}, err
		}
		path = canonicalTraceIndexPath(path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}

		var identity traceFileIdentity
		if selection.manifest != nil && path == selection.manifest.Path() {
			identity = selection.manifest.Identity()
		} else {
			file, openedIdentity, err := openTraceSourceRegularContextPolicy(ctx, path, selection.allowDigestScan)
			if err != nil {
				return traceSourceUniverse{}, fmt.Errorf("open trace source artifact %s: %w", path, err)
			}
			if spec, ok := selection.traceArtifactSpec(path); ok && selection.bundle.schemaMode == traceBundleSchemaV2 && !spec.provenanceBound {
				return traceSourceUniverse{}, errors.Join(
					fmt.Errorf("trace bundle V2 child %s has no frozen provenance binding", path),
					file.Close(),
				)
			} else if ok && spec.provenanceBound {
				attestErr := attestTraceBundleChild(
					ctx, file, openedIdentity, spec.provenanceBytes, spec.provenanceSHA256, selection.allowDigestScan,
				)
				if attestErr == nil {
					attestErr = validateTraceFileIdentityAfterRead(file, openedIdentity, "trace bundle child digest attestation")
				}
				if attestErr != nil {
					return traceSourceUniverse{}, errors.Join(
						fmt.Errorf("verify trace bundle child %s: %w", path, attestErr),
						file.Close(),
					)
				}
			}
			if closeErr := file.Close(); closeErr != nil {
				return traceSourceUniverse{}, fmt.Errorf("close trace source artifact %s after identity capture: %w", path, closeErr)
			}
			identity = openedIdentity
		}
		if !identity.Initialized() || identity.Size() < 0 {
			return traceSourceUniverse{}, fmt.Errorf("trace source artifact has no valid generation identity: %s", path)
		}
		if strongRequired && !identity.Strong() {
			return traceSourceUniverse{}, fmt.Errorf("trace composite artifact has no strong generation identity: %s", path)
		}
		if selection.bundle.schemaMode == traceBundleSchemaV2 {
			physicalKey := identity.CacheToken()
			if prior, duplicate := physicalSeen[physicalKey]; duplicate && prior != path {
				return traceSourceUniverse{}, fmt.Errorf("trace bundle members %s and %s resolve to one physical generation", prior, path)
			}
			physicalSeen[physicalKey] = path
		}
		if identity.Size() > math.MaxInt64-universe.totalBytes {
			return traceSourceUniverse{}, fmt.Errorf("trace source universe byte size overflow")
		}
		universe.totalBytes += identity.Size()
		universe.entries = append(universe.entries, traceSourceUniverseEntry{path: path, identity: identity})
		fmt.Fprintf(&key, "%d:%s:%s|", len(path), path, identity.CacheToken())
	}
	if len(universe.entries) == 0 {
		return traceSourceUniverse{}, fmt.Errorf("trace source universe is empty")
	}
	universe.cacheToken = key.String()
	return universe, nil
}

func (s *traceIndexSelection) traceArtifactSpec(path string) (traceArtifactSpec, bool) {
	if s == nil {
		return traceArtifactSpec{}, false
	}
	path = canonicalTraceIndexPath(path)
	for _, spec := range s.artifactSpecs {
		if canonicalTraceIndexPath(spec.source.SourcePath) == path {
			return spec, true
		}
	}
	return traceArtifactSpec{}, false
}
