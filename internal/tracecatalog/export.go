package tracecatalog

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const ExportSchemaVersion = 1

type ExportFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type ExportReference struct {
	QueryID     string `json:"query_id"`
	AttemptID   string `json:"attempt_id"`
	Role        string `json:"role"`
	OriginalRef string `json:"original_ref"`
	File        string `json:"file,omitempty"`
	Status      string `json:"status"`
	Reason      string `json:"reason,omitempty"`
}

type ExportManifest struct {
	SchemaVersion      int                     `json:"schema_version"`
	PackageID          string                  `json:"package_id"`
	Catalog            Snapshot                `json:"catalog_navigation"`
	ExecutionCoverage  ExecutionCoverage       `json:"execution_coverage"`
	QueryOrigins       map[string]QueryOrigin  `json:"query_origins"`
	Representatives    *RepresentativeSnapshot `json:"representatives_navigation,omitempty"`
	References         []ExportReference       `json:"references"`
	Files              []ExportFile            `json:"files"`
	MaterialComplete   bool                    `json:"material_complete"`
	MaterialIssues     []Issue                 `json:"material_issues"`
	NativeAvailability string                  `json:"native_availability"`
	AnalysisCompletion string                  `json:"analysis_completion"`
	Boundary           string                  `json:"boundary"`
}

type ExportReceipt struct {
	SchemaVersion      int               `json:"schema_version"`
	PackageID          string            `json:"package_id"`
	Target             string            `json:"target"`
	ManifestPath       string            `json:"manifest_path"`
	ManifestSHA256     string            `json:"manifest_sha256"`
	ReportPath         string            `json:"report_path"`
	PhysicalSuccess    bool              `json:"physical_success"`
	MaterialComplete   bool              `json:"material_complete"`
	ExecutionCoverage  ExecutionCoverage `json:"execution_coverage"`
	NativeAvailability string            `json:"native_availability"`
	AnalysisCompletion string            `json:"analysis_completion"`
}

const ExportBoundary = "This versioned package archives current-run native result bytes and navigation, not a diagnosis or new read/query/causal permission. Native availability is not inferred from tool success. Material completeness concerns copied current-attempt references only, not input/event coverage. Historical navigation does not count as current execution. No source capture is copied; original source identity and reference paths remain audit metadata. " + ExecutionCoverageBoundary

// Export publishes a NEW named package below a host-owned output root. The
// caller must separately reauthorize the catalog inputs. Paths or JSON loaded
// from disk cannot construct a Catalog or private reference binding.
func (c *Catalog) Export(ctx context.Context, outputRoot, target string) (ExportReceipt, error) {
	if c == nil || c.run == nil {
		return ExportReceipt{}, fmt.Errorf("current live catalog required")
	}
	if err := validExportTarget(target); err != nil {
		return ExportReceipt{}, err
	}
	if err := ctx.Err(); err != nil {
		return ExportReceipt{}, err
	}
	if outputRoot == "" {
		return ExportReceipt{}, fmt.Errorf("host output root is required")
	}
	work, err := os.OpenRoot(outputRoot)
	if err != nil {
		return ExportReceipt{}, err
	}
	defer work.Close()
	if err := work.Mkdir("evidence-exports", 0700); err != nil && !os.IsExist(err) {
		return ExportReceipt{}, err
	}
	info, err := work.Lstat("evidence-exports")
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ExportReceipt{}, fmt.Errorf("managed export directory is not a plain directory")
	}
	root, err := work.OpenRoot("evidence-exports")
	if err != nil {
		return ExportReceipt{}, err
	}
	defer root.Close()
	// Atomic acquisition protects same-target publishers across live forks and
	// processes. Existing target/lock is never replaced or treated as a cache hit.
	lock := "." + target + ".lock"
	if err := root.Mkdir(lock, 0700); err != nil {
		return ExportReceipt{}, fmt.Errorf("export target is busy: %w", err)
	}
	defer root.Remove(lock)
	if _, err := root.Lstat(target); err == nil {
		return ExportReceipt{}, fmt.Errorf("export target already exists; choose a new explicit package name")
	} else if !os.IsNotExist(err) {
		return ExportReceipt{}, err
	}
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return ExportReceipt{}, err
	}
	stage := ".stage-" + hex.EncodeToString(random[:])
	if err := root.Mkdir(stage, 0700); err != nil {
		return ExportReceipt{}, err
	}
	defer root.RemoveAll(stage) // only this freshly-created, root-confined staging directory
	staging, err := root.OpenRoot(stage)
	if err != nil {
		return ExportReceipt{}, err
	}
	defer staging.Close()
	if err := staging.Mkdir("results", 0700); err != nil {
		return ExportReceipt{}, err
	}

	live := c.Clone()
	validSources := map[string]bool{}
	for _, a := range live.Snapshot().Artifacts {
		_, resolveErr := live.Resolve(ctx, a.ID)
		validSources[a.ID] = resolveErr == nil
	}
	if err := ctx.Err(); err != nil {
		return ExportReceipt{}, err
	}
	execution := live.CurrentExecution()
	m := ExportManifest{SchemaVersion: ExportSchemaVersion, Catalog: execution.Snapshot, ExecutionCoverage: execution.Coverage, QueryOrigins: execution.QueryOrigins,
		References: []ExportReference{}, Files: []ExportFile{}, MaterialIssues: []Issue{}, MaterialComplete: true, NativeAvailability: "unknown", AnalysisCompletion: "not_asserted", Boundary: ExportBoundary}
	if representatives, ok := live.Representatives(); ok {
		m.Representatives = &representatives
	}
	if len(m.Catalog.Queries) == 0 {
		m.MaterialComplete = false
		m.MaterialIssues = append(m.MaterialIssues, Issue{Code: "no_current_queries", Message: "No current query result roster exists; candidate discovery is not an evidence set."})
	}
	for _, a := range m.Catalog.Artifacts {
		if !validSources[a.ID] {
			m.MaterialComplete = false
			m.MaterialIssues = append(m.MaterialIssues, Issue{Code: "source_not_current", Path: a.Path, Message: "Original source is no longer current or available; old result files cannot prove current material."})
		}
	}
	var total int64
	copied := map[string]ExportFile{}
	for _, q := range m.Catalog.Queries {
		if len(q.Attempts) == 0 {
			m.MaterialComplete = false
			m.MaterialIssues = append(m.MaterialIssues, Issue{Code: "query_not_executed", Path: q.ID, Message: "This current query has no executed attempt and no original result to export."})
		}
		for _, attempt := range q.Attempts {
			if attempt.PayloadRef == "" && attempt.RawRef == "" {
				m.MaterialComplete = false
				m.MaterialIssues = append(m.MaterialIssues, Issue{Code: "attempt_without_result", Path: attempt.ID, Message: "This recorded attempt has no original result reference; its execution outcome is retained independently."})
			}
			binding, bound := live.exportBinding(q.ID, attempt)
			for _, role := range []struct{ name, ref string }{{"native_payload", attempt.PayloadRef}, {"tool_raw", attempt.RawRef}} {
				if role.ref == "" {
					continue
				}
				ref := ExportReference{QueryID: q.ID, AttemptID: attempt.ID, Role: role.name, OriginalRef: role.ref, Status: "missing"}
				file, known := binding.files[role.ref]
				switch {
				case !validSources[q.ArtifactID]:
					ref.Reason = "source is not current"
				case !bound || !known:
					ref.Reason = "no current producer reference binding"
					if binding.issues[role.ref] != "" {
						ref.Reason = binding.issues[role.ref]
					}
				default:
					_, data, readErr := readExportFile(ctx, file.root, file.path, &file)
					if readErr != nil {
						ref.Reason = boundedMessage(readErr.Error())
						if errors.Is(readErr, ErrExportReferenceStale) {
							ref.Status = "stale_reference"
						}
						break
					}
					name := "results/" + file.sha256 + ".blob"
					if _, exists := copied[name]; !exists {
						if len(copied) >= MaxExportFiles || total+int64(len(data)) > MaxExportMaterialBytes {
							ref.Status = "omitted_budget"
							ref.Reason = "package material budget exceeded; entire result omitted"
							break
						}
						if err := exportWriteFile(staging, name, data); err != nil {
							return ExportReceipt{}, err
						}
						copied[name] = ExportFile{Path: name, SHA256: file.sha256, Bytes: int64(len(data))}
						m.Files = append(m.Files, copied[name])
						total += int64(len(data))
					}
					ref.Status, ref.File = "copied", name
				}
				if ref.Status != "copied" {
					m.MaterialComplete = false
				}
				m.References = append(m.References, ref)
			}
			if attempt.Truncated {
				m.MaterialComplete = false
				m.MaterialIssues = append(m.MaterialIssues, Issue{Code: "result_truncated", Path: q.ID, Message: "Producer marked this attempt incomplete; copying its bytes does not restore omitted members."})
			}
		}
	}
	// Recheck actual bound source universes after copying. A mutation during
	// export aborts the transaction instead of publishing an obsolete complete set.
	for _, a := range m.Catalog.Artifacts {
		if validSources[a.ID] {
			if _, err := live.Resolve(ctx, a.ID); err != nil {
				return ExportReceipt{}, fmt.Errorf("source changed during export: %w", err)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return ExportReceipt{}, err
	}
	report := exportReport(m)
	if len(report) > MaxSnapshotBytes {
		return ExportReceipt{}, fmt.Errorf("report exceeds export metadata limit")
	}
	if err := exportWriteFile(staging, "report.md", []byte(report)); err != nil {
		return ExportReceipt{}, err
	}
	reportSum := sha256.Sum256([]byte(report))
	m.Files = append(m.Files, ExportFile{Path: "report.md", SHA256: hex.EncodeToString(reportSum[:]), Bytes: int64(len(report))})
	m.PackageID = digest("evidence_package_", m)
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return ExportReceipt{}, err
	}
	data = append(data, '\n')
	if len(data) > MaxSnapshotBytes {
		return ExportReceipt{}, fmt.Errorf("manifest exceeds export metadata limit; nothing published")
	}
	if err := exportWriteFile(staging, "manifest.json", data); err != nil {
		return ExportReceipt{}, err
	}
	if err := ctx.Err(); err != nil {
		return ExportReceipt{}, err
	}
	if _, err := root.Lstat(target); err == nil {
		return ExportReceipt{}, fmt.Errorf("export target appeared during publication")
	} else if !os.IsNotExist(err) {
		return ExportReceipt{}, err
	}
	if err := exportPublishNoReplace(root, stage, target); err != nil {
		return ExportReceipt{}, err
	}
	sum := sha256.Sum256(data)
	packagePath := filepath.Join(outputRoot, "evidence-exports", target)
	return ExportReceipt{SchemaVersion: ExportSchemaVersion, PackageID: m.PackageID, Target: target, ManifestPath: filepath.Join(packagePath, "manifest.json"), ManifestSHA256: hex.EncodeToString(sum[:]), ReportPath: filepath.Join(packagePath, "report.md"), PhysicalSuccess: true, MaterialComplete: m.MaterialComplete, ExecutionCoverage: m.ExecutionCoverage, NativeAvailability: m.NativeAvailability, AnalysisCompletion: m.AnalysisCompletion}, nil
}

func exportPublishNoReplace(root *os.Root, stage, target string) error {
	if stage == "" || stage != filepath.Base(stage) || strings.ContainsAny(stage, "/\\") {
		return fmt.Errorf("invalid export staging leaf")
	}
	if err := validExportTarget(target); err != nil {
		return err
	}
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	return exportRenameNoReplace(dir.Fd(), stage, target)
}

func validExportTarget(target string) error {
	if target == "" || target != strings.TrimSpace(target) || len(target) > 120 || !utf8.ValidString(target) || target == "." || target == ".." || strings.HasPrefix(target, ".") || strings.ContainsAny(target, "/\\\x00\r\n") || filepath.IsAbs(target) {
		return fmt.Errorf("export target must be an explicit new package name, not a path (1..120 UTF-8 bytes)")
	}
	return nil
}

func exportWriteFile(root *os.Root, name string, data []byte) error {
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func exportReport(m ExportManifest) string {
	var b strings.Builder
	b.WriteString("# Capture evidence package\n\n")
	b.WriteString(ExportBoundary + "\n\n")
	fmt.Fprintf(&b, "Material references complete: %t. Native availability: unknown. Analysis completion: not asserted.\n\n", m.MaterialComplete)
	coverage, _ := json.MarshalIndent(m.ExecutionCoverage, "", "  ")
	b.WriteString("## Current execution coverage\n\n```json\n" + string(coverage) + "\n```\n\n")
	b.WriteString("## Current query roster\n\nEach JSON line identifies a query, not a distinct physical thread across captures. Original source/window and all attempt references are retained in manifest.json.\n\n```json\n")
	for _, q := range m.Catalog.Queries {
		row, _ := json.Marshal(struct {
			QueryID    string      `json:"query_id"`
			ArtifactID string      `json:"artifact_id"`
			Origin     QueryOrigin `json:"origin"`
			Object     Object      `json:"object"`
			View       string      `json:"view"`
			Outcome    Outcome     `json:"execution_outcome"`
			Attempts   int         `json:"attempts"`
		}{q.ID, q.ArtifactID, m.QueryOrigins[q.ID], q.Object, q.View, q.Outcome, len(q.Attempts)})
		b.Write(row)
		b.WriteByte('\n')
	}
	b.WriteString("```\n\nNo diagnosis, causal conclusion, or inference about unqueried candidates is generated by this report.\n")
	return b.String()
}
