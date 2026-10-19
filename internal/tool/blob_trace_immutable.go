package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/immutablefile"
	"github.com/hanchaoqun/codrax/internal/logging"
)

// Native query results are completed bytes, not a query cache or an authority
// carrier. Repeated producers may reuse identical material without changing a
// prior attempt's file generation. Other tools and attached-input repair retain
// their existing policies. Never repair/overwrite a conflicting result file.
func storeImmutableTraceResult(workDir, nameHint, output string) string {
	if strings.TrimSpace(workDir) == "" {
		return ""
	}
	path, err := publishImmutableTraceResult(workDir, nameHint, output)
	if err != nil {
		logging.Warning("[trace_query] immutable result publication failed: %v", err)
		return ""
	}
	return path
}

func immutableTraceResultName(nameHint, output string) string {
	ext := filepath.Ext(nameHint)
	if ext == "" {
		ext = ".txt"
	}
	// Both components are leaves, never caller-controlled path segments.
	ext = "." + sanitizeToolName(strings.TrimPrefix(ext, "."))
	stem := sanitizeToolName(strings.TrimSuffix(firstNonEmptyBlobName(nameHint, "trace_query"), filepath.Ext(nameHint)))
	if len(stem) > 120 {
		stem = stem[:120]
	}
	if len(ext) > 20 {
		ext = ".txt"
	}
	sum := sha256.Sum256([]byte(output))
	return stem + "-" + hex.EncodeToString(sum[:]) + ext
}

func publishImmutableTraceResult(workDir, nameHint, output string) (string, error) {
	return immutablefile.Publish(workDir, immutableTraceResultName(nameHint, output), []byte(output))
}

func immutableTraceResultMatches(root *os.Root, leaf, output string) error {
	return immutablefile.Matches(root, leaf, []byte(output))
}
