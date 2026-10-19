package attachment

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

// TraceSelection is a request coordinate, not a receipt or permission. Path
// always names a physical file; ArchiveMember never becomes a filesystem path.
type TraceSelection struct {
	Path          string
	ArchiveMember string
}

func ValidateTraceArchiveMember(member string) error {
	if member == "" || strings.TrimSpace(member) != member || !utf8.ValidString(member) || strings.ContainsRune(member, 0) ||
		strings.Contains(member, "\\") || strings.HasSuffix(member, "/") || path.IsAbs(member) || path.Clean(member) != member ||
		member == "." || member == ".." || strings.HasPrefix(member, "../") ||
		(len(member) >= 2 && ((member[0] >= 'a' && member[0] <= 'z') || (member[0] >= 'A' && member[0] <= 'Z')) && member[1] == ':') {
		return fmt.Errorf("archive member must be one exact canonical slash-separated name")
	}
	return nil
}

// WithArchiveSelection records a producer-validated selection inside an
// already bound material. It cannot renew generations or add query authority.
func (m *TraceMaterial) WithArchiveSelection(ctx context.Context, member string, explicit bool) (*TraceMaterial, error) {
	if err := ValidateTraceArchiveMember(member); err != nil {
		return nil, err
	}
	if m == nil || m.sourcePath == m.queryPath {
		return nil, fmt.Errorf("archive selection requires prepared derived material")
	}
	if err := m.Validate(ctx, m.preview); err != nil {
		return nil, err
	}
	copy := *m
	copy.archiveMember, copy.explicitArchiveSelection = member, explicit
	return &copy, nil
}

func (m *TraceMaterial) ArchiveMember() string {
	if m == nil {
		return ""
	}
	return m.archiveMember
}

func (m *TraceMaterial) ExplicitArchiveSelection() bool {
	return m != nil && m.explicitArchiveSelection
}

// MatchesSelection resolves only the requested original/member coordinate.
// Supplying a member against a derived path is not a new preparation request.
func (m *TraceMaterial) MatchesSelection(original, member string) bool {
	if member == "" {
		return m.MatchesPath(original)
	}
	return m != nil && m.archiveMember == member && m.matchesOriginalSource(original)
}

// ValidateOriginalSource rechecks only the physical source and its producer
// sidecar rule. One selection's damaged derived file must not block a healthy
// sibling selection, but changing members must never evade source generation.
func (m *TraceMaterial) ValidateOriginalSource(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if m == nil || m.sourcePath == "" {
		return fmt.Errorf("prepared source is missing")
	}
	if m.sourceCheck != nil {
		if err := m.sourceCheck(ctx); err != nil {
			return err
		}
	}
	expected, ok := m.bindings[m.sourcePath]
	if !ok {
		return fmt.Errorf("prepared source generation is missing")
	}
	current, err := filegeneration.FromPath(m.sourcePath)
	if err != nil {
		return err
	}
	if !expected.SameVersion(current) {
		return fmt.Errorf("prepared trace source changed %q; start a new run", m.sourcePath)
	}
	return ctx.Err()
}

func (m *TraceMaterial) matchesOriginalSource(original string) bool {
	if m == nil || !filepath.IsAbs(original) || m.Validate(context.Background(), m.preview) != nil {
		return false
	}
	canonical, err := filepath.EvalSymlinks(original)
	if err != nil {
		return false
	}
	bound, err := filepath.EvalSymlinks(m.sourcePath)
	if err != nil || canonical != bound {
		return false
	}
	current, err := filegeneration.FromPath(original)
	return err == nil && m.bindings[m.sourcePath].SameVersion(current)
}
