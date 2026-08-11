package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func writeTraceFindingSidecar(path string, finding *types.TraceFindingV1) (string, error) {
	if finding == nil {
		return "", fmt.Errorf("trace finding is nil")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("trace finding output path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve trace finding output path: %w", err)
	}
	if _, err := os.Stat(abs); err == nil {
		return "", fmt.Errorf("trace finding output already exists: %s", abs)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect trace finding output: %w", err)
	}
	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create trace finding output directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".trace-finding-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create trace finding temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	published := false
	defer func() {
		_ = tmp.Close()
		if !published {
			_ = os.Remove(tmpPath)
		}
	}()
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(finding); err != nil {
		return "", fmt.Errorf("encode trace finding: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return "", fmt.Errorf("sync trace finding: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close trace finding: %w", err)
	}
	if err := os.Rename(tmpPath, abs); err != nil {
		return "", fmt.Errorf("publish trace finding: %w", err)
	}
	published = true
	return abs, nil
}
