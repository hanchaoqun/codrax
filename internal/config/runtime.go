package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// RuntimeSettings holds the per-process knobs that the codrax binary
// exposes on the command line: where to write logs, what level to
// emit, where to persist conversation memory, and which language the
// model should default to.
//
// Every field is a pointer so the merge logic in main.go can tell
// "user omitted this key in the YAML file" (nil) from "user set it
// to the zero value" (non-nil pointer to the zero value). This matters
// for LogStdout in particular: a default of false and a config file
// with `log_stdout: false` must both be allowed to override each
// other based on precedence.
type RuntimeSettings struct {
	LogDir    *string `yaml:"log_dir"`
	LogLevel  *string `yaml:"log_level"`
	LogStdout *bool   `yaml:"log_stdout"`
	MemoryDir *string `yaml:"memory_dir"`
	Lang      *string `yaml:"lang"`
}

// LoadRuntimeSettings reads path as a YAML document into a
// RuntimeSettings. The empty-but-present case (a file with no keys)
// is valid and returns a zero-value struct, which the caller treats
// as "inherit all defaults". A missing file is signaled by
// os.ErrNotExist so the caller can decide whether silence (default
// path) or a loud error (explicit --settings path) is appropriate.
func LoadRuntimeSettings(path string) (*RuntimeSettings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s RuntimeSettings
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse runtime settings: %w", err)
	}
	return &s, nil
}

// IsNotExist is re-exported so main.go can keep its runtime-config
// branching self-contained without importing os just for this check.
func IsNotExist(err error) bool { return errors.Is(err, os.ErrNotExist) }
