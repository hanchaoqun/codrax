package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
)

func inspectExportPackage(r tracecatalog.ExportReceipt, target string) (m tracecatalog.ExportManifest, verified []tracecatalog.ExportFile, err error) {
	if target == "" || r.Target != target || r.SchemaVersion != tracecatalog.ExportSchemaVersion || filepath.Base(r.ManifestPath) != "manifest.json" || r.ReportPath != filepath.Join(filepath.Dir(r.ManifestPath), "report.md") {
		return m, nil, errors.New("export target/schema/manifest/report path differs from requested delivery")
	}
	dir := filepath.Dir(r.ManifestPath)
	if filepath.Base(dir) != r.Target {
		return m, nil, errors.New("physical package directory differs from requested target")
	}
	if info, statErr := os.Lstat(dir); statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return m, nil, errors.New("package root is not a plain directory")
	}
	data, err := readAuditFile(r.ManifestPath)
	if err != nil {
		return m, nil, err
	}
	if hash(data) != r.ManifestSHA256 {
		return m, nil, errors.New("manifest bytes differ from original export receipt")
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, nil, err
	}
	if m.SchemaVersion != r.SchemaVersion || m.PackageID == "" || m.PackageID != r.PackageID || m.MaterialComplete != r.MaterialComplete || m.NativeAvailability != r.NativeAvailability || m.AnalysisCompletion != r.AnalysisCompletion || !reflect.DeepEqual(m.ExecutionCoverage, r.ExecutionCoverage) {
		return m, nil, errors.New("manifest identity/completion axes differ from export receipt")
	}
	files := map[string]tracecatalog.ExportFile{}
	for _, file := range m.Files {
		if _, duplicate := files[file.Path]; duplicate {
			return m, verified, fmt.Errorf("duplicate package file: %s", file.Path)
		}
		if err := plainPackageFile(dir, file.Path); err != nil {
			return m, verified, err
		}
		body, err := readAuditFile(filepath.Join(dir, file.Path))
		if err != nil {
			return m, verified, err
		}
		if int64(len(body)) != file.Bytes || hash(body) != file.SHA256 {
			return m, verified, fmt.Errorf("package file bytes/hash differ: %s", file.Path)
		}
		files[file.Path] = file
		verified = append(verified, file)
	}
	if _, ok := files["report.md"]; !ok {
		return m, verified, errors.New("manifest omits execution report")
	}
	if err := inspectExportReferences(m, files); err != nil {
		return m, verified, err
	}
	for _, ref := range m.References {
		if ref.Status != "copied" {
			continue
		}
		if !filepath.IsAbs(ref.OriginalRef) {
			return m, verified, errors.New("copied reference has no exact absolute producer path")
		}
		original, err := readAuditFile(ref.OriginalRef)
		if err != nil {
			return m, verified, fmt.Errorf("original result bytes unavailable for independent copy check: %w", err)
		}
		file := files[ref.File]
		if int64(len(original)) != file.Bytes || hash(original) != file.SHA256 {
			return m, verified, errors.New("copied file differs from the exact referenced producer result")
		}
	}
	return m, verified, nil
}

func plainPackageFile(root, name string) error {
	if name == "." || name == "" || filepath.IsAbs(name) || filepath.Clean(name) != name || strings.Contains(name, "\\") || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
		return fmt.Errorf("unsafe package member path: %s", name)
	}
	parts := strings.Split(name, string(filepath.Separator))
	path := root
	for i, part := range parts {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (i+1 < len(parts) && !info.IsDir()) || (i+1 == len(parts) && !info.Mode().IsRegular()) {
			return fmt.Errorf("package member is redirected or non-regular: %s", name)
		}
	}
	return nil
}

func inspectExportReferences(m tracecatalog.ExportManifest, files map[string]tracecatalog.ExportFile) error {
	coverage := tracecatalog.ExecutionCoverage{Basis: "observed_queries_only", PlannedOutcomes: map[tracecatalog.Outcome]int{}, ObservedOutcomes: map[tracecatalog.Outcome]int{}}
	type key struct{ query, attempt, role string }
	expectedRefs := map[key]string{}
	seenQueries, seenAttempts := map[string]bool{}, map[string]bool{}
	for _, q := range m.Catalog.Queries {
		if q.ID == "" || seenQueries[q.ID] {
			return errors.New("empty or duplicate exported query identity")
		}
		seenQueries[q.ID] = true
		switch m.QueryOrigins[q.ID] {
		case tracecatalog.QueryOriginDeclared:
			coverage.DeclaredPlans++
			coverage.PlannedOutcomes[q.Outcome]++
		case tracecatalog.QueryOriginResumed:
			coverage.ResumedPlans++
			coverage.PlannedOutcomes[q.Outcome]++
		case tracecatalog.QueryOriginObserved:
			coverage.ObservedQueries++
			coverage.ObservedOutcomes[q.Outcome]++
		default:
			return errors.New("exported query has missing or unknown current origin")
		}
		coverage.Attempts += len(q.Attempts)
		if len(q.Attempts) == 0 && m.MaterialComplete {
			return errors.New("unexecuted query labeled material complete")
		}
		for _, attempt := range q.Attempts {
			if attempt.ID == "" || seenAttempts[attempt.ID] {
				return errors.New("empty or duplicate exported attempt identity")
			}
			seenAttempts[attempt.ID] = true
			if m.MaterialComplete && (attempt.Truncated || (attempt.PayloadRef == "" && attempt.RawRef == "")) {
				return errors.New("missing or truncated attempt labeled material complete")
			}
			for role, ref := range map[string]string{"native_payload": attempt.PayloadRef, "tool_raw": attempt.RawRef} {
				if ref != "" {
					expectedRefs[key{q.ID, attempt.ID, role}] = ref
				}
			}
		}
	}
	if len(m.QueryOrigins) != len(seenQueries) {
		return errors.New("query origins do not match exact current roster")
	}
	if coverage.DeclaredPlans+coverage.ResumedPlans > 0 {
		coverage.Basis = "registered_current_plans"
	}
	if !reflect.DeepEqual(coverage, m.ExecutionCoverage) {
		return errors.New("execution coverage differs from exact current plans/attempts")
	}
	usedFiles := map[string]bool{"report.md": true}
	for _, ref := range m.References {
		id := key{ref.QueryID, ref.AttemptID, ref.Role}
		original, ok := expectedRefs[id]
		if !ok || original != ref.OriginalRef {
			return errors.New("reference does not match one exact current query/attempt/role")
		}
		delete(expectedRefs, id)
		switch ref.Status {
		case "copied":
			if _, ok := files[ref.File]; !ok || ref.File == "report.md" {
				return errors.New("copied reference has no result file")
			}
			usedFiles[ref.File] = true
		case "missing", "stale_reference", "omitted_budget":
			if m.MaterialComplete || ref.File != "" || ref.Reason == "" {
				return errors.New("non-copied reference has inconsistent completeness/file/reason")
			}
		default:
			return errors.New("unknown exported reference status")
		}
	}
	if len(expectedRefs) != 0 || len(usedFiles) != len(files) {
		return errors.New("export omits an attempt reference or has an unowned result file")
	}
	return nil
}

// Slice order is not execution identity. Attempts remain separately identified;
// repeated real queries are never rejected just because their count exceeds 4.
func sameCurrentNavigation(a, b tracecatalog.Snapshot) bool {
	normalize := func(s tracecatalog.Snapshot) any {
		s.History = nil
		s.Discovery = tracecatalog.Discovery{}
		s.Artifacts = append([]tracecatalog.Artifact(nil), s.Artifacts...)
		s.Queries = append([]tracecatalog.QueryRecord(nil), s.Queries...)
		sort.Slice(s.Artifacts, func(i, j int) bool { return s.Artifacts[i].ID < s.Artifacts[j].ID })
		sort.Slice(s.Queries, func(i, j int) bool { return s.Queries[i].ID < s.Queries[j].ID })
		for i := range s.Queries {
			s.Queries[i].Attempts = append([]tracecatalog.Attempt(nil), s.Queries[i].Attempts...)
			sort.Slice(s.Queries[i].Attempts, func(j, k int) bool { return s.Queries[i].Attempts[j].ID < s.Queries[i].Attempts[k].ID })
		}
		data, _ := json.Marshal(s)
		var decoded any
		_ = json.Unmarshal(data, &decoded)
		return decoded
	}
	return reflect.DeepEqual(normalize(a), normalize(b))
}
