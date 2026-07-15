package filegeneration

import "strings"

// IsWindowsNamedPipePath recognizes the Win32/NT pathname spellings that can
// make an ordinary file open wait for a pipe server. It is platform-neutral so
// every release host can pin the Windows lexical admission gate.
func IsWindowsNamedPipePath(path string) bool {
	p := strings.ToLower(strings.ReplaceAll(path, "/", `\`))
	for _, prefix := range []string{
		`\\.\pipe`,
		`\\?\pipe`,
		`\\?\globalroot\device\namedpipe`,
		`\\.\globalroot\device\namedpipe`,
	} {
		if windowsPathHasComponentPrefix(p, prefix) {
			return true
		}
	}
	if windowsPathHasComponentPrefix(p, `\\?\unc`) {
		return windowsUNCNamedPipe(strings.TrimPrefix(p, `\\?\unc\`))
	}
	if strings.HasPrefix(p, `\\`) {
		return windowsUNCNamedPipe(strings.TrimPrefix(p, `\\`))
	}
	return false
}

func windowsPathHasComponentPrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+`\`)
}

func windowsUNCNamedPipe(rest string) bool {
	parts := strings.Split(rest, `\`)
	return len(parts) >= 2 && parts[0] != "" && parts[1] == "pipe"
}
