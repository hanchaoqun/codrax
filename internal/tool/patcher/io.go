package patcher

import (
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// readFileIfExists reads a file and returns its content, whether it existed,
// and its file mode. For non-existent files it returns the default mode 0644
// so callers can reuse it when creating new files.
func readFileIfExists(path string) (content string, exists bool, mode os.FileMode, err error) {
	info, statErr := os.Stat(path)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return "", false, 0o644, nil
		}
		return "", false, 0, statErr
	}
	if info.IsDir() {
		return "", false, 0, &os.PathError{Op: "read", Path: path, Err: os.ErrInvalid}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false, 0, err
	}
	return string(b), true, info.Mode().Perm(), nil
}

// atomicWriteFile writes content to path by creating a temp file in the same
// directory and renaming it into place. The file mode is applied before the
// rename so the final file has the intended permissions.
func atomicWriteFile(path, content string, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".applypatch-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	// Ensure the temp file is removed if anything after this point fails.
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	success = true
	return nil
}

// isBinary heuristically detects binary content: null byte in the first 8 KiB
// or invalid UTF-8. Text with occasional high bytes passes; true binary does
// not.
func isBinary(content string) bool {
	const sniff = 8 * 1024
	sample := content
	if len(sample) > sniff {
		sample = sample[:sniff]
	}
	if strings.IndexByte(sample, 0) >= 0 {
		return true
	}
	if !utf8.ValidString(sample) {
		return true
	}
	return false
}
