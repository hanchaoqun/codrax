package tracequery

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

type traceBundleSchemaMode uint8

const (
	traceBundleSchemaLegacy traceBundleSchemaMode = iota
	traceBundleSchemaV2
)

func classifyTraceBundleSchema(bundlePath string, bundle *traceBundleFile) error {
	if bundle == nil {
		return fmt.Errorf("trace bundle schema: manifest is nil")
	}
	bundle.schemaMode = traceBundleSchemaLegacy
	if bundle.Schema == "" {
		if bundle.CaptureID != "" || traceBundleHasV2OnlyChildField(*bundle) {
			return fmt.Errorf("trace bundle %s mixes V2 provenance fields with a missing schema", bundlePath)
		}
		return nil
	}
	if bundle.Schema != tracebundle.SchemaV2 {
		return fmt.Errorf("trace bundle %s uses unsupported schema %q", bundlePath, bundle.Schema)
	}
	if err := validateTraceBundleArchiveProvenance(bundlePath, bundle.ArchiveProvenance); err != nil {
		return err
	}

	members := make([]tracebundle.CaptureMember, 0, len(bundle.Artifacts))
	systraceChildren := make(map[string]int)
	perfChildren := make(map[string]struct{})
	seen := make(map[string]struct{}, len(bundle.Artifacts))
	for index := range bundle.Artifacts {
		artifact := &bundle.Artifacts[index]
		// Schema V2 freezes wire tokens exactly. In particular, a padded
		// causal type must not be normalized into a different, authoritative
		// capture member. Keep legacy's tolerant reader unchanged below.
		if strings.TrimSpace(artifact.Type) != artifact.Type {
			return fmt.Errorf("trace bundle %s artifact %d type must be exact and unpadded: got %q", bundlePath, index, artifact.Type)
		}
		kind, causal, err := traceBundleCausalKind(artifact.Type, artifact.Path)
		if err != nil {
			return fmt.Errorf("trace bundle %s artifact %d: %w", bundlePath, index, err)
		}
		if !causal {
			continue
		}
		wirePath, err := strictTraceBundleRelativePath(artifact.Path)
		if err != nil {
			return fmt.Errorf("trace bundle %s artifact %d: %w", bundlePath, index, err)
		}
		if _, duplicate := seen[wirePath]; duplicate {
			return fmt.Errorf("trace bundle %s declares duplicate causal child %q", bundlePath, wirePath)
		}
		seen[wirePath] = struct{}{}
		if artifact.Bytes == nil {
			return fmt.Errorf("trace bundle %s causal child %q is missing bytes", bundlePath, wirePath)
		}
		if *artifact.Bytes < 0 {
			return fmt.Errorf("trace bundle %s causal child %q has negative bytes", bundlePath, wirePath)
		}
		if err := tracebundle.ValidateSHA256(artifact.SHA256); err != nil {
			return fmt.Errorf("trace bundle %s causal child %q has invalid sha256: %w", bundlePath, wirePath, err)
		}
		members = append(members, tracebundle.CaptureMember{
			Type: kind, Path: wirePath, Bytes: *artifact.Bytes, SHA256: artifact.SHA256,
		})
		switch kind {
		case "systrace":
			systraceChildren[wirePath]++
		case "perftrace":
			perfChildren[wirePath] = struct{}{}
		}
	}

	if bundle.Systrace != "" {
		wirePath, err := strictTraceBundleRelativePath(bundle.Systrace)
		if err != nil {
			return fmt.Errorf("trace bundle %s primary systrace: %w", bundlePath, err)
		}
		if systraceChildren[wirePath] != 1 {
			return fmt.Errorf("trace bundle %s primary systrace %q must reference exactly one bound systrace child", bundlePath, wirePath)
		}
	} else if len(systraceChildren) > 0 {
		return fmt.Errorf("trace bundle %s has a bound systrace child but no primary systrace reference", bundlePath)
	}

	for index, alignment := range bundle.PerfClockAlignments {
		wirePath, err := strictTraceBundleRelativePath(alignment.ArtifactPath)
		if err != nil {
			return fmt.Errorf("trace bundle %s clock alignment %d: %w", bundlePath, index, err)
		}
		if _, ok := perfChildren[wirePath]; !ok {
			return fmt.Errorf("trace bundle %s clock alignment %d references unbound perf child %q", bundlePath, index, wirePath)
		}
	}

	wantCaptureID, err := tracebundle.CaptureID(members)
	if err != nil {
		return fmt.Errorf("trace bundle %s capture identity: %w", bundlePath, err)
	}
	if bundle.CaptureID != wantCaptureID {
		return fmt.Errorf("trace bundle %s capture_id mismatch: got=%q want=%q", bundlePath, bundle.CaptureID, wantCaptureID)
	}
	bundle.schemaMode = traceBundleSchemaV2
	return nil
}

func traceBundleHasV2OnlyChildField(bundle traceBundleFile) bool {
	if bundle.ArchiveProvenance != nil {
		return true
	}
	for _, artifact := range bundle.Artifacts {
		if strings.TrimSpace(artifact.SHA256) != "" {
			return true
		}
	}
	return false
}

func validateTraceBundleArchiveProvenance(bundlePath string, provenance *traceBundleArchiveProvenance) error {
	if provenance == nil {
		return nil
	}
	fail := func(format string, args ...any) error {
		return fmt.Errorf("trace bundle %s archive_provenance: "+format, append([]any{bundlePath}, args...)...)
	}
	if provenance.Format != "zip" {
		return fail("format must be exact zip: got %q", provenance.Format)
	}
	if provenance.ArchiveBytes <= 0 || provenance.MemberBytes <= 0 {
		return fail("byte sizes must be positive: archive=%d member=%d", provenance.ArchiveBytes, provenance.MemberBytes)
	}
	if err := tracebundle.ValidateSHA256(provenance.ArchiveSHA256); err != nil {
		return fail("invalid archive_sha256: %v", err)
	}
	if err := tracebundle.ValidateSHA256(provenance.MemberSHA256); err != nil {
		return fail("invalid member_sha256: %v", err)
	}
	if err := tracebundle.ValidateCapturePath(provenance.Member); err != nil {
		return fail("invalid member: %v", err)
	}
	extension := strings.ToLower(path.Ext(provenance.Member))
	if extension != ".sys" && extension != ".htrace" {
		return fail("member must use .sys or .htrace: got %q", provenance.Member)
	}
	if provenance.Selection != "unique_candidate" && provenance.Selection != "explicit_member" {
		return fail("selection is not in the closed set: got %q", provenance.Selection)
	}
	return nil
}

func traceBundleCausalKind(rawType, rawPath string) (kind string, causal bool, err error) {
	rawType = strings.TrimSpace(rawType)
	suffixPerf := strings.EqualFold(path.Ext(strings.ReplaceAll(rawPath, "\\", "/")), ".perftrace")
	switch rawType {
	case "systrace":
		if suffixPerf {
			return "", true, fmt.Errorf("type=systrace conflicts with .perftrace child path")
		}
		return "systrace", true, nil
	case "perftrace":
		return "perftrace", true, nil
	default:
		if suffixPerf {
			return "", true, fmt.Errorf(".perftrace child requires exact type=perftrace")
		}
		return "", false, nil
	}
}

func strictTraceBundleRelativePath(raw string) (string, error) {
	if err := tracebundle.ValidateCapturePath(raw); err != nil {
		return "", err
	}
	return path.Clean(raw), nil
}

func strictTraceBundleResolvedPath(bundlePath, wirePath string) (string, error) {
	wirePath, err := strictTraceBundleRelativePath(wirePath)
	if err != nil {
		return "", err
	}
	baseDir := filepath.Dir(bundlePath)
	return canonicalTraceIndexPath(filepath.Join(baseDir, filepath.FromSlash(wirePath))), nil
}

func legacySingleTraceBundleSpecs(bundlePath string, bundle traceBundleFile) ([]traceArtifactSpec, error) {
	paths := make(map[string]struct{})
	addSystrace := func(raw string) error {
		if raw == "" {
			return nil
		}
		wirePath, err := strictTraceBundleRelativePath(raw)
		if err != nil {
			return err
		}
		paths[wirePath] = struct{}{}
		return nil
	}
	if err := addSystrace(bundle.Systrace); err != nil {
		return nil, fmt.Errorf("legacy trace bundle primary systrace: %w", err)
	}
	for index, artifact := range bundle.Artifacts {
		kind, causal, err := traceBundleCausalKind(artifact.Type, artifact.Path)
		if err != nil {
			return nil, fmt.Errorf("legacy trace bundle artifact %d: %w", index, err)
		}
		if !causal {
			continue
		}
		if kind != "systrace" {
			return nil, fmt.Errorf("legacy trace bundle contains an unbound perf child; select the physical .perftrace directly")
		}
		if err := addSystrace(artifact.Path); err != nil {
			return nil, fmt.Errorf("legacy trace bundle artifact %d: %w", index, err)
		}
	}
	if len(paths) != 1 {
		return nil, fmt.Errorf("legacy trace bundle must contain exactly one bundle-local systrace and no perf child; select one physical child directly")
	}
	ordered := make([]string, 0, 1)
	for wirePath := range paths {
		ordered = append(ordered, wirePath)
	}
	sort.Strings(ordered)
	resolved, err := strictTraceBundleResolvedPath(bundlePath, ordered[0])
	if err != nil {
		return nil, err
	}
	return finalizeTraceArtifactSpecs([]traceArtifactSpec{{source: TraceArtifactSource{
		SourcePath: resolved,
		Kind:       "systrace",
		TimeDomain: inferTraceArtifactTimeDomain(resolved, "systrace"),
	}}}), nil
}
