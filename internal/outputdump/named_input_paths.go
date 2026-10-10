package outputdump

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

// NamedInputPathsFromRequest inventories existing, explicitly located regular
// files. It neither infers intent nor attaches material. All file kinds share
// the same lexical discovery; content readers must establish their own bounds.
func NamedInputPathsFromRequest(request, repoRoot string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	if strings.TrimSpace(repoRoot) == "" {
		repoRoot, _ = os.Getwd()
	}
	var out []string
	seen := map[string]bool{}
	add := func(raw string) {
		if len(out) >= limit {
			return
		}
		path := normalizeRequestRuntimeArtifactPath(raw)
		if !requestPathHasExplicitLocator(path) || filegeneration.IsWindowsNamedPipePath(path) {
			return
		}
		if strings.HasPrefix(path, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return
			}
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(repoRoot, path)
		}
		path, err := filepath.Abs(filepath.Clean(path))
		if err != nil || seen[path] {
			return
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return
		}
		seen[path] = true
		out = append(out, path)
	}
	visitRequestPathTokens(request, add)
	return out
}

func visitRequestPathTokens(request string, add func(string)) {
	// Quoted paths precede split tokens, so a path with spaces consumes one
	// source budget slot and cannot be displaced by an existing partial path.
	tokenText := []byte(request)
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
			for i := left; i <= right; i++ {
				tokenText[i] = ' '
			}
			start = right + 1
		}
	}
	for _, token := range requestPathTokenRE.FindAllString(string(tokenText), -1) {
		add(token)
	}
}
