package traceinput

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

type managedDirectory struct {
	path      string
	authority *hitraceconv.ManagedOutputDirectory
}

func newManagedDirectory(anchor, fallback string) (*managedDirectory, error) {
	if strings.TrimSpace(anchor) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		anchor = filepath.Join(cwd, ".codrax")
	}
	authority, err := hitraceconv.NewManagedOutputDirectory(anchor, fallback, "trace-input-*")
	if err != nil {
		return nil, err
	}
	return &managedDirectory{path: authority.Path(), authority: authority}, nil
}

func (d *managedDirectory) validate() error {
	return d.authority.Validate()
}

func (d *managedDirectory) cleanup() error {
	return d.authority.Cleanup()
}

func (d *managedDirectory) close() error {
	return d.authority.Close()
}
