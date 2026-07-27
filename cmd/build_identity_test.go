package cmd

import (
	"encoding/hex"
	"testing"
)

func TestResolveCodraxBuildIdentityKeepsRevisionAndArtifactAuthoritiesSeparate(t *testing.T) {
	previous := buildRevision
	buildRevision = "0123456789ab"
	defer func() { buildRevision = previous }()

	identity := resolveCodraxBuildIdentity()
	if identity.Revision != "0123456789ab" || identity.RevisionSource != "ldflags" {
		t.Fatalf("ldflags revision authority mismatch: %+v", identity)
	}
	if identity.ExecutableHashStatus != "available" || len(identity.ExecutableSHA256) != 64 {
		t.Fatalf("running executable fingerprint unavailable: %+v", identity)
	}
	if _, err := hex.DecodeString(identity.ExecutableSHA256); err != nil {
		t.Fatalf("running executable fingerprint is not hex SHA-256: %q", identity.ExecutableSHA256)
	}
}
