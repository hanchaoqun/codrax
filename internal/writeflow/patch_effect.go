package writeflow

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
	"gopkg.in/yaml.v3"
)

var unifiedDiffHunkHeaderRE = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@`)

func PatchEffectRecordFromUnifiedDiff(planID, sliceID, source, baseRef, headRef, diff string) types.PatchEffectRecord {
	record := types.PatchEffectRecord{
		PlanID:          strings.TrimSpace(planID),
		SliceID:         strings.TrimSpace(sliceID),
		Source:          strings.TrimSpace(source),
		BaseRef:         strings.TrimSpace(baseRef),
		HeadRef:         strings.TrimSpace(headRef),
		DiffFingerprint: patchEffectFingerprint(diff),
		DiffBytes:       len([]byte(diff)),
		CreatedAt:       time.Now(),
	}
	if record.Source == "" {
		record.Source = "unified_diff"
	}
	record.RecordID = patchEffectRecordID(record.PlanID, record.SliceID, record.DiffFingerprint)
	var current *types.PatchEffectFile
	var currentHunk *types.PatchEffectHunk
	flushFile := func() {
		if current == nil {
			return
		}
		if currentHunk != nil {
			current.Hunks = append(current.Hunks, *currentHunk)
			currentHunk = nil
		}
		normalizePatchEffectFile(current)
		if current.Path != "" || current.OldPath != "" {
			record.Files = append(record.Files, *current)
		}
		current = nil
	}
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			flushFile()
			oldPath, newPath := parsePatchEffectDiffGitLine(line)
			current = &types.PatchEffectFile{
				Path:    newPath,
				OldPath: oldPath,
				Status:  "modified",
			}
			continue
		}
		if current == nil {
			continue
		}
		switch {
		case strings.HasPrefix(line, "new file mode "):
			current.Status = "created"
		case strings.HasPrefix(line, "deleted file mode "):
			current.Status = "deleted"
		case strings.HasPrefix(line, "rename from "):
			current.Status = "renamed"
			current.OldPath = normalizePatchEffectPath(strings.TrimPrefix(line, "rename from "))
		case strings.HasPrefix(line, "rename to "):
			current.Status = "renamed"
			current.Path = normalizePatchEffectPath(strings.TrimPrefix(line, "rename to "))
		case strings.HasPrefix(line, "--- "):
			oldPath := normalizePatchEffectDiffPath(strings.TrimPrefix(line, "--- "))
			if oldPath != "" {
				current.OldPath = oldPath
			}
		case strings.HasPrefix(line, "+++ "):
			newPath := normalizePatchEffectDiffPath(strings.TrimPrefix(line, "+++ "))
			if newPath != "" {
				current.Path = newPath
			}
		case strings.HasPrefix(line, "@@ "):
			if currentHunk != nil {
				current.Hunks = append(current.Hunks, *currentHunk)
			}
			hunk := parsePatchEffectHunkHeader(line)
			currentHunk = &hunk
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			current.AddedLines++
			if currentHunk != nil {
				currentHunk.AddedLines++
			}
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			current.RemovedLines++
			if currentHunk != nil {
				currentHunk.RemovedLines++
			}
		}
	}
	flushFile()
	record.Files = normalizePatchEffectFiles(record.Files)
	return record
}

func parsePatchEffectDiffGitLine(line string) (string, string) {
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) < 4 {
		return "", ""
	}
	return normalizePatchEffectDiffPath(parts[2]), normalizePatchEffectDiffPath(parts[3])
}

func parsePatchEffectHunkHeader(line string) types.PatchEffectHunk {
	match := unifiedDiffHunkHeaderRE.FindStringSubmatch(line)
	if len(match) == 0 {
		return types.PatchEffectHunk{}
	}
	return types.PatchEffectHunk{
		OldStart: parsePatchEffectInt(match[1], 0),
		OldLines: parsePatchEffectIntDefault(match[2], 1),
		NewStart: parsePatchEffectInt(match[3], 0),
		NewLines: parsePatchEffectIntDefault(match[4], 1),
	}
}

func parsePatchEffectInt(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

func parsePatchEffectIntDefault(raw string, fallback int) int {
	return parsePatchEffectInt(raw, fallback)
}

func normalizePatchEffectFiles(in []types.PatchEffectFile) []types.PatchEffectFile {
	out := make([]types.PatchEffectFile, 0, len(in))
	seen := map[string]int{}
	for _, file := range in {
		normalizePatchEffectFile(&file)
		key := file.OldPath + "->" + file.Path + ":" + file.Status
		if idx, ok := seen[key]; ok {
			out[idx].AddedLines += file.AddedLines
			out[idx].RemovedLines += file.RemovedLines
			out[idx].Hunks = append(out[idx].Hunks, file.Hunks...)
			continue
		}
		seen[key] = len(out)
		out = append(out, file)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].OldPath < out[j].OldPath
	})
	return out
}

func normalizePatchEffectFile(file *types.PatchEffectFile) {
	if file == nil {
		return
	}
	file.Path = normalizePatchEffectPath(file.Path)
	file.OldPath = normalizePatchEffectPath(file.OldPath)
	if file.Path == "" && file.OldPath != "" {
		file.Path = file.OldPath
	}
	file.Status = strings.TrimSpace(file.Status)
	if file.Status == "" {
		file.Status = "modified"
	}
	ext := strings.ToLower(filepath.Ext(file.Path))
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(file.OldPath))
	}
	file.Language = strings.TrimPrefix(ext, ".")
	if file.PathRole == "" {
		file.PathRole = types.ClassifySourcePathRole(file.Path)
	}
}

func normalizePatchEffectDiffPath(raw string) string {
	raw = strings.Trim(strings.TrimSpace(raw), `"`)
	switch raw {
	case "", "/dev/null":
		return ""
	}
	raw = strings.TrimPrefix(raw, "a/")
	raw = strings.TrimPrefix(raw, "b/")
	return normalizePatchEffectPath(raw)
}

func normalizePatchEffectPath(raw string) string {
	path := filepath.ToSlash(strings.TrimSpace(raw))
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		return ""
	}
	return path
}

func patchEffectFingerprint(diff string) string {
	sum := sha256.Sum256([]byte(diff))
	return hex.EncodeToString(sum[:])
}

func patchEffectRecordID(planID, sliceID, fp string) string {
	planID = strings.TrimSpace(planID)
	sliceID = strings.TrimSpace(sliceID)
	if len(fp) > 12 {
		fp = fp[:12]
	}
	if sliceID == "" {
		return fmt.Sprintf("patch-effect:%s:%s", planID, fp)
	}
	return fmt.Sprintf("patch-effect:%s:%s:%s", planID, sliceID, fp)
}

func AnnotatePatchEffectStructuredFileParses(record *types.PatchEffectRecord, root string) {
	if record == nil || strings.TrimSpace(root) == "" {
		return
	}
	for i := range record.Files {
		file := &record.Files[i]
		if strings.TrimSpace(file.Path) == "" || strings.TrimSpace(file.Status) == "deleted" {
			continue
		}
		kind := patchEffectStructuredFileKind(file.Path)
		if kind == "" {
			continue
		}
		abs, ok := patchEffectSafeRepoPath(root, file.Path)
		if !ok {
			file.Events = append(file.Events, types.PatchEffectEvent{
				Code:     "patch_effect_path_outside_worktree",
				Severity: "error",
				Path:     file.Path,
				Message:  "patch effect path is outside the worktree root",
			})
			continue
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			file.Events = append(file.Events, types.PatchEffectEvent{
				Code:     "structured_file_missing",
				Severity: "error",
				Path:     file.Path,
				Message:  kind + " file could not be read after apply",
			})
			continue
		}
		if err := validatePatchEffectStructuredFile(kind, data); err != nil {
			file.Events = append(file.Events, types.PatchEffectEvent{
				Code:     "structured_file_parse_error",
				Severity: "error",
				Path:     file.Path,
				Message:  kind + " parse failed: " + err.Error(),
			})
		}
	}
}

func patchEffectStructuredFileKind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".xml":
		return "xml"
	default:
		return ""
	}
}

func validatePatchEffectStructuredFile(kind string, data []byte) error {
	switch kind {
	case "json":
		var v any
		return json.Unmarshal(data, &v)
	case "yaml":
		var v any
		return yaml.Unmarshal(data, &v)
	case "xml":
		decoder := xml.NewDecoder(bytes.NewReader(data))
		for {
			if _, err := decoder.Token(); err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
		}
	default:
		return nil
	}
}

func patchEffectSafeRepoPath(root, rel string) (string, bool) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	rel = normalizePatchEffectPath(rel)
	if rel == "" {
		return "", false
	}
	abs, err := filepath.Abs(filepath.Join(rootAbs, filepath.FromSlash(rel)))
	if err != nil {
		return "", false
	}
	if abs == rootAbs || strings.HasPrefix(abs, rootAbs+string(filepath.Separator)) {
		return abs, true
	}
	return "", false
}
