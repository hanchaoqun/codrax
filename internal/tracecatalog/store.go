package tracecatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

const MaxSnapshotBytes = 8 << 20

// Save publishes a compact navigation snapshot via same-directory rename.
// It never writes source files or embeds original query payloads. Callers own
// the private output location and must merge live forks before saving it.
func (c *Catalog) Save(ctx context.Context, path string) error {
	if c == nil || c.run == nil {
		return fmt.Errorf("nil catalog")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Only same-run live publications participate in the union. Disk contents
	// never restore membership, validators or the private declared-plan marker.
	c.run.publication.Lock()
	defer c.run.publication.Unlock()
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	dir, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return err
	}
	abs = filepath.Join(dir, filepath.Base(abs))
	publication := c.Clone()
	if prior := c.run.published[abs]; prior != nil {
		if err := publication.Merge(prior); err != nil {
			return err
		}
	} else if len(c.run.published) >= 8 {
		return fmt.Errorf("catalog publication destination limit reached")
	}
	snapshot := publication.Snapshot()
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	if len(data)+1 > MaxSnapshotBytes {
		return fmt.Errorf("catalog snapshot exceeds %d bytes; original data retained in memory", MaxSnapshotBytes)
	}
	for _, artifact := range snapshot.Artifacts {
		if artifact.Path == abs {
			return fmt.Errorf("catalog destination is a discovered source file")
		}
	}
	if info, err := os.Lstat(abs); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("catalog destination is not a regular file")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	temp, err := os.CreateTemp(dir, ".trace-catalog-*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	published := false
	defer func() {
		_ = temp.Close()
		if !published {
			_ = os.Remove(name)
		}
	}()
	if _, err := temp.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(name, abs); err != nil {
		return err
	}
	published = true
	if c.run.published == nil {
		c.run.published = map[string]*Catalog{}
	}
	c.run.published[abs] = publication
	return nil
}

// Load reads historical navigation only. Deliberately returns Snapshot, not a
// Catalog: no serialized field can recreate source validators, run ownership,
// current query receipts, or permission to read/query a referenced path.
func Load(ctx context.Context, path string) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	f, generation, err := filegeneration.OpenRegularReadOnly(path)
	if err != nil {
		return Snapshot{}, err
	}
	defer f.Close()
	if generation.Size() > MaxSnapshotBytes {
		return Snapshot{}, fmt.Errorf("catalog snapshot exceeds size limit")
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxSnapshotBytes+1))
	if err != nil {
		return Snapshot{}, err
	}
	if len(data) > MaxSnapshotBytes {
		return Snapshot{}, fmt.Errorf("catalog snapshot exceeds size limit")
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	after, err := filegeneration.FromFile(f)
	if err != nil || !generation.SameVersion(after) {
		return Snapshot{}, fmt.Errorf("catalog changed while reading")
	}
	current, err := filegeneration.FromPath(path)
	if err != nil || !generation.SameVersion(current) {
		return Snapshot{}, fmt.Errorf("catalog path changed while reading")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var snapshot Snapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return Snapshot{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Snapshot{}, fmt.Errorf("catalog has trailing data")
	}
	// Indented JSON persistence may indent RawMessage values. Canonicalize the
	// exact decoded parameters again before checking their compound query key.
	for i := range snapshot.Queries {
		plan, err := normalizePlan(snapshot.Queries[i].Plan)
		if err != nil {
			return Snapshot{}, err
		}
		snapshot.Queries[i].Plan = plan
	}
	if err := validateSnapshot(snapshot); err != nil {
		return Snapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func validateSnapshot(s Snapshot) error {
	if s.SchemaVersion != SchemaVersion || s.ID == "" || !s.NavigationOnly || !filepath.IsAbs(s.Root) {
		return fmt.Errorf("unsupported or non-navigation catalog snapshot")
	}
	if len(s.Artifacts) > DefaultMaxArtifacts || len(s.Queries) > MaxQueries || len(s.Discovery.Issues) > MaxDiscoveryIssues {
		return fmt.Errorf("catalog snapshot has excessive entries")
	}
	artifacts := map[string]Artifact{}
	for _, a := range s.Artifacts {
		if a.ID != digest("artifact_", a.Path) || !filepath.IsAbs(a.Path) || a.Bytes < 0 {
			return fmt.Errorf("catalog artifact identity is invalid")
		}
		if _, duplicate := artifacts[a.ID]; duplicate {
			return fmt.Errorf("duplicate catalog artifact")
		}
		rel, err := filepath.Rel(s.Root, a.Path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("catalog artifact is outside its recorded root")
		}
		if a.Path == s.Root {
			rel = filepath.Base(a.Path)
		}
		if filepath.ToSlash(rel) != a.RelativePath {
			return fmt.Errorf("catalog artifact relative path differs")
		}
		switch a.Status {
		case "candidate", "unavailable", "stale":
		default:
			return fmt.Errorf("catalog artifact status is invalid")
		}
		artifacts[a.ID] = a
	}
	queries := map[string]bool{}
	attemptIDs := map[string]bool{}
	sequences := map[uint64]bool{}
	for _, q := range s.Queries {
		a, exists := artifacts[q.ArtifactID]
		if !exists || q.ArtifactRevision != a.RevisionID || q.ID != queryID(q) || queries[q.ID] {
			return fmt.Errorf("catalog query identity is invalid or duplicated")
		}
		queries[q.ID] = true
		p, err := normalizePlan(q.Plan)
		if err != nil || !bytes.Equal(p.Parameters, q.Parameters) {
			return fmt.Errorf("catalog query parameters are not canonical")
		}
		var previous uint64
		for _, attempt := range q.Attempts {
			if err := validateCompletion(attempt.Completion); err != nil {
				return err
			}
			expected := digest("attempt_", struct {
				Query    string
				Sequence uint64
			}{q.ID, attempt.Sequence})
			if attempt.ID != expected || attemptIDs[attempt.ID] || sequences[attempt.Sequence] || attempt.Sequence <= previous {
				return fmt.Errorf("catalog attempt identity or order is invalid")
			}
			attemptIDs[attempt.ID] = true
			sequences[attempt.Sequence] = true
			previous = attempt.Sequence
		}
		expected := OutcomeNotExecuted
		if len(q.Attempts) > 0 {
			expected = q.Attempts[len(q.Attempts)-1].Outcome
		}
		if q.Outcome != expected && !(q.Outcome == OutcomeStale && a.Status == "stale") {
			return fmt.Errorf("catalog current outcome differs from history")
		}
	}
	if len(attemptIDs) > MaxAttempts {
		return fmt.Errorf("catalog attempt count exceeds limit")
	}
	return nil
}
