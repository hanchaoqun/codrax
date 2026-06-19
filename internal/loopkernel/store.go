package loopkernel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func WriteLoopEventsToFile(events []LoopEvent, path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("WriteLoopEventsToFile: empty path")
	}
	normalized := NormalizeLoopEvents(events)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("WriteLoopEventsToFile: mkdir %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return fmt.Errorf("WriteLoopEventsToFile: marshal: %w", err)
	}
	if err := types.AtomicWriteFileSync(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("WriteLoopEventsToFile: write %s: %w", path, err)
	}
	return nil
}

func LoadLoopEventsFromFile(path string) ([]LoopEvent, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("LoadLoopEventsFromFile: read %s: %w", path, err)
	}
	var events []LoopEvent
	if err := json.Unmarshal(data, &events); err != nil {
		return nil, fmt.Errorf("LoadLoopEventsFromFile: parse %s: %w", path, err)
	}
	return NormalizeLoopEvents(events), nil
}
