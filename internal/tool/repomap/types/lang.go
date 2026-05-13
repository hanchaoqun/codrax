package types

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/kotlin"
	"github.com/smacker/go-tree-sitter/lua"
	"github.com/smacker/go-tree-sitter/protobuf"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/swift"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// Supported language names.
const (
	LangGo         = "go"
	LangPython     = "python"
	LangJavaScript = "javascript"
	LangTypeScript = "typescript"
	LangJava       = "java"
	LangRust       = "rust"
	LangC          = "c"
	LangCpp        = "cpp"

	// LangArkTS is HarmonyOS ArkTS (strict mode, API 12+ Stable). The
	// .ets ext maps here unconditionally; the .ts ext is dynamically
	// promoted from LangTypeScript to LangArkTS by the scanner only
	// when the project root carries oh-package.json5 — see the
	// red-line L-ArkTS-2 in docs/design/harmonyos_arkts_cangjie_support.md.
	//
	// At parse time the extractor uses tree-sitter-typescript at Tier 1
	// (TypeScript is a strict superset of ArkTS surface) plus an
	// ArkTS-aware post-pass that recognises @Component struct, @Builder,
	// @State and the 21-decorator whitelist.
	LangArkTS = "arkts"

	// LangCangjie is HarmonyOS Cangjie 1.0.0 LTS (cjnative). No
	// neighbour tree-sitter grammar covers Cangjie surface
	// (func / package_clause / extend / match / interface as trait),
	// so the extractor uses a hand-written Go-native top-level decl
	// scanner. The .cjo compiled artefact is denied at scan time
	// (red line L-Cangjie-1) and FileInfo.Package must come from the
	// `package_clause` only, never from path inference (L-Cangjie-2).
	LangCangjie = "cangjie"

	// LangKotlin is Kotlin (Android / JVM / multiplatform). Rides on
	// the smacker/go-tree-sitter/kotlin grammar that ships with the
	// vendored tree-sitter mod. File extensions: .kt (source),
	// .kts (Kotlin script — build.gradle.kts etc.).
	//
	// Stack frames in Android panics are JVM-style (identical to
	// Java frames aside from file ext): `at com.example.Foo$bar(Foo.kt:42)`.
	// Existing Gradle / JUnit XML test runners handle Kotlin out of
	// the box so no new verifier is required — just a skill prompt
	// addition for Kotlin style and a log_triage dispatch branch.
	LangKotlin = "kotlin"

	// LangRuby — Ruby (.rb) via tree-sitter-ruby. detectRunner already
	// handles Gemfile via the rspec runner; this constant pulls Ruby
	// into repomap so structural search / symbol resolution work.
	LangRuby = "ruby"

	// LangSwift — Swift (.swift) via tree-sitter-swift. Surfaces
	// classes/structs/protocols/extensions/funcs for top-level
	// navigation. Test runner support (Package.swift → swift test)
	// is shipped alongside in run_tests.go.
	LangSwift = "swift"

	// LangLua — Lua (.lua) via tree-sitter-lua. Captures top-level
	// "function ..." and "local function ..." plus "function M.foo()"
	// table-key forms common in module-pattern Lua.
	LangLua = "lua"

	// LangProto — Protocol Buffers (.proto) via tree-sitter-protobuf.
	// Exposes message / enum / service / rpc as Symbols so cross-file
	// "where is service X defined" navigation works for gRPC repos.
	LangProto = "proto"
)

// supportedReadLanguages is the authoritative read-mode language
// matrix for repomap-backed code understanding. Keep this list in
// lockstep with extToLang / scanner promotions; downstream language
// coverage tests consume this helper instead of hand-maintaining
// partial language samples.
var supportedReadLanguages = []string{
	LangGo,
	LangPython,
	LangJavaScript,
	LangTypeScript,
	LangJava,
	LangKotlin,
	LangRust,
	LangC,
	LangCpp,
	LangRuby,
	LangSwift,
	LangLua,
	LangProto,
	LangArkTS,
	LangCangjie,
}

// SupportedReadLanguages returns the complete repomap read-mode
// language matrix in stable declaration order. The returned slice is a
// defensive copy so callers can sort or filter without mutating the
// process-wide authority list.
func SupportedReadLanguages() []string {
	out := make([]string, len(supportedReadLanguages))
	copy(out, supportedReadLanguages)
	return out
}

// IsSupportedReadLanguage reports whether lang is present in the
// authoritative repomap read-mode language matrix.
func IsSupportedReadLanguage(lang string) bool {
	for _, declared := range supportedReadLanguages {
		if lang == declared {
			return true
		}
	}
	return false
}

// extToLang maps file extensions to language identifiers.
//
// Note: ".ts" maps to LangTypeScript here. The scanner promotes ".ts"
// files inside an ArkTS project to LangArkTS based on a sibling
// oh-package.json5 — see scanner.go::promoteArkTSExt. Pure TypeScript
// projects therefore keep LangTypeScript and are not polluted.
var extToLang = map[string]string{
	".go":    LangGo,
	".py":    LangPython,
	".pyi":   LangPython,
	".js":    LangJavaScript,
	".jsx":   LangJavaScript,
	".mjs":   LangJavaScript,
	".ts":    LangTypeScript,
	".tsx":   LangTypeScript,
	".java":  LangJava,
	".rs":    LangRust,
	".c":     LangC,
	".h":     LangC,
	".cc":    LangCpp,
	".cpp":   LangCpp,
	".cxx":   LangCpp,
	".hpp":   LangCpp,
	".hh":    LangCpp,
	".ets":   LangArkTS,
	".cj":    LangCangjie,
	".kt":    LangKotlin,
	".kts":   LangKotlin,
	".rb":    LangRuby,
	".swift": LangSwift,
	".lua":   LangLua,
	".proto": LangProto,
	// CUDA reuses the tree-sitter-cpp grammar — surface decls
	// (functions, structs, classes, namespaces) parse identically; the
	// __global__ / __device__ qualifiers register as identifiers and
	// don't break the cpp parser. Lossy for kernel-launch syntax
	// (`<<< >>>`) but that's expression-level, not symbol-level.
	".cu":  LangCpp,
	".cuh": LangCpp,
	// Objective-C / Objective-C++ remap to LangC / LangCpp. The cpp
	// grammar tolerates `@interface` / `@implementation` blocks
	// enough for top-level function symbols and class names to land;
	// `+` / `-` instance methods are a known lossy spot, but the
	// underlying file is still indexed for grep / structural search.
	".m":  LangC,
	".mm": LangCpp,
	// .cjo is intentionally absent: it is a Cangjie compiled artefact
	// and must not enter repomap (red line L-Cangjie-1).
}

// DetectLanguage returns the language identifier for a file path,
// or "" if the language is not supported.
func DetectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	return extToLang[ext]
}

// GetSitterLanguage returns the tree-sitter Language for a language name.
// Returns nil for unsupported languages.
//
// LangArkTS reuses the tree-sitter-typescript parser: TypeScript is a
// strict superset of ArkTS strict-mode surface for what tree-sitter
// can structurally recognise (decorators, classes, modules, types).
// ArkTS-specific symbols (`@Component struct Foo`, `build()`, the 21
// state/UI decorators) are detected by the post-pass in
// extract_arkts.go via tree-sitter queries on the same TS parse tree.
//
// LangCangjie returns nil because no tree-sitter grammar covers
// Cangjie. The Cangjie extractor uses a hand-written Go-native
// scanner (parser.go's switch falls through to extractCangjie when
// language is LangCangjie even with nil sitter Language).
func GetSitterLanguage(lang string) *sitter.Language {
	switch lang {
	case LangGo:
		return golang.GetLanguage()
	case LangPython:
		return python.GetLanguage()
	case LangJavaScript:
		return javascript.GetLanguage()
	case LangTypeScript, LangArkTS:
		// ArkTS rides on TypeScript grammar; the post-pass discriminates.
		return typescript.GetLanguage()
	case LangJava:
		return java.GetLanguage()
	case LangKotlin:
		return kotlin.GetLanguage()
	case LangRust:
		return rust.GetLanguage()
	case LangC:
		return c.GetLanguage()
	case LangCpp:
		return cpp.GetLanguage()
	case LangRuby:
		return ruby.GetLanguage()
	case LangSwift:
		return swift.GetLanguage()
	case LangLua:
		return lua.GetLanguage()
	case LangProto:
		return protobuf.GetLanguage()
	default:
		// LangCangjie + unknowns: nil tells the parser to dispatch to
		// the Go-native extractor (see parser.go).
		return nil
	}
}

// IsExported reports whether a symbol name is considered exported in
// the given language.
func IsExported(lang, name string) bool {
	if name == "" {
		return false
	}
	switch lang {
	case LangGo:
		return unicode.IsUpper(rune(name[0]))
	case LangPython:
		return !strings.HasPrefix(name, "_")
	case LangJava:
		// Java export is access-modifier based; we treat public as exported.
		// The parser sets Exported based on modifier keywords.
		return true
	case LangRust:
		// Rust uses `pub` keyword; parser sets Exported.
		return true
	case LangC, LangCpp:
		// C/C++ header declarations are "exported"; parser sets Exported.
		return true
	case LangJavaScript, LangTypeScript:
		// JS/TS uses export keyword; parser sets Exported.
		return true
	case LangArkTS:
		// ArkTS export rules: top-level `export` keyword OR any
		// component decorator (@Entry / @Component / @Preview etc.)
		// implies module-visible. The extractor sets Exported when
		// either is observed; we default to true so undecorated
		// internal helpers without explicit `export` still surface
		// in graph queries (matches TypeScript behaviour).
		return true
	case LangKotlin:
		// Kotlin's default top-level visibility is `public` unless
		// a `private` / `internal` / `protected` modifier is present.
		// The extractor's per-symbol logic reads the modifier set and
		// sets Exported accordingly; this fallback (used when the
		// extractor could not determine visibility) returns true to
		// match Kotlin's public-by-default convention.
		return true
	case LangCangjie:
		// Cangjie default visibility is package-internal; the
		// extractor must set Exported only when a `public` or `open`
		// modifier is observed. Returning false here is the safe
		// default for the function-scope IsExported helper, which
		// callers use as a fallback when the extractor did not
		// populate Symbol.Exported (e.g. regex Tier 2 fallback).
		return false
	case LangRuby:
		// Ruby visibility is method-level (private / protected); top-
		// level constants and methods are exported by default. The
		// extractor toggles when it sees a `private` directive.
		return true
	case LangSwift:
		// Swift defaults to internal for top-level decls; explicit
		// `public` / `open` make a symbol cross-module visible. The
		// fallback here returns true so undecorated symbols still
		// surface in module-local navigation.
		return true
	case LangLua:
		// Lua scoping is dynamic (`local function` is module-private).
		// The extractor sets Exported based on the `local` keyword.
		return true
	case LangProto:
		// Every Protobuf message / service / RPC is module-visible by
		// the language model — nothing is "private". Default true.
		return true
	default:
		return true
	}
}

// IsArkTSProject reports whether `oh-package.json5` lives at any
// ancestor of `relPath` inside `repoRoot`. Used by the scanner to
// promote `.ts` files in HarmonyOS projects from LangTypeScript to
// LangArkTS without polluting pure TypeScript repositories
// (red line L-ArkTS-2). A nil/empty repoRoot returns false.
//
// Sub-repo boundary rule (L-ArkTS-3): the ancestor walk stops at any
// directory that itself contains `.git` (and is not `repoRoot`),
// because crossing such a boundary leaves the file's owning git
// repository — an `oh-package.json5` in the parent's tree belongs to
// a different sub-repo and must not promote files in this one. This
// is the canonical fix for the multi-repo ArkTS leak even before
// MultiGraph routing lands, so single-repo and multi-repo callers
// share one correct semantic.
//
// Exported because both scanner.go and tests need to consult it.
// Implementation lives next to the language constants so the rule
// stays pinned to one canonical location.
func IsArkTSProject(repoRoot, relPath string) bool {
	if repoRoot == "" {
		return false
	}
	// Walk upward from relPath's dir, looking for oh-package.json5.
	// The cap (12 levels) is the practical depth of HarmonyOS
	// projects (entry/src/main/ets/<feature>/<view>) plus headroom.
	dir := filepath.Dir(relPath)
	for i := 0; i < 12; i++ {
		absDir := filepath.Join(repoRoot, dir)
		if _, err := os.Stat(filepath.Join(absDir, "oh-package.json5")); err == nil {
			return true
		}
		// Sub-repo boundary: if current dir is itself a git repo and
		// is not the repoRoot, stop the walk. Going further would
		// peek into a sibling sub-repo's tree, which is the leak we
		// are guarding against. `.git` may be a directory (independent
		// repo) or a regular file (worktree/submodule pointer) — both
		// count as boundaries.
		if dir != "." && dir != "" {
			if _, err := os.Stat(filepath.Join(absDir, ".git")); err == nil {
				return false
			}
		}
		next := filepath.Dir(dir)
		if next == dir || next == "." || next == string(os.PathSeparator) {
			// Try repo root itself one last time then stop.
			if _, err := os.Stat(filepath.Join(repoRoot, "oh-package.json5")); err == nil {
				return true
			}
			break
		}
		dir = next
	}
	return false
}
