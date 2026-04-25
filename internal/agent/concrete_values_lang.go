package agent

import (
	"strings"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// concrete_values_lang.go — language-aware concrete-value extractors.
//
// Each scanner in this file adds one "kind" of concrete value to the
// output of extractConcreteValues. The scanners are pure functions:
// they take the comment-stripped source lines + a language tag and
// return concreteValueEntry slices. All filtering, capping, and
// dedup are internal so the caller can unconditionally append.
//
// Supported languages (mirrors repomap/types/lang.go):
//   Go, Python, JavaScript, TypeScript, Java, Rust, C, C++

// ── P0.1: conditional ──────────────────────────────────────────────

// scanConditionalPatterns extracts condition guards (if / switch /
// match expressions) from source lines. The emitted "value" is the
// condition expression itself so the Concrete Values table shows
// WHAT activates a code path, not just THAT a branch exists.
//
// Cap: first 4 non-trivial conditions per snippet. Trivial guards
// (err != nil, !ok, == nil, != nil, == false, == true) are skipped
// because they carry zero domain information.
func scanConditionalPatterns(lines []string, lang string) []concreteValueEntry {
	var out []concreteValueEntry
	const cap = 4
	for _, line := range lines {
		if len(out) >= cap {
			break
		}
		trimmed := strings.TrimSpace(line)
		cond := extractConditionExpr(trimmed, lang)
		if cond == "" || isTrivialCondition(cond) {
			continue
		}
		out = append(out, concreteValueEntry{
			kind:  "conditional",
			value: "if " + cond,
		})
	}
	return out
}

// extractConditionExpr pulls the condition expression from an
// if / switch / match / if-let line. Returns "" when the line is
// not a conditional.
func extractConditionExpr(line, lang string) string {
	// Normalize: strip leading keywords like "} else if" → "if"
	if idx := strings.Index(line, "else if"); idx >= 0 {
		line = strings.TrimSpace(line[idx+len("else"):])
	} else if idx := strings.Index(line, "elif "); idx >= 0 && (lang == repotypes.LangPython) {
		line = "if " + strings.TrimSpace(line[idx+len("elif "):])
	}

	switch {
	// if ...
	case strings.HasPrefix(line, "if "):
		return extractBetween(line[3:], lang)
	case strings.HasPrefix(line, "if("):
		return extractBetween(line[2:], lang)

	// switch / match
	case strings.HasPrefix(line, "switch "):
		return "switch " + extractBetween(line[7:], lang)
	case strings.HasPrefix(line, "switch("):
		return "switch" + extractBetween(line[6:], lang)
	case strings.HasPrefix(line, "match "):
		return "match " + extractBetween(line[6:], lang)

	// Rust: if let Pattern = expr
	case strings.HasPrefix(line, "if let "):
		return extractBetween(line[3:], lang)
	}
	return ""
}

// extractBetween returns the expression up to the block-opener
// ({ for C-like, : for Python). Strips balanced parens from C-like
// languages (Java/JS/C/C++ wrap conditions in mandatory parens).
func extractBetween(s, lang string) string {
	// Find the end delimiter.
	end := -1
	switch lang {
	case repotypes.LangPython:
		end = strings.LastIndex(s, ":")
	default:
		end = strings.LastIndex(s, "{")
	}
	if end < 0 {
		// Single-line if without block opener — take the whole rest.
		s = strings.TrimRight(s, " \t;")
		if len(s) > 80 {
			s = s[:80]
		}
		return strings.TrimSpace(s)
	}
	expr := strings.TrimSpace(s[:end])
	// Strip outer parens for C-like: if (cond) → cond
	if len(expr) >= 2 && expr[0] == '(' && expr[len(expr)-1] == ')' {
		expr = expr[1 : len(expr)-1]
	}
	if len(expr) > 80 {
		expr = expr[:80]
	}
	return strings.TrimSpace(expr)
}

// isTrivialCondition filters conditions that carry zero domain
// information. These are the error-check / nil-check guards that
// appear on nearly every line in Go/Rust/Java code and would flood
// the Concrete Values table with noise.
func isTrivialCondition(cond string) bool {
	lower := strings.ToLower(cond)
	for _, trivial := range []string{
		"err != nil", "err == nil", "!ok", "ok",
		"== nil", "!= nil", "== null", "!= null",
		"== true", "== false", "!= true", "!= false",
		"== none", "!= none", "is none", "is not none",
	} {
		if lower == trivial || strings.TrimSpace(lower) == trivial {
			return true
		}
	}
	// Very short (≤ 3 chars): single-var checks like `if x`
	if len(strings.TrimSpace(cond)) <= 3 {
		return true
	}
	return false
}

// ── P0.2: embeds ───────────────────────────────────────────────────

// scanEmbedsPatterns detects type embedding / class inheritance.
//
// | Language   | Pattern                                    |
// |------------|--------------------------------------------|
// | Go         | bare type name in struct/interface body     |
// | Python     | class Foo(Base1, Base2):                   |
// | JS/TS      | class Foo extends Bar                      |
// | TS         | interface I extends J, K                   |
// | Java       | class Foo extends Bar                      |
// | Rust       | trait Sub: Super + Super2                  |
// | C++        | class D : public Base, protected Mixin     |
// | C          | struct Foo { struct Bar base; }  (heuristic)|
func scanEmbedsPatterns(lines []string, lang string) []concreteValueEntry {
	var out []concreteValueEntry
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch lang {
		case repotypes.LangPython:
			// class Foo(Bar, Baz):
			if strings.HasPrefix(trimmed, "class ") {
				if lp := strings.Index(trimmed, "("); lp > 0 {
					if rp := strings.Index(trimmed[lp:], ")"); rp > 0 {
						bases := trimmed[lp+1 : lp+rp]
						for _, b := range strings.Split(bases, ",") {
							b = strings.TrimSpace(b)
							if b != "" && b != "object" && b != "ABC" {
								out = append(out, concreteValueEntry{
									kind: "embeds", value: "inherits " + b,
								})
							}
						}
					}
				}
			}

		case repotypes.LangJavaScript, repotypes.LangTypeScript, repotypes.LangArkTS:
			// class Foo extends Bar { ... }
			// TS also: interface I extends J, K { ... }
			// ArkTS also: @Observed class X extends Y { ... } and
			// struct components do NOT extend (ArkUI inheritance
			// flows via decorator-driven prototype magic, not
			// `extends`), so the same TS pattern suffices.
			if strings.Contains(trimmed, " extends ") {
				if idx := strings.Index(trimmed, " extends "); idx > 0 {
					rest := strings.TrimSpace(trimmed[idx+len(" extends "):])
					// Cut at { or implements
					for _, sep := range []string{"{", " implements "} {
						if ci := strings.Index(rest, sep); ci > 0 {
							rest = rest[:ci]
						}
					}
					for _, b := range strings.Split(rest, ",") {
						b = strings.TrimSpace(b)
						if b != "" {
							out = append(out, concreteValueEntry{
								kind: "embeds", value: "extends " + b,
							})
						}
					}
				}
			}

		case repotypes.LangJava, repotypes.LangKotlin:
			// Java: class Foo extends Bar implements ...
			// Kotlin: class Foo : Bar, IFace { ... } — the `:` is
			// the Kotlin inheritance separator and requires a
			// different extraction path (see the dedicated Kotlin
			// branch below for colon-based inheritance).
			if strings.Contains(trimmed, " extends ") {
				if idx := strings.Index(trimmed, " extends "); idx > 0 {
					rest := strings.TrimSpace(trimmed[idx+len(" extends "):])
					for _, sep := range []string{"{", " implements "} {
						if ci := strings.Index(rest, sep); ci > 0 {
							rest = rest[:ci]
						}
					}
					b := strings.TrimSpace(rest)
					if b != "" {
						out = append(out, concreteValueEntry{
							kind: "embeds", value: "extends " + b,
						})
					}
				}
			}
			if lang == repotypes.LangKotlin &&
				(strings.HasPrefix(trimmed, "class ") ||
					strings.HasPrefix(trimmed, "data class ") ||
					strings.HasPrefix(trimmed, "sealed class ") ||
					strings.HasPrefix(trimmed, "object ") ||
					strings.HasPrefix(trimmed, "interface ")) {
				// Kotlin inheritance separator: after the class name
				// `class Foo : Bar, IFace()`. Strip generics and
				// constructor parens to get the plain type names.
				if colon := strings.Index(trimmed, " : "); colon > 0 {
					rest := trimmed[colon+3:]
					if brace := strings.Index(rest, "{"); brace > 0 {
						rest = rest[:brace]
					}
					if where := strings.Index(rest, " where "); where > 0 {
						rest = rest[:where]
					}
					for _, b := range strings.Split(rest, ",") {
						b = strings.TrimSpace(b)
						if paren := strings.Index(b, "("); paren > 0 {
							b = b[:paren]
						}
						if lt := strings.Index(b, "<"); lt > 0 {
							b = b[:lt]
						}
						b = strings.TrimSpace(b)
						if b != "" {
							out = append(out, concreteValueEntry{
								kind: "embeds", value: "inherits " + b,
							})
						}
					}
				}
			}

		case repotypes.LangCangjie:
			// Cangjie inheritance operator is `<:` (the lexer sees
			// it as `<` immediately followed by `:` — in source
			// text they are adjacent tokens).
			if strings.Contains(trimmed, "<:") {
				if idx := strings.Index(trimmed, "<:"); idx > 0 {
					rest := strings.TrimSpace(trimmed[idx+2:])
					if brace := strings.Index(rest, "{"); brace > 0 {
						rest = rest[:brace]
					}
					if where := strings.Index(rest, " where "); where > 0 {
						rest = rest[:where]
					}
					// Cangjie uses `&` to AND multiple bounds.
					sep := "&"
					if !strings.Contains(rest, "&") && strings.Contains(rest, ",") {
						sep = ","
					}
					for _, b := range strings.Split(rest, sep) {
						b = strings.TrimSpace(b)
						if lt := strings.Index(b, "<"); lt > 0 {
							b = b[:lt]
						}
						b = strings.TrimSpace(b)
						if b != "" {
							out = append(out, concreteValueEntry{
								kind: "embeds", value: "implements " + b,
							})
						}
					}
				}
			}

		case repotypes.LangRust:
			// trait Sub: Super + Super2 { ... }
			if strings.HasPrefix(trimmed, "trait ") || strings.HasPrefix(trimmed, "pub trait ") {
				if ci := strings.Index(trimmed, ":"); ci > 0 {
					rest := trimmed[ci+1:]
					if bi := strings.Index(rest, "{"); bi > 0 {
						rest = rest[:bi]
					}
					for _, b := range strings.Split(rest, "+") {
						b = strings.TrimSpace(b)
						if b != "" {
							out = append(out, concreteValueEntry{
								kind: "embeds", value: "supertrait " + b,
							})
						}
					}
				}
			}

		case repotypes.LangCpp:
			// class Derived : public Base, protected Mixin {
			if (strings.HasPrefix(trimmed, "class ") || strings.HasPrefix(trimmed, "struct ")) &&
				strings.Contains(trimmed, " : ") {
				if ci := strings.Index(trimmed, " : "); ci > 0 {
					rest := trimmed[ci+3:]
					if bi := strings.Index(rest, "{"); bi > 0 {
						rest = rest[:bi]
					}
					for _, b := range strings.Split(rest, ",") {
						b = strings.TrimSpace(b)
						// Strip access specifier
						for _, prefix := range []string{"public ", "protected ", "private ", "virtual "} {
							b = strings.TrimPrefix(b, prefix)
						}
						b = strings.TrimSpace(b)
						if b != "" {
							out = append(out, concreteValueEntry{
								kind: "embeds", value: "inherits " + b,
							})
						}
					}
				}
			}

		case repotypes.LangGo:
			// Go struct embedding: bare type name inside struct body.
			// Heuristic: a line that is just an identifier (optionally
			// with * prefix) and no field name before it.
			if isGoEmbeddedField(trimmed) {
				out = append(out, concreteValueEntry{
					kind: "embeds", value: "embeds " + trimmed,
				})
			}

		case repotypes.LangC:
			// C struct field embedding: "struct Bar base;" inside a
			// struct body. We look for lines where the pattern is
			// "struct <Type> <name>;" — the first such field is the
			// embedding convention in C (e.g. Linux kernel's
			// container_of pattern). Works both as a standalone
			// line and inlined inside a single-line struct def.
			//
			// Scan all tokens of the line for the "struct X name;"
			// sub-pattern (handles both "{ struct Bar base; }" and
			// standalone "struct Bar base;").
			for i := 0; i+2 < len(strings.Fields(trimmed)); i++ {
				fields := strings.Fields(trimmed)
				if fields[i] == "struct" && i+2 < len(fields) {
					typeName := fields[i+1]
					fieldName := strings.TrimRight(fields[i+2], ";,")
					if typeName != "" && fieldName != "" &&
						fieldName != "{" && fieldName != "}" &&
						typeName != strings.TrimRight(trimmed, " {;") {
						out = append(out, concreteValueEntry{
							kind: "embeds", value: "embeds struct " + typeName,
						})
						break
					}
				}
			}
		}
	}
	// Cap
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

// isGoEmbeddedField detects Go embedded fields: a line inside a
// struct body that is just a type name (possibly with * prefix or
// package qualifier), e.g.:
//   ReadOnly
//   *Foo
//   pkg.Bar
//   *pkg.Bar
func isGoEmbeddedField(trimmed string) bool {
	if trimmed == "" || trimmed == "{" || trimmed == "}" {
		return false
	}
	// Must NOT contain spaces (would be a named field: "name Type")
	// Exception: comment suffix
	noComment := trimmed
	if ci := strings.Index(noComment, "//"); ci > 0 {
		noComment = strings.TrimSpace(noComment[:ci])
	}
	if strings.Contains(noComment, " ") || strings.Contains(noComment, "\t") {
		return false
	}
	// Must look like a type reference: starts with upper or * or package.
	s := noComment
	s = strings.TrimPrefix(s, "*")
	if s == "" {
		return false
	}
	// Package-qualified: pkg.Type
	if di := strings.Index(s, "."); di > 0 {
		s = s[di+1:]
	}
	if s == "" {
		return false
	}
	// Must start with uppercase (exported type)
	return s[0] >= 'A' && s[0] <= 'Z'
}

// ── P0.3: implements ───────────────────────────────────────────────

// scanImplementsPatterns detects interface implementation declarations.
//
// | Language   | Pattern                                    |
// |------------|--------------------------------------------|
// | Go         | var _ Interface = (*Type)(nil)              |
// | TS         | class Foo implements I, J                  |
// | Java       | class Foo implements I, J                  |
// | Rust       | impl Trait for Type { ... }                |
// | C++        | (handled via embeds — virtual inheritance)  |
// | Python/JS/C| — no concept, skip                         |
func scanImplementsPatterns(lines []string, lang string) []concreteValueEntry {
	var out []concreteValueEntry
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch lang {
		case repotypes.LangGo:
			// var _ Interface = (*Type)(nil)
			// var _ Interface = &Type{}
			// var _ Interface = Type{}
			if strings.HasPrefix(trimmed, "var _ ") || strings.HasPrefix(trimmed, "var _\t") {
				rest := strings.TrimSpace(trimmed[5:])
				if eqIdx := strings.Index(rest, "="); eqIdx > 0 {
					iface := strings.TrimSpace(rest[:eqIdx])
					if iface != "" {
						out = append(out, concreteValueEntry{
							kind: "implements", value: "implements " + iface,
						})
					}
				}
			}

		case repotypes.LangTypeScript, repotypes.LangArkTS, repotypes.LangJava:
			// class Foo implements I, J { ... }
			// ArkTS also uses `implements` for TS-shaped classes
			// (non-@Component utility classes).
			if strings.Contains(trimmed, " implements ") {
				if idx := strings.Index(trimmed, " implements "); idx > 0 {
					rest := strings.TrimSpace(trimmed[idx+len(" implements "):])
					if bi := strings.Index(rest, "{"); bi > 0 {
						rest = rest[:bi]
					}
					for _, iface := range strings.Split(rest, ",") {
						iface = strings.TrimSpace(iface)
						if iface != "" {
							out = append(out, concreteValueEntry{
								kind: "implements", value: "implements " + iface,
							})
						}
					}
				}
			}

		case repotypes.LangKotlin:
			// Kotlin does not distinguish extends / implements at
			// the keyword level: `class Foo : Base, IFace` handles
			// both. The embeds pass above already emits `inherits`
			// entries; to surface explicit interface implementations
			// we re-scan here for any `: IFace` where `IFace` has
			// no constructor parens (heuristic — classes use parens
			// in Kotlin inheritance: `: Base()` vs `: IFace`).
			if strings.Contains(trimmed, " : ") {
				if colon := strings.Index(trimmed, " : "); colon > 0 {
					rest := strings.TrimSpace(trimmed[colon+3:])
					if brace := strings.Index(rest, "{"); brace > 0 {
						rest = rest[:brace]
					}
					if where := strings.Index(rest, " where "); where > 0 {
						rest = rest[:where]
					}
					for _, b := range strings.Split(rest, ",") {
						b = strings.TrimSpace(b)
						if strings.Contains(b, "(") {
							continue // class super, handled as embeds
						}
						if lt := strings.Index(b, "<"); lt > 0 {
							b = b[:lt]
						}
						b = strings.TrimSpace(b)
						if b != "" {
							out = append(out, concreteValueEntry{
								kind: "implements", value: "implements " + b,
							})
						}
					}
				}
			}

		case repotypes.LangCangjie:
			// Cangjie: `extend Type <: Trait` at the source line
			// already produces an `implements` entry via the
			// embeds pass above (the `<:` matcher). An explicit
			// `implements` keyword does not exist in Cangjie.

		case repotypes.LangRust:
			// impl Trait for Type { ... }
			if strings.HasPrefix(trimmed, "impl ") || strings.HasPrefix(trimmed, "pub impl ") {
				s := trimmed
				s = strings.TrimPrefix(s, "pub ")
				s = strings.TrimPrefix(s, "impl ")
				if forIdx := strings.Index(s, " for "); forIdx > 0 {
					trait := strings.TrimSpace(s[:forIdx])
					rest := strings.TrimSpace(s[forIdx+5:])
					if bi := strings.Index(rest, "{"); bi > 0 {
						rest = rest[:bi]
					}
					// Strip generics: <...>
					if gi := strings.Index(rest, "<"); gi > 0 {
						rest = rest[:gi]
					}
					typ := strings.TrimSpace(rest)
					if trait != "" && typ != "" {
						out = append(out, concreteValueEntry{
							kind:  "implements",
							value: typ + " implements " + trait,
						})
					}
				}
			}
		}
	}
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

// ── P0.4: errors ───────────────────────────────────────────────────

// scanErrorsPatterns detects error construction and signaling.
//
// | Language   | Pattern                                    |
// |------------|--------------------------------------------|
// | Go         | return fmt.Errorf / errors.New / panic     |
// | Python     | raise XxxError(...)                        |
// | JS/TS      | throw new Error(...)                       |
// | Java       | throw new XxxException(...)                |
// | Rust       | return Err(...) / panic!(...)               |
// | C++        | throw std::xxx(...) / throw T{}            |
// | C          | — no clean pattern, skip                   |
func scanErrorsPatterns(lines []string, lang string) []concreteValueEntry {
	var out []concreteValueEntry
	const cap = 4
	for _, line := range lines {
		if len(out) >= cap {
			break
		}
		trimmed := strings.TrimSpace(line)
		switch lang {
		case repotypes.LangGo:
			// return fmt.Errorf(...)
			if strings.Contains(trimmed, "fmt.Errorf(") ||
				strings.Contains(trimmed, "errors.New(") ||
				strings.Contains(trimmed, "errors.Wrap(") ||
				strings.Contains(trimmed, "errors.Wrapf(") {
				msg := extractErrorMessage(trimmed)
				out = append(out, concreteValueEntry{
					kind: "errors", value: "error: " + msg,
				})
			} else if strings.HasPrefix(trimmed, "panic(") || strings.Contains(trimmed, " panic(") {
				msg := extractErrorMessage(trimmed)
				out = append(out, concreteValueEntry{
					kind: "errors", value: "panic: " + msg,
				})
			}

		case repotypes.LangPython:
			// raise ValueError("...")
			if strings.HasPrefix(trimmed, "raise ") {
				rest := strings.TrimPrefix(trimmed, "raise ")
				if len(rest) > 60 {
					rest = rest[:60]
				}
				out = append(out, concreteValueEntry{
					kind: "errors", value: "raises " + strings.TrimSpace(rest),
				})
			}

		case repotypes.LangJavaScript, repotypes.LangTypeScript, repotypes.LangArkTS:
			// throw new Error("...")
			// ArkTS uses the same throw syntax as TypeScript.
			if strings.HasPrefix(trimmed, "throw ") {
				rest := strings.TrimPrefix(trimmed, "throw ")
				rest = strings.TrimRight(rest, ";")
				if len(rest) > 60 {
					rest = rest[:60]
				}
				out = append(out, concreteValueEntry{
					kind: "errors", value: "throws " + strings.TrimSpace(rest),
				})
			}

		case repotypes.LangJava, repotypes.LangKotlin:
			// throw new XxxException("...") in Java; `throw
			// RuntimeException(...)` in Kotlin (no `new` keyword).
			if strings.HasPrefix(trimmed, "throw ") {
				rest := strings.TrimPrefix(trimmed, "throw ")
				rest = strings.TrimRight(rest, ";")
				if len(rest) > 60 {
					rest = rest[:60]
				}
				out = append(out, concreteValueEntry{
					kind: "errors", value: "throws " + strings.TrimSpace(rest),
				})
			}

		case repotypes.LangCangjie:
			// Cangjie raises exceptions with `throw Exception(...)`
			// (similar to Java syntax, no `new` keyword). `panic`
			// is a function call like Go for unrecoverable errors.
			if strings.HasPrefix(trimmed, "throw ") {
				rest := strings.TrimPrefix(trimmed, "throw ")
				if len(rest) > 60 {
					rest = rest[:60]
				}
				out = append(out, concreteValueEntry{
					kind: "errors", value: "throws " + strings.TrimSpace(rest),
				})
			} else if strings.Contains(trimmed, "panic(") {
				msg := extractErrorMessage(trimmed)
				out = append(out, concreteValueEntry{
					kind: "errors", value: "panic: " + msg,
				})
			}

		case repotypes.LangRust:
			// return Err(...)
			if strings.Contains(trimmed, "Err(") &&
				(strings.HasPrefix(trimmed, "return ") ||
					strings.HasPrefix(trimmed, "Err(")) {
				msg := extractErrorMessage(trimmed)
				out = append(out, concreteValueEntry{
					kind: "errors", value: "Err: " + msg,
				})
			} else if strings.Contains(trimmed, "panic!(") ||
				strings.Contains(trimmed, "bail!(") {
				msg := extractErrorMessage(trimmed)
				out = append(out, concreteValueEntry{
					kind: "errors", value: "panic: " + msg,
				})
			}

		case repotypes.LangCpp:
			// throw std::runtime_error("...")
			if strings.HasPrefix(trimmed, "throw ") {
				rest := strings.TrimPrefix(trimmed, "throw ")
				rest = strings.TrimRight(rest, ";")
				if len(rest) > 60 {
					rest = rest[:60]
				}
				out = append(out, concreteValueEntry{
					kind: "errors", value: "throws " + strings.TrimSpace(rest),
				})
			}
		}
	}
	return out
}

// extractErrorMessage pulls the first quoted string or the function
// call expression from an error-producing line for a compact
// human-readable value.
func extractErrorMessage(line string) string {
	// Try to find a quoted string.
	if qi := strings.Index(line, "\""); qi >= 0 {
		end := strings.Index(line[qi+1:], "\"")
		if end >= 0 && end <= 80 {
			return line[qi : qi+1+end+1]
		}
	}
	// Fall back to the function call.
	if pi := strings.Index(line, "("); pi > 0 {
		prefix := line[:pi]
		// Take only the last identifier.
		for i := len(prefix) - 1; i >= 0; i-- {
			if prefix[i] == ' ' || prefix[i] == '\t' || prefix[i] == '=' {
				prefix = prefix[i+1:]
				break
			}
		}
		if len(prefix) > 40 {
			prefix = prefix[:40]
		}
		return prefix + "(...)"
	}
	if len(line) > 60 {
		return line[:60]
	}
	return line
}
