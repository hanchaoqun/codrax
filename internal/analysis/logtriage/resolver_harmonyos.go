package logtriage

import (
	"path/filepath"
	"strings"
)

// ResolveArkTSFile ranks candidate repo-relative paths against an
// ArkTS frame's basename and returns matches in descending priority.
//
// Ranking tiers (mirrors ResolveJavaFile structure so the validator
// can treat both languages with the same harness):
//
//  1. Canonical Stage Model layout: `entry/src/main/ets/...<basename>`
//     — this is the layout the DevEco Studio Stage Model generator
//     scaffolds for every ArkTS application.
//  2. Common module layout: `<module>/src/main/ets/...<basename>`
//     or `commons/<...>/src/main/ets/...<basename>`. HarmonyOS apps
//     commonly carve shared components into a sibling `commons/`.
//  3. Baseline: basename matches but neither Stage Model root nor
//     commons root is observable on the path.
//
// The function does not call os.Stat — the caller owns verification.
// baseName must already be the bare filename ("Index.ets"); callers
// that hold a dir/basename pair should call filepath.Base first.
func ResolveArkTSFile(baseName string, candidates []string) []string {
	if baseName == "" || len(candidates) == 0 {
		return nil
	}

	type scored struct {
		path string
		tier int
		sort int
	}
	var hits []scored
	seen := make(map[string]bool, len(candidates))

	for _, cand := range candidates {
		if seen[cand] {
			continue
		}
		seen[cand] = true
		if filepath.Base(cand) != baseName {
			continue
		}
		norm := filepath.ToSlash(cand)

		tier := 3
		switch {
		case strings.Contains(norm, "entry/src/main/ets/"):
			tier = 1
		case strings.Contains(norm, "/src/main/ets/"):
			tier = 2
		case strings.Contains(norm, "commons/") && strings.Contains(norm, "/src/main/ets/"):
			tier = 2
		}
		hits = append(hits, scored{path: cand, tier: tier, sort: len(norm)})
	}
	if len(hits) == 0 {
		return nil
	}

	// In-place insertion sort by (tier asc, sort asc, path asc) —
	// same shape as ResolveJavaFile so the behaviour is symmetric.
	for i := 0; i < len(hits); i++ {
		for j := i + 1; j < len(hits); j++ {
			if hits[j].tier < hits[i].tier ||
				(hits[j].tier == hits[i].tier && hits[j].sort < hits[i].sort) ||
				(hits[j].tier == hits[i].tier && hits[j].sort == hits[i].sort && hits[j].path < hits[i].path) {
				hits[i], hits[j] = hits[j], hits[i]
			}
		}
	}
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.path)
	}
	return out
}

// ResolveCangjieFile ranks candidate repo-relative paths against a
// Cangjie frame's (optional package, basename) pair.
//
// Ranking tiers:
//
//  1. Exact package-path match: the file's directory ends with the
//     package-as-path form. `demo.cart` → `src/cart/` or `cart/`.
//  2. Source-root heuristics: file under `src/`, `cangjie/`, or
//     `src/main/cangjie/` (DevEco Cangjie modules) and the dir
//     ending matches the package path.
//  3. Baseline: basename match with no package signal.
//
// pkg is allowed to be empty — older Cangjie frames sometimes
// emit just the basename + line, in which case the tier stays at 3.
func ResolveCangjieFile(pkg, baseName string, candidates []string) []string {
	if baseName == "" || len(candidates) == 0 {
		return nil
	}
	pkgPath := strings.ReplaceAll(pkg, ".", "/")

	type scored struct {
		path string
		tier int
		sort int
	}
	var hits []scored
	seen := make(map[string]bool, len(candidates))

	for _, cand := range candidates {
		if seen[cand] {
			continue
		}
		seen[cand] = true
		if filepath.Base(cand) != baseName {
			continue
		}
		norm := filepath.ToSlash(cand)
		dir := filepath.ToSlash(filepath.Dir(norm))

		tier := 3
		if pkgPath != "" {
			if strings.HasSuffix(dir, "/"+pkgPath) || dir == pkgPath {
				tier = 1
			} else {
				for _, root := range []string{"src/main/cangjie/", "src/cangjie/", "cangjie/", "src/"} {
					if idx := strings.Index(norm, root); idx >= 0 {
						after := norm[idx+len(root):]
						afterDir := filepath.ToSlash(filepath.Dir(after))
						if afterDir == pkgPath || strings.HasSuffix(afterDir, pkgPath) {
							tier = 2
							break
						}
					}
				}
			}
		}
		hits = append(hits, scored{path: cand, tier: tier, sort: len(norm)})
	}
	if len(hits) == 0 {
		return nil
	}

	for i := 0; i < len(hits); i++ {
		for j := i + 1; j < len(hits); j++ {
			if hits[j].tier < hits[i].tier ||
				(hits[j].tier == hits[i].tier && hits[j].sort < hits[i].sort) ||
				(hits[j].tier == hits[i].tier && hits[j].sort == hits[i].sort && hits[j].path < hits[i].path) {
				hits[i], hits[j] = hits[j], hits[i]
			}
		}
	}
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.path)
	}
	return out
}

// isArkTSBasename reports whether path looks like a bare ArkTS
// basename (no directory separator, ends with ".ets"). Mirrors
// isJavaBasename for the validator dispatch.
func isArkTSBasename(path string) bool {
	if path == "" {
		return false
	}
	if strings.ContainsAny(path, "/\\") {
		return false
	}
	return strings.HasSuffix(path, ".ets")
}

// isCangjieBasename reports whether path looks like a bare Cangjie
// basename (no directory separator, ends with ".cj"). .cjo files
// are compiled artefacts, never sources — so the validator would
// not expect them in frames; the check is still restricted to .cj
// to keep the scope tight.
func isCangjieBasename(path string) bool {
	if path == "" {
		return false
	}
	if strings.ContainsAny(path, "/\\") {
		return false
	}
	return strings.HasSuffix(path, ".cj")
}

// ResolveKotlinFile ranks candidate repo-relative paths against a
// Kotlin frame's (package, baseName) pair. Kotlin panic frames emitted
// by the JVM on Android look like `at com.example.ui.MainActivity$
// onCreate(MainActivity.kt:42)` — the file section is typically
// truncated to the basename, and the package is carried in the
// fully-qualified class name before it.
//
// Reuses the same tier structure as ResolveJavaFile: (1) package-path
// match, (2) source-root heuristic, (3) basename baseline. The
// source roots checked include Android Gradle layouts (`src/main/
// kotlin/`, `src/main/java/` — some modules keep .kt inside java/)
// and JB multiplatform layouts (`commonMain/`, `androidMain/`, …).
func ResolveKotlinFile(pkg, baseName string, candidates []string) []string {
	if baseName == "" || len(candidates) == 0 {
		return nil
	}
	pkgPath := strings.ReplaceAll(pkg, ".", "/")

	type scored struct {
		path string
		tier int
		sort int
	}
	var hits []scored
	seen := make(map[string]bool, len(candidates))

	for _, cand := range candidates {
		if seen[cand] {
			continue
		}
		seen[cand] = true
		if filepath.Base(cand) != baseName {
			continue
		}
		norm := filepath.ToSlash(cand)
		dir := filepath.ToSlash(filepath.Dir(norm))

		tier := 3
		if pkgPath != "" {
			if strings.HasSuffix(dir, "/"+pkgPath) || dir == pkgPath {
				tier = 1
			} else {
				for _, root := range []string{
					"src/main/kotlin/",
					"src/main/java/",
					"src/commonMain/kotlin/",
					"src/androidMain/kotlin/",
					"src/jvmMain/kotlin/",
					"src/test/kotlin/",
					"src/",
				} {
					if idx := strings.Index(norm, root); idx >= 0 {
						after := norm[idx+len(root):]
						afterDir := filepath.ToSlash(filepath.Dir(after))
						if afterDir == pkgPath || strings.HasSuffix(afterDir, pkgPath) {
							tier = 2
							break
						}
					}
				}
			}
		}
		hits = append(hits, scored{path: cand, tier: tier, sort: len(norm)})
	}
	if len(hits) == 0 {
		return nil
	}

	for i := 0; i < len(hits); i++ {
		for j := i + 1; j < len(hits); j++ {
			if hits[j].tier < hits[i].tier ||
				(hits[j].tier == hits[i].tier && hits[j].sort < hits[i].sort) ||
				(hits[j].tier == hits[i].tier && hits[j].sort == hits[i].sort && hits[j].path < hits[i].path) {
				hits[i], hits[j] = hits[j], hits[i]
			}
		}
	}
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.path)
	}
	return out
}

// isKotlinBasename reports whether path looks like a bare Kotlin
// basename. Matches both `.kt` (source) and `.kts` (Kotlin script
// — build.gradle.kts etc.). `.kts` files rarely appear in stack
// frames but including them keeps the extension set symmetric
// with the repomap scanner.
func isKotlinBasename(path string) bool {
	if path == "" {
		return false
	}
	if strings.ContainsAny(path, "/\\") {
		return false
	}
	return strings.HasSuffix(path, ".kt") || strings.HasSuffix(path, ".kts")
}
