package attachment

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

// TraceMaterial binds a bounded, immutable model preview to complete physical
// query material. It is an in-process attachment receipt, never a model field
// or a serialized permission. Restoring a preview alone cannot restore it.
// No scheduling or causal capability is conferred by this transport receipt.
type TraceMaterial struct {
	sourcePath        string
	queryPath         string
	preview           string
	bindings          map[string]filegeneration.Identity
	selfContainedText bool
}

// BindTraceMaterial consumes the generations captured by the input preparer.
// Capture must precede its reads/conversion; recapturing after a source change
// here would bless a preview from a different generation. Copy the map so the
// caller cannot mutate the receipt after publication.
func BindTraceMaterial(sourcePath, queryPath, preview string, bindings map[string]filegeneration.Identity) (*TraceMaterial, error) {
	for _, path := range []string{sourcePath, queryPath} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return nil, fmt.Errorf("trace material requires a clean absolute path: %q", path)
		}
		if err := ValidateSourceLabel(path); err != nil {
			return nil, err
		}
		if id, ok := bindings[path]; !ok || !id.Initialized() || !id.Mode().IsRegular() {
			return nil, fmt.Errorf("trace material missing regular-file generation for %q", path)
		}
	}
	if strings.TrimSpace(preview) == "" {
		return nil, fmt.Errorf("trace material preview is empty")
	}
	if err := ValidateTextString(KindTrace, queryPath, preview, false); err != nil {
		return nil, err
	}
	if err := ValidateSingleTraceAttachmentProvenance(preview); err != nil {
		return nil, err
	}
	m := &TraceMaterial{sourcePath: sourcePath, queryPath: queryPath, preview: preview, bindings: make(map[string]filegeneration.Identity, len(bindings))}
	for path, id := range bindings {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || !id.Initialized() || !id.Mode().IsRegular() {
			return nil, fmt.Errorf("trace material invalid member generation for %q", path)
		}
		m.bindings[path] = id
	}
	if err := m.Validate(context.Background(), preview); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *TraceMaterial) Preview() string    { return m.preview }
func (m *TraceMaterial) SourcePath() string { return m.sourcePath }
func (m *TraceMaterial) QueryPath() string  { return m.queryPath }

// BindCompleteTextTraceMaterial is reserved for a preparer's completed,
// untruncated read of one plain text source. Unlike a converted trace, bundle,
// or bounded preview, these immutable bytes may retain legacy text snapshot
// semantics. Ordinary BindTraceMaterial never grants this permission.
func BindCompleteTextTraceMaterial(sourcePath, preview string, bindings map[string]filegeneration.Identity) (*TraceMaterial, error) {
	if len(bindings) != 1 || strings.HasSuffix(strings.ToLower(sourcePath), ".tracebundle.json") {
		return nil, fmt.Errorf("complete text material must be one non-bundle source")
	}
	header := "# codrax-source: " + sourcePath + "\n"
	id, ok := bindings[sourcePath]
	if !ok || !strings.HasPrefix(preview, header) || int64(len(preview)-len(header)) != id.Size() {
		return nil, fmt.Errorf("complete text material must retain the entire source body")
	}
	m, err := BindTraceMaterial(sourcePath, sourcePath, preview, bindings)
	if err != nil {
		return nil, err
	}
	m.selfContainedText = true
	return m, nil
}

// SelfContainedText is an in-process, producer-minted completeness fact. It
// does not survive JSON serialization or authorize a new physical source.
func (m *TraceMaterial) SelfContainedText() bool { return m != nil && m.selfContainedText }

// MatchesPath recognizes only the two paths already bound by preparation,
// including canonical symlink spellings. This is routing, not permission:
// consumers must still Validate the receipt before and after reading.
func (m *TraceMaterial) MatchesPath(path string) bool {
	if m == nil || !filepath.IsAbs(path) {
		return false
	}
	equal := func(a, b string) bool { return a == b || runtime.GOOS == "windows" && strings.EqualFold(a, b) }
	path = filepath.Clean(path)
	for _, bound := range []string{m.sourcePath, m.queryPath} {
		if equal(path, bound) {
			return true
		}
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil {
			continue
		}
		boundCanonical, err := filepath.EvalSymlinks(bound)
		if err == nil && equal(canonical, boundCanonical) {
			return true
		}
	}
	return false
}

// Validate rejects stale source, derived output, bundle members or a preview
// not belonging to this attachment. All consumers fail closed rather than
// silently falling back to a truncated or previous attached_trace.txt blob.
func (m *TraceMaterial) Validate(ctx context.Context, preview string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if m == nil || m.queryPath == "" || len(m.bindings) == 0 || preview != m.preview {
		return fmt.Errorf("trace attachment no longer matches its prepared material; attach the source again")
	}
	paths := make([]string, 0, len(m.bindings))
	for path := range m.bindings {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := filegeneration.FromPath(path)
		if err != nil {
			return fmt.Errorf("prepared trace member unavailable %q: %w", path, err)
		}
		if !m.bindings[path].SameVersion(current) {
			return fmt.Errorf("prepared trace member changed %q; attach the source again", path)
		}
	}
	return ctx.Err()
}
