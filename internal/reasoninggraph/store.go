package reasoninggraph

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func WriteEventsToFile(events []ReasoningEvent, path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("WriteEventsToFile: empty path")
	}
	normalized := NormalizeEvents(events)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("WriteEventsToFile: mkdir %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return fmt.Errorf("WriteEventsToFile: marshal: %w", err)
	}
	if err := types.AtomicWriteFileSync(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("WriteEventsToFile: write %s: %w", path, err)
	}
	return nil
}

func LoadEventsFromFile(path string) ([]ReasoningEvent, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("LoadEventsFromFile: read %s: %w", path, err)
	}
	var events []ReasoningEvent
	if err := json.Unmarshal(data, &events); err != nil {
		return nil, fmt.Errorf("LoadEventsFromFile: parse %s: %w", path, err)
	}
	return NormalizeEvents(events), nil
}
