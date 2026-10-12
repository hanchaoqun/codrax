package dataquery

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

// NamedInputs carries only this request's exact regular files, not directory
// authority or evidence of consumption. The private generation is rechecked
// before and after data execution; it cannot be reconstructed from plan JSON.
type NamedInputs struct {
	files map[string]filegeneration.Identity
	used  map[string]bool
}

func NewNamedInputs(paths []string) *NamedInputs {
	n := &NamedInputs{files: make(map[string]filegeneration.Identity)}
	for _, path := range paths {
		path, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		id, err := filegeneration.FromPath(path)
		if err == nil && id.Mode().IsRegular() {
			n.files[filepath.Clean(path)] = id
		}
	}
	return n
}

func (n *NamedInputs) resolve(root, raw string) (string, bool, error) {
	path := raw
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path, err := filepath.Abs(path)
	if err != nil || n == nil {
		return "", false, err
	}
	id, ok := n.files[path]
	if !ok {
		return "", false, nil
	}
	if n.used != nil {
		n.used[path] = true
	}
	current, err := filegeneration.FromPath(path)
	if err != nil || !id.SameVersion(current) {
		return "", true, fmt.Errorf("named input changed or unavailable: %s", path)
	}
	return path, true, nil
}

func (n *NamedInputs) validate() error {
	if n != nil {
		for path := range n.used {
			if _, _, err := n.resolve("", path); err != nil {
				return err
			}
		}
	}
	return nil
}

func (n *NamedInputs) tracked() *NamedInputs {
	if n == nil {
		return nil
	}
	return &NamedInputs{files: n.files, used: map[string]bool{}}
}

// DiscoverCandidateFilesWithNamed gives explicit inputs first claim on the
// bounded inventory. Unknown kinds remain visible, never declared text-ready.
func DiscoverCandidateFilesWithNamed(root string, paths []string, limit int) ([]CandidateFile, *NamedInputs, error) {
	if limit <= 0 {
		limit = 200
	}
	named := NewNamedInputs(paths)
	var out []CandidateFile
	seen := make(map[string]bool)
	for _, path := range paths {
		if len(out) >= limit {
			break
		}
		abs, ok, err := named.resolve(root, path)
		if !ok || err != nil {
			out = append(out, CandidateFile{Path: path, Kind: "unknown", InspectError: "named input unavailable or changed"})
			continue
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		out = append(out, inspectNamedCandidate(abs, abs))
	}
	repo, err := DiscoverCandidateFiles(root, limit)
	if err != nil {
		return nil, nil, err
	}
	for _, candidate := range repo {
		if len(out) >= limit {
			break
		}
		abs, _ := filepath.Abs(filepath.Join(root, candidate.Path))
		if !seen[abs] {
			out = append(out, candidate)
			seen[abs] = true
		}
	}
	return out, named, nil
}

func inspectNamedCandidate(path, display string) CandidateFile {
	f := CandidateFile{Path: display, Kind: dataKindForPath(path)}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		f.Kind = "unknown"
		f.InspectError = "input unavailable or not a regular file"
		return f
	}
	f.Size = info.Size()
	if f.Kind == "" {
		f.Kind = "unknown"
		f.InspectError = "no supported data reader; content not inspected"
		return f
	}
	if f.Size > EffectiveMaxFileBytes(0) {
		f.InspectError = fmt.Sprintf("input exceeds %d bytes", EffectiveMaxFileBytes(0))
		return f
	}
	return inspectCandidateFile(path, f)
}

func inspectedMaterialPaths(artifact DataArtifact) []string {
	var out []string
	for _, child := range artifact.Children {
		// Inventory/schema/reader availability is not content consumption.
		if child.Fields["extraction_status"] == "text_ready" && child.Fields["inspect_error"] == "" {
			out = append(out, child.SourcePaths...)
		}
	}
	return out
}

// copyNamedDataInputs preserves source spelling in helper/coverage results but
// opens only bounded private copies; absolute user paths never reach Python IO.
func copyNamedDataInputs(root, workDir string, paths []string, maxFile, maxTotal int64, extra map[string]string, named *NamedInputs) ([]string, map[string]string, error) {
	var ordinary, originals []string
	copies := make(map[string]string)
	type namedCopy struct{ key, path string }
	var selected []namedCopy
	seen := make(map[string]bool)
	var total int64
	for _, raw := range paths {
		abs, ok, err := named.resolve(root, raw)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			if _, _, err := resolveInputPath(root, raw); err == nil {
				ordinary = append(ordinary, raw)
				continue
			}
			return nil, nil, fmt.Errorf("input path %s is outside data root and not named in the current request", raw)
		}
		if !isRunnerInputKind(dataKindForPath(abs)) {
			return nil, nil, fmt.Errorf("input %s needs a supported content reader", raw)
		}
		key := normalizeMaterialPath(raw)
		if seen[key] {
			continue
		}
		seen[key] = true
		selected = append(selected, namedCopy{key: key, path: abs})
	}
	if len(ordinary) > 0 {
		rels, err := copyInputsWithExtras(root, workDir, ordinary, maxFile, maxTotal, extra)
		if err != nil {
			return nil, nil, err
		}
		for _, rel := range rels {
			info, err := os.Stat(filepath.Join(workDir, rel))
			if err != nil {
				return nil, nil, err
			}
			total += info.Size()
		}
		originals = append(originals, rels...)
	}
	if len(selected) > 0 {
		// Allocate after ordinary copies: no repository path can occupy or
		// overwrite a named source's private slot, even through a directory input.
		privateDir, err := os.MkdirTemp(workDir, "_named-")
		if err != nil {
			return nil, nil, err
		}
		for _, source := range selected {
			digest := sha256.Sum256([]byte(source.key))
			rel := filepath.ToSlash(filepath.Join(filepath.Base(privateDir), fmt.Sprintf("%x", digest), filepath.Base(source.path)))
			size, err := copyOneInput(source.path, filepath.Join(workDir, rel), maxFile)
			if err != nil {
				return nil, nil, err
			}
			total += size
			if total > maxTotal {
				return nil, nil, fmt.Errorf("data task input total exceeds %d bytes", maxTotal)
			}
			copies[source.key] = rel
			originals = append(originals, source.key)
		}
	}
	if len(originals) == 0 {
		return nil, nil, fmt.Errorf("no supported data input files were copied")
	}
	if err := named.validate(); err != nil {
		return nil, nil, err
	}
	return originals, copies, nil
}
