package types

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

const DefaultChangePlanSliceMaxChanges = 8
const DefaultOnlineChangePlanSliceMaxChanges = 4

type ChangePlanSliceStatus string

const (
	ChangePlanSlicePending    ChangePlanSliceStatus = "pending"
	ChangePlanSliceApplying   ChangePlanSliceStatus = "applying"
	ChangePlanSliceObserving  ChangePlanSliceStatus = "observing"
	ChangePlanSliceVerified   ChangePlanSliceStatus = "verified"
	ChangePlanSliceUnverified ChangePlanSliceStatus = "unverified"
	ChangePlanSliceFailed     ChangePlanSliceStatus = "failed"
	ChangePlanSliceBlocked    ChangePlanSliceStatus = "blocked"
)

// ChangePlanSlice is the bounded edit/run/observe unit inside a ChangePlan.
// ChangeIndexes are zero-based indexes into ChangePlan.Changes; Paths are a
// normalized, derived convenience view for policy, status cards, and focused
// verification. Hard routing should trust ChangeIndexes plus the owning
// ChangePlan, not planner-authored prose.
type ChangePlanSlice struct {
	ID                 string                `json:"id"`
	Status             ChangePlanSliceStatus `json:"status,omitempty"`
	ChangeIndexes      []int                 `json:"change_indexes,omitempty"`
	Paths              []string              `json:"paths,omitempty"`
	DependsOnSlices    []string              `json:"depends_on_slices,omitempty"`
	ContractRefs       []string              `json:"contract_refs,omitempty"`
	ChangedSymbolRefs  []string              `json:"changed_symbol_refs,omitempty"`
	VerificationProbes []string              `json:"verification_probes,omitempty"`
}

type ChangePlanSliceOptions struct {
	MaxChangesPerSlice       int
	IsolatedPaths            []string
	SplitOnPathRoleBoundary  bool
	SplitOnOwnerPathBoundary bool
}

// OnlineChangePlanSliceOptions returns the commercial write-mode default for
// deterministic edit/run/observe units. It is a scheduling policy only: safety
// approval still comes from typed risk/effect gates, while this helper keeps
// unrelated roles, owner scopes, and sensitive operational surfaces from being
// batched into one runtime unit.
func OnlineChangePlanSliceOptions(plan *ChangePlan) ChangePlanSliceOptions {
	return ChangePlanSliceOptions{
		MaxChangesPerSlice:       DefaultOnlineChangePlanSliceMaxChanges,
		IsolatedPaths:            ChangePlanSliceIsolationPaths(plan),
		SplitOnPathRoleBoundary:  true,
		SplitOnOwnerPathBoundary: true,
	}
}

func ChangePlanSliceIsolationPaths(plan *ChangePlan) []string {
	if plan == nil || len(plan.Changes) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, change := range plan.Changes {
		for _, raw := range []string{change.Path, change.NewPath} {
			p := cleanChangePlanSlicePath(raw)
			if p == "" || !changePlanSlicePathRequiresIsolation(p) {
				continue
			}
			key := strings.ToLower(p)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func DeriveChangePlanSlices(plan *ChangePlan, opts ChangePlanSliceOptions) []ChangePlanSlice {
	if plan == nil || len(plan.Changes) == 0 {
		return nil
	}
	maxChanges := normalizeChangePlanSliceMax(opts.MaxChangesPerSlice)
	isolated := changePlanSlicePathSet(opts.IsolatedPaths)
	var groups [][]int
	current := make([]int, 0, maxChanges)
	currentKey := ""
	flush := func() {
		if len(current) == 0 {
			return
		}
		groups = append(groups, append([]int(nil), current...))
		current = current[:0]
		currentKey = ""
	}
	for i, change := range plan.Changes {
		iso := changePlanSliceChangeIsIsolated(change, isolated)
		key := changePlanSliceGroupingKey(change, opts)
		if iso {
			flush()
		} else if currentKey != "" && key != "" && currentKey != key {
			flush()
		} else if len(current) >= maxChanges {
			flush()
		}
		current = append(current, i)
		if currentKey == "" {
			currentKey = key
		}
		if iso {
			flush()
		}
	}
	flush()

	out := make([]ChangePlanSlice, 0, len(groups))
	for i, indexes := range groups {
		out = append(out, ChangePlanSlice{
			ID:            fmt.Sprintf("slice-%03d", i+1),
			Status:        ChangePlanSlicePending,
			ChangeIndexes: append([]int(nil), indexes...),
			Paths:         changePlanSlicePathsForIndexes(plan, indexes),
		})
	}
	attachChangePlanSliceDependencies(plan, out)
	return out
}

func NormalizeChangePlanSlices(plan *ChangePlan, opts ChangePlanSliceOptions) []ChangePlanSlice {
	if plan == nil || len(plan.Changes) == 0 {
		return nil
	}
	if len(plan.Slices) == 0 {
		return DeriveChangePlanSlices(plan, opts)
	}
	out := make([]ChangePlanSlice, 0, len(plan.Slices))
	for i, slice := range plan.Slices {
		slice.ID = strings.TrimSpace(slice.ID)
		if slice.ID == "" {
			slice.ID = fmt.Sprintf("slice-%03d", i+1)
		}
		slice.Status = normalizeChangePlanSliceStatus(slice.Status)
		if slice.Status == "" {
			slice.Status = ChangePlanSlicePending
		}
		slice.ChangeIndexes = normalizeChangePlanSliceIndexes(slice.ChangeIndexes, len(plan.Changes))
		if len(slice.ChangeIndexes) == 0 {
			continue
		}
		slice.Paths = changePlanSlicePathsForIndexes(plan, slice.ChangeIndexes)
		slice.DependsOnSlices = dedupTrimWriteWorkflowRunStrings(slice.DependsOnSlices)
		slice.ContractRefs = dedupTrimWriteWorkflowRunStrings(slice.ContractRefs)
		slice.ChangedSymbolRefs = dedupTrimWriteWorkflowRunStrings(slice.ChangedSymbolRefs)
		slice.VerificationProbes = dedupTrimWriteWorkflowRunStrings(slice.VerificationProbes)
		out = append(out, slice)
	}
	if len(out) == 0 {
		return DeriveChangePlanSlices(plan, opts)
	}
	attachChangePlanSliceDependencies(plan, out)
	return out
}

func ValidateChangePlanSlices(plan *ChangePlan, slices []ChangePlanSlice) []string {
	if plan == nil {
		if len(slices) == 0 {
			return nil
		}
		return []string{"plan is nil"}
	}
	if len(plan.Changes) == 0 {
		if len(slices) == 0 {
			return nil
		}
		return []string{"plan has no changes"}
	}
	var violations []string
	seenIndexes := make(map[int]string, len(plan.Changes))
	sliceByID := make(map[string]int, len(slices))
	changeSlice := make(map[int]int, len(plan.Changes))
	for si, slice := range slices {
		id := strings.TrimSpace(slice.ID)
		if id == "" {
			violations = append(violations, fmt.Sprintf("slice at index %d has empty id", si))
		} else if prior, ok := sliceByID[id]; ok {
			violations = append(violations, fmt.Sprintf("slice %q duplicates slice at index %d", id, prior))
		}
		sliceByID[id] = si
		if normalizeChangePlanSliceStatus(slice.Status) == "" {
			violations = append(violations, fmt.Sprintf("slice %q has invalid status %q", id, slice.Status))
		}
		if len(slice.ChangeIndexes) == 0 {
			violations = append(violations, fmt.Sprintf("slice %q has no change indexes", id))
			continue
		}
		for _, idx := range slice.ChangeIndexes {
			if idx < 0 || idx >= len(plan.Changes) {
				violations = append(violations, fmt.Sprintf("slice %q references out-of-range change index %d", id, idx))
				continue
			}
			if priorID, ok := seenIndexes[idx]; ok {
				violations = append(violations, fmt.Sprintf("change index %d appears in both %q and %q", idx, priorID, id))
				continue
			}
			seenIndexes[idx] = id
			changeSlice[idx] = si
		}
	}
	for i := range plan.Changes {
		if _, ok := seenIndexes[i]; !ok {
			violations = append(violations, fmt.Sprintf("change index %d is not assigned to any slice", i))
		}
	}

	pathToIndex := changePlanSlicePathToIndex(plan)
	for depIdx, change := range plan.Changes {
		depSliceIdx, ok := changeSlice[depIdx]
		if !ok {
			continue
		}
		for _, depPath := range change.DependsOn {
			depChangeIdx, ok := pathToIndex[strings.TrimSpace(depPath)]
			if !ok {
				violations = append(violations, fmt.Sprintf("change index %d depends on unknown path %q", depIdx, depPath))
				continue
			}
			reqSliceIdx, ok := changeSlice[depChangeIdx]
			if !ok {
				continue
			}
			if reqSliceIdx > depSliceIdx {
				violations = append(violations, fmt.Sprintf("change index %d depends on later slice path %q", depIdx, depPath))
			}
		}
	}
	return violations
}

func ActiveChangePlanApplySlice(plan *ChangePlan, run *WriteWorkflowRun) (ChangePlanSlice, bool) {
	if plan == nil || run == nil {
		return ChangePlanSlice{}, false
	}
	normalizedRun := NormalizeWriteWorkflowRun(*run)
	batch, ok := writeWorkflowActiveBatch(normalizedRun)
	if !ok {
		return ChangePlanSlice{}, false
	}
	activeID := strings.TrimSpace(batch.ActiveSliceID)
	slices := batch.Slices
	if len(slices) == 0 && activeID != "" {
		slices = writeWorkflowSlicesFromPlanSlices(NormalizeChangePlanSlices(plan, ChangePlanSliceOptions{}), plan.ID)
	}
	if len(slices) == 0 {
		return ChangePlanSlice{}, false
	}
	if activeID == "" {
		for _, slice := range slices {
			if changePlanSliceStatusIsTerminal(slice.Status) {
				continue
			}
			activeID = strings.TrimSpace(slice.ID)
			break
		}
	}
	if activeID == "" {
		return ChangePlanSlice{}, false
	}
	for _, slice := range slices {
		if strings.TrimSpace(slice.ID) != activeID {
			continue
		}
		out := ChangePlanSlice{
			ID:                strings.TrimSpace(slice.ID),
			Status:            normalizeChangePlanSliceStatus(slice.Status),
			ChangeIndexes:     normalizeChangePlanSliceIndexes(slice.ChangeIndexes, len(plan.Changes)),
			DependsOnSlices:   dedupTrimWriteWorkflowRunStrings(slice.DependsOnSlices),
			ContractRefs:      nil,
			ChangedSymbolRefs: nil,
		}
		out.Paths = changePlanSlicePathsForIndexes(plan, out.ChangeIndexes)
		if out.Status == "" {
			out.Status = ChangePlanSlicePending
		}
		if len(out.ChangeIndexes) == 0 {
			return ChangePlanSlice{}, false
		}
		return out, true
	}
	return ChangePlanSlice{}, false
}

func ActiveChangePlanApplyTargetPaths(plan *ChangePlan, run *WriteWorkflowRun) ([]string, bool) {
	slice, ok := ActiveChangePlanApplySlice(plan, run)
	if !ok {
		return changePlanAllTargetPaths(plan), false
	}
	return changePlanSlicePathsForIndexes(plan, slice.ChangeIndexes), true
}

func ActiveChangePlanApplyChanges(plan *ChangePlan, run *WriteWorkflowRun) ([]FileChange, bool) {
	slice, ok := ActiveChangePlanApplySlice(plan, run)
	if !ok {
		if plan == nil {
			return nil, false
		}
		return append([]FileChange(nil), plan.Changes...), false
	}
	out := make([]FileChange, 0, len(slice.ChangeIndexes))
	for _, idx := range slice.ChangeIndexes {
		if idx < 0 || idx >= len(plan.Changes) {
			continue
		}
		out = append(out, plan.Changes[idx])
	}
	return out, true
}

func WriteWorkflowSlicesFromChangePlan(plan *ChangePlan) []WriteWorkflowSlice {
	if plan == nil {
		return nil
	}
	return writeWorkflowSlicesFromPlanSlices(NormalizeChangePlanSlices(plan, ChangePlanSliceOptions{}), plan.ID)
}

func normalizeChangePlanSliceStatus(in ChangePlanSliceStatus) ChangePlanSliceStatus {
	switch in {
	case ChangePlanSlicePending, ChangePlanSliceApplying, ChangePlanSliceObserving,
		ChangePlanSliceVerified, ChangePlanSliceUnverified, ChangePlanSliceFailed,
		ChangePlanSliceBlocked:
		return in
	default:
		return ""
	}
}

func normalizeChangePlanSliceMax(raw int) int {
	if raw <= 0 {
		return DefaultChangePlanSliceMaxChanges
	}
	return raw
}

func normalizeChangePlanSliceIndexes(indexes []int, changeCount int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(indexes))
	for _, idx := range indexes {
		if idx < 0 || idx >= changeCount {
			continue
		}
		if _, ok := seen[idx]; ok {
			continue
		}
		seen[idx] = struct{}{}
		out = append(out, idx)
	}
	sort.Ints(out)
	return out
}

func attachChangePlanSliceDependencies(plan *ChangePlan, slices []ChangePlanSlice) {
	if plan == nil || len(slices) == 0 {
		return
	}
	pathToIndex := changePlanSlicePathToIndex(plan)
	changeToSliceID := map[int]string{}
	for _, slice := range slices {
		for _, idx := range slice.ChangeIndexes {
			changeToSliceID[idx] = slice.ID
		}
	}
	for si := range slices {
		deps := make([]string, 0)
		seen := map[string]struct{}{}
		for _, idx := range slices[si].ChangeIndexes {
			if idx < 0 || idx >= len(plan.Changes) {
				continue
			}
			for _, depPath := range plan.Changes[idx].DependsOn {
				depIdx, ok := pathToIndex[strings.TrimSpace(depPath)]
				if !ok {
					continue
				}
				depSliceID := changeToSliceID[depIdx]
				if depSliceID == "" || depSliceID == slices[si].ID {
					continue
				}
				key := strings.ToLower(depSliceID)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				deps = append(deps, depSliceID)
			}
		}
		slices[si].DependsOnSlices = deps
	}
}

func changePlanSlicePathToIndex(plan *ChangePlan) map[string]int {
	out := make(map[string]int, len(plan.Changes)*2)
	for i, change := range plan.Changes {
		if path := strings.TrimSpace(change.Path); path != "" {
			out[path] = i
		}
		if path := strings.TrimSpace(change.NewPath); path != "" {
			out[path] = i
		}
	}
	return out
}

func changePlanSlicePathsForIndexes(plan *ChangePlan, indexes []int) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(indexes))
	for _, idx := range indexes {
		if plan == nil || idx < 0 || idx >= len(plan.Changes) {
			continue
		}
		change := plan.Changes[idx]
		for _, path := range []string{change.Path, change.NewPath} {
			path = strings.TrimSpace(path)
			if path == "" {
				continue
			}
			key := strings.ToLower(path)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, path)
		}
	}
	return out
}

func changePlanSlicePathSet(paths []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		out[strings.ToLower(path)] = struct{}{}
	}
	return out
}

func changePlanSliceChangeIsIsolated(change FileChange, isolated map[string]struct{}) bool {
	for _, path := range []string{change.Path, change.NewPath} {
		path = cleanChangePlanSlicePath(path)
		if path == "" {
			continue
		}
		if len(isolated) > 0 {
			if _, ok := isolated[strings.ToLower(path)]; ok {
				return true
			}
		}
		if changePlanSlicePathRequiresIsolation(path) {
			return true
		}
	}
	return false
}

func changePlanSliceGroupingKey(change FileChange, opts ChangePlanSliceOptions) string {
	if !opts.SplitOnPathRoleBoundary && !opts.SplitOnOwnerPathBoundary {
		return ""
	}
	primary := cleanChangePlanSlicePath(firstChangePlanSlicePath(change))
	if primary == "" {
		return ""
	}
	var parts []string
	if opts.SplitOnPathRoleBoundary {
		parts = append(parts, string(ClassifySourcePathRole(primary)))
	}
	if opts.SplitOnOwnerPathBoundary {
		parts = append(parts, changePlanSliceOwnerPathKey(primary))
	}
	return strings.Join(parts, "\x00")
}

func firstChangePlanSlicePath(change FileChange) string {
	for _, raw := range []string{change.Path, change.NewPath} {
		if p := cleanChangePlanSlicePath(raw); p != "" {
			return p
		}
	}
	return ""
}

func cleanChangePlanSlicePath(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
	if raw == "" || raw == "." {
		return ""
	}
	clean := path.Clean(raw)
	if clean == "." {
		return ""
	}
	return strings.TrimPrefix(clean, "./")
}

func changePlanSliceOwnerPathKey(rel string) string {
	rel = cleanChangePlanSlicePath(rel)
	if rel == "" {
		return ""
	}
	dir := path.Dir(rel)
	if dir == "." || dir == "/" {
		return ""
	}
	parts := strings.Split(strings.Trim(dir, "/"), "/")
	if len(parts) <= 2 {
		return strings.Join(parts, "/")
	}
	return strings.Join(parts[:2], "/")
}

func changePlanSlicePathRequiresIsolation(rel string) bool {
	rel = strings.ToLower(cleanChangePlanSlicePath(rel))
	if rel == "" {
		return false
	}
	base := path.Base(rel)
	if strings.HasPrefix(rel, ".github/workflows/") ||
		strings.HasPrefix(rel, ".github/actions/") ||
		strings.HasPrefix(rel, ".githooks/") ||
		strings.HasPrefix(rel, ".husky/") ||
		strings.Contains(rel, "/migrations/") ||
		strings.Contains(rel, "/database/migrations/") {
		return true
	}
	switch base {
	case "go.mod", "go.sum",
		"package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock",
		"cargo.toml", "cargo.lock", "pom.xml", "build.gradle", "settings.gradle",
		"requirements.txt", "pyproject.toml", "poetry.lock", "pipfile", "pipfile.lock",
		"dockerfile", "docker-compose.yml", "androidmanifest.xml",
		"codrax.yaml", "providers.yaml":
		return true
	}
	switch path.Ext(base) {
	case ".pem", ".key", ".p12", ".pfx":
		return true
	default:
		return false
	}
}

func writeWorkflowSlicesFromPlanSlices(slices []ChangePlanSlice, planID string) []WriteWorkflowSlice {
	out := make([]WriteWorkflowSlice, 0, len(slices))
	for _, slice := range slices {
		id := strings.TrimSpace(slice.ID)
		if id == "" || len(slice.ChangeIndexes) == 0 {
			continue
		}
		status := normalizeChangePlanSliceStatus(slice.Status)
		if status == "" {
			status = ChangePlanSlicePending
		}
		out = append(out, WriteWorkflowSlice{
			ID:              id,
			Status:          status,
			PlanID:          strings.TrimSpace(planID),
			ChangeIndexes:   append([]int(nil), slice.ChangeIndexes...),
			Paths:           append([]string(nil), slice.Paths...),
			DependsOnSlices: append([]string(nil), slice.DependsOnSlices...),
		})
	}
	return out
}

func changePlanSliceStatusIsTerminal(status ChangePlanSliceStatus) bool {
	switch normalizeChangePlanSliceStatus(status) {
	case ChangePlanSliceVerified, ChangePlanSliceUnverified, ChangePlanSliceFailed, ChangePlanSliceBlocked:
		return true
	default:
		return false
	}
}

func changePlanAllTargetPaths(plan *ChangePlan) []string {
	if plan == nil {
		return nil
	}
	return append([]string(nil), plan.TargetPaths...)
}
