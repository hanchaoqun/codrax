package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"runtime/debug"
	"strings"
)

type codraxBuildIdentity struct {
	Revision             string `json:"revision"`
	RevisionSource       string `json:"revision_source"`
	ExecutableSHA256     string `json:"executable_sha256,omitempty"`
	ExecutableHashStatus string `json:"executable_hash_status"`
}

// resolveCodraxBuildIdentity keeps revision and artifact identity separate.
// A source archive without .git cannot truthfully recover a commit revision,
// but the running executable can still be identified exactly for customer
// replay comparison.
func resolveCodraxBuildIdentity() codraxBuildIdentity {
	identity := codraxBuildIdentity{
		Revision:             "unknown",
		RevisionSource:       "unavailable",
		ExecutableHashStatus: "unavailable",
	}
	if revision := strings.TrimSpace(buildRevision); revision != "" &&
		!strings.HasPrefix(strings.ToLower(revision), "unknown") {
		identity.Revision = revision
		identity.RevisionSource = "ldflags"
	} else if info, ok := debug.ReadBuildInfo(); ok {
		revision := ""
		modified := false
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = strings.TrimSpace(setting.Value)
			case "vcs.modified":
				modified = setting.Value == "true"
			}
		}
		if revision != "" {
			if modified {
				revision += "-dirty"
			}
			identity.Revision = revision
			identity.RevisionSource = "go_build_info"
		}
	}

	executable, err := os.Executable()
	if err != nil {
		identity.ExecutableHashStatus = "executable_path_unavailable"
		return identity
	}
	file, err := os.Open(executable)
	if err != nil {
		identity.ExecutableHashStatus = "executable_open_failed"
		return identity
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		identity.ExecutableHashStatus = "executable_read_failed"
		return identity
	}
	identity.ExecutableSHA256 = hex.EncodeToString(hash.Sum(nil))
	identity.ExecutableHashStatus = "available"
	return identity
}
