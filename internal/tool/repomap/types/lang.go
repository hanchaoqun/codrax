package types

import (
	"path/filepath"
	"strings"
	"unicode"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/rust"
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
)

// extToLang maps file extensions to language identifiers.
var extToLang = map[string]string{
	".go":   LangGo,
	".py":   LangPython,
	".pyi":  LangPython,
	".js":   LangJavaScript,
	".jsx":  LangJavaScript,
	".mjs":  LangJavaScript,
	".ts":   LangTypeScript,
	".tsx":  LangTypeScript,
	".java": LangJava,
	".rs":   LangRust,
	".c":    LangC,
	".h":    LangC,
	".cc":   LangCpp,
	".cpp":  LangCpp,
	".cxx":  LangCpp,
	".hpp":  LangCpp,
	".hh":   LangCpp,
}

// DetectLanguage returns the language identifier for a file path,
// or "" if the language is not supported.
func DetectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	return extToLang[ext]
}

// GetSitterLanguage returns the tree-sitter Language for a language name.
// Returns nil for unsupported languages.
func GetSitterLanguage(lang string) *sitter.Language {
	switch lang {
	case LangGo:
		return golang.GetLanguage()
	case LangPython:
		return python.GetLanguage()
	case LangJavaScript:
		return javascript.GetLanguage()
	case LangTypeScript:
		return typescript.GetLanguage()
	case LangJava:
		return java.GetLanguage()
	case LangRust:
		return rust.GetLanguage()
	case LangC:
		return c.GetLanguage()
	case LangCpp:
		return cpp.GetLanguage()
	default:
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
	default:
		return true
	}
}
