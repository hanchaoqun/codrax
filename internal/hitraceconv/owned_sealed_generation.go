package hitraceconv

import (
	"fmt"
	"os"
	"strings"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

// inspectOwnedSealedGenerationPath binds a public pathname through the
// platform-safe read authority, never through path-only FileInfo identity.
// On Windows, os.Lstat returns a lazy file ID whose later os.SameFile reopen
// does not share DELETE; that is incompatible with our deliberately strict
// held publication handle. File.Stat here is descriptor-derived on every
// platform and the strong generation is recaptured on the same handle.
func inspectOwnedSealedGenerationPath(
	path string,
	expected os.FileInfo,
	expectedGeneration filegeneration.Identity,
	size int64,
) (identity filegeneration.Identity, resultErr error) {
	if strings.TrimSpace(path) == "" || expected == nil || size < 0 {
		return identity, fmt.Errorf("owned sealed generation inspection inputs are incomplete")
	}
	file, err := openOwnedSealedGenerationFile(path)
	if err != nil {
		return identity, err
	}
	defer func() {
		resultErr = traceDBJoinPreservingSingle(resultErr, file.Close())
	}()
	opened, err := file.Stat()
	if err != nil {
		return identity, err
	}
	if err := validateOwnedSealedGenerationPathBinding(path, opened); err != nil {
		return identity, err
	}
	identity, err = filegeneration.FromFile(file)
	if err != nil {
		return filegeneration.Identity{}, err
	}
	if !opened.Mode().IsRegular() || opened.Size() != size || !os.SameFile(expected, opened) ||
		!identity.Strong() || !identity.Mode().IsRegular() || identity.Size() != size ||
		(expectedGeneration.Initialized() && !expectedGeneration.SameVersion(identity)) {
		return filegeneration.Identity{}, fmt.Errorf("owned sealed generation path does not match its expected descriptor")
	}
	confirmed, err := filegeneration.FromFile(file)
	if err != nil {
		return filegeneration.Identity{}, fmt.Errorf("recapture owned sealed generation descriptor: %w", err)
	}
	if !identity.SameVersion(confirmed) {
		return filegeneration.Identity{}, fmt.Errorf("owned sealed generation descriptor changed during inspection")
	}
	return identity, nil
}
