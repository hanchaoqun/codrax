package probe

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// probePythons discovers every Python interpreter the operator
// might use: PATH binaries (python3 / python), project venvs
// (.venv / venv / env / .virtualenv with Unix and Windows
// layouts), pyenv-managed installs (~/.pyenv/versions), conda
// envs (CONDA_PREFIX or active prefix). Each interpreter is
// fingerprinted with version + has-pytest / has-pytest-json-report
// fast checks against its site-packages.
//
// Output is ordered: project venv first (preferred for tests),
// then PATH, then pyenv / conda. Recommender uses the first hit
// as the default suggestion.
func probePythons(facts *types.EnvFacts) []types.PythonInterp {
	var out []types.PythonInterp
	seen := make(map[string]bool)

	// Project venvs in repo root take priority. The ProjectFiles
	// map records pyproject.toml etc.; we read its location to
	// know where to look.
	for _, pf := range facts.ProjectFiles {
		dir := filepath.Dir(pf)
		for _, name := range []string{".venv", "venv", "env", ".virtualenv"} {
			for _, sub := range venvBinSubpaths() {
				candidate := filepath.Join(dir, name, sub)
				if isExecutable(candidate) && !seen[candidate] {
					seen[candidate] = true
					out = append(out, fingerprintPython(candidate, "venv"))
				}
			}
		}
	}

	// PATH binaries.
	for _, name := range []string{"python3", "python", "python3.13", "python3.12", "python3.11", "python3.10", "python3.9"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		// Resolve symlinks so two PATH entries pointing at the
		// same binary deduplicate cleanly.
		if real, err := filepath.EvalSymlinks(path); err == nil {
			path = real
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, fingerprintPython(path, "system"))
	}

	// pyenv-managed installs. PYENV_ROOT or default ~/.pyenv.
	pyenvRoot := os.Getenv("PYENV_ROOT")
	if pyenvRoot == "" {
		if home, err := os.UserHomeDir(); err == nil {
			pyenvRoot = filepath.Join(home, ".pyenv")
		}
	}
	if pyenvRoot != "" {
		versionsDir := filepath.Join(pyenvRoot, "versions")
		if entries, err := os.ReadDir(versionsDir); err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				candidate := filepath.Join(versionsDir, e.Name(), "bin", "python")
				if !isExecutable(candidate) {
					candidate = filepath.Join(versionsDir, e.Name(), "bin", "python3")
				}
				if isExecutable(candidate) && !seen[candidate] {
					seen[candidate] = true
					out = append(out, fingerprintPython(candidate, "pyenv"))
				}
			}
		}
	}

	// Active conda environment. CONDA_PREFIX is the env var conda
	// sets when you `conda activate <env>`; if set, the python
	// binary lives at $CONDA_PREFIX/bin/python (Unix) or
	// $CONDA_PREFIX\python.exe (Windows).
	if condaPrefix := os.Getenv("CONDA_PREFIX"); condaPrefix != "" {
		candidate := filepath.Join(condaPrefix, "bin", "python")
		if runtime.GOOS == "windows" {
			candidate = filepath.Join(condaPrefix, "python.exe")
		}
		if isExecutable(candidate) && !seen[candidate] {
			seen[candidate] = true
			out = append(out, fingerprintPython(candidate, "conda"))
		}
	}

	return out
}

// fingerprintPython runs `python -V` and probes for pytest /
// pytest-json-report via `python -c "import pytest"`. All probes
// are bounded by short timeouts; failures fill zero values.
func fingerprintPython(path, origin string) types.PythonInterp {
	p := types.PythonInterp{Path: path, Origin: origin}
	if v, err := captureCmd(path, "-V"); err == nil {
		// Output: "Python 3.11.5"
		v = strings.TrimSpace(v)
		v = strings.TrimPrefix(v, "Python ")
		p.Version = v
	}
	// Fast import probes — if the module imports cleanly the binary
	// has it on its sys.path.
	if err := runQuick(path, "-c", "import pytest"); err == nil {
		p.HasPytest = true
	}
	if err := runQuick(path, "-c", "import pytest_jsonreport"); err == nil {
		p.HasJsonReport = true
	}
	// site-packages path is informational; helps the renderer
	// explain "I see pytest at <path>".
	if sp, err := captureCmd(path, "-c", "import site; print(site.getsitepackages()[0])"); err == nil {
		p.SitePackagesPath = strings.TrimSpace(sp)
	}
	return p
}

// venvBinSubpaths returns the OS-appropriate venv binary paths to
// probe. Unix has bin/python and bin/python3; Windows uses
// Scripts\python.exe.
func venvBinSubpaths() []string {
	if runtime.GOOS == "windows" {
		return []string{filepath.Join("Scripts", "python.exe")}
	}
	return []string{filepath.Join("bin", "python"), filepath.Join("bin", "python3")}
}
