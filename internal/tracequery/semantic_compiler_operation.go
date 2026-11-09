package tracequery

import "strings"

// traceSpanCompilerWorkClass separates the runtime/compiler domain from an
// operation explicitly named by the raw native marker. Namespaces, class
// names and signature arguments describe the environment, not the operation:
// e.g. a compiler can allocate memory, load a cache, or initialize itself.
// Unclassified markers remain ordinary business spans, with the same identity
// and interval. This runs on capture records, never on questions or answers.
func traceSpanCompilerWorkClass(name, lower string) string {
	if !strings.Contains(lower, "compil") && !strings.Contains(lower, "dex2oat") {
		return ""
	}
	head, _, _ := strings.Cut(strings.TrimSpace(name), "(")
	if len(head) >= 2 && strings.EqualFold(head[:2], "h:") {
		head = strings.TrimSpace(head[2:])
	}
	words := traceCompilerMarkerWords(head)
	operation := words
	if last := strings.LastIndex(head, "::"); last >= 0 {
		// Real ART label form: "JIT compiling <method>". Its initial words
		// are an action label even when the named method is itself qualified.
		// A callable such as "jit::CompileTask::Initialize" has no such label.
		label, _, _ := strings.Cut(head, "::")
		labelWords := traceCompilerMarkerWords(label)
		if len(labelWords) < 2 || labelWords[0] != "jit" || !traceCompilerActionWord(labelWords[1]) ||
			!strings.HasPrefix(strings.ToLower(label), "jit ") {
			operation = traceCompilerMarkerWords(head[last+2:])
			if len(operation) == 0 || !traceCompilerActionWord(operation[0]) {
				return ""
			}
		}
	}
	compiling, jit, runtime := false, false, false
	for _, word := range operation {
		compiling = compiling || traceCompilerActionWord(word)
	}
	if !compiling {
		return ""
	}
	for _, word := range words {
		switch word {
		case "jit", "jitcompile", "jitcompiling", "jitcompilation":
			jit = true
		case "ark", "arkts", "art", "dex", "dex2oat", "bytecode", "wasm", "js", "runtime", "runtimecompile", "runtimecompiling", "runtimecompilation":
			runtime = true
		}
	}
	if jit || strings.Contains(strings.ToLower(head), "just-in-time") {
		return "jit_compile"
	}
	if runtime {
		return "runtime_compile"
	}
	return ""
}

func traceCompilerActionWord(word string) bool {
	switch word {
	case "compile", "compiling", "compilation", "recompile", "recompiling", "recompilation", "dex2oat",
		"jitcompile", "jitcompiling", "jitcompilation", "runtimecompile", "runtimecompiling", "runtimecompilation":
		return true
	}
	return false
}

// Preserve word boundaries for both spaced instrumentation and camel-case
// symbols (JITCompileTask -> jit/compile/task; JitCompiler -> jit/compiler).
func traceCompilerMarkerWords(marker string) []string {
	upper := func(c byte) bool { return c >= 'A' && c <= 'Z' }
	lower := func(c byte) bool { return c >= 'a' && c <= 'z' }
	var normalized strings.Builder
	normalized.Grow(len(marker))
	for i := 0; i < len(marker); i++ {
		c := marker[i]
		if upper(c) {
			if i > 0 && (lower(marker[i-1]) || (upper(marker[i-1]) && i+1 < len(marker) && lower(marker[i+1]))) {
				normalized.WriteByte(' ')
			}
			c += 'a' - 'A'
		}
		normalized.WriteByte(c)
	}
	return strings.FieldsFunc(normalized.String(), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
}
