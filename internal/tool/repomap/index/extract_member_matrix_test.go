package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestExtractMemberCarrierMatrix_AllSupportedReadLanguages(t *testing.T) {
	cases := map[string]struct {
		relPath string
		source  string
		name    string
		kind    string
		parent  string
	}{
		types.LangGo: {
			relPath: "runtime.go",
			source: `package runtime
type Runtime struct {
	RetryBudget int
}
`,
			name: "RetryBudget", kind: "field", parent: "Runtime",
		},
		types.LangPython: {
			relPath: "runtime.py",
			source: `class Runtime:
    retry_budget = 3
`,
			name: "retry_budget", kind: "field", parent: "Runtime",
		},
		types.LangJavaScript: {
			relPath: "runtime.js",
			source: `class Runtime {
  retryBudget = 3
}
`,
			name: "retryBudget", kind: "field", parent: "Runtime",
		},
		types.LangTypeScript: {
			relPath: "runtime.ts",
			source: `interface Runtime {
  retryBudget: number
}
`,
			name: "retryBudget", kind: "field", parent: "Runtime",
		},
		types.LangJava: {
			relPath: "Runtime.java",
			source: `public class Runtime {
  public int retryBudget;
}
`,
			name: "retryBudget", kind: "field", parent: "Runtime",
		},
		types.LangKotlin: {
			relPath: "Runtime.kt",
			source: `class Runtime {
  val retryBudget: Int = 3
}
`,
			name: "retryBudget", kind: "field", parent: "Runtime",
		},
		types.LangRust: {
			relPath: "runtime.rs",
			source: `pub struct Runtime {
    pub retry_budget: usize,
}
`,
			name: "retry_budget", kind: "field", parent: "Runtime",
		},
		types.LangC: {
			relPath: "runtime.c",
			source: `struct Runtime {
  int retry_budget;
};
`,
			name: "retry_budget", kind: "field", parent: "Runtime",
		},
		types.LangCpp: {
			relPath: "runtime.hpp",
			source: `class Runtime {
public:
  int retryBudget;
};
`,
			name: "retryBudget", kind: "field", parent: "Runtime",
		},
		types.LangRuby: {
			relPath: "lib/runtime/runtime.rb",
			source: `class Runtime
  RETRY_BUDGET = 3
end
`,
			name: "RETRY_BUDGET", kind: "const", parent: "Runtime",
		},
		types.LangSwift: {
			relPath: "Sources/App/Runtime.swift",
			source: `struct Runtime {
  var retryBudget: Int
}
`,
			name: "retryBudget", kind: "field", parent: "Runtime",
		},
		types.LangLua: {
			relPath: "lua/runtime.lua",
			source: `Runtime = {}
function Runtime.retryBudget()
  return 3
end
`,
			name: "retryBudget", kind: "method", parent: "Runtime",
		},
		types.LangProto: {
			relPath: "runtime.proto",
			source: `syntax = "proto3";
message Runtime {
  int32 retry_budget = 1;
}
`,
			name: "retry_budget", kind: "field", parent: "Runtime",
		},
		types.LangArkTS: {
			relPath: "entry/src/main/ets/pages/Runtime.ets",
			source: `@Component
struct Runtime {
  @State retryBudget: number = 3
  build() {}
}
`,
			name: "retryBudget", kind: "state-field", parent: "Runtime",
		},
		types.LangCangjie: {
			relPath: "src/runtime.cj",
			source: `package demo
public class Runtime {
  public let retryBudget: Int64 = 3
}
`,
			name: "retryBudget", kind: "field", parent: "Runtime",
		},
	}
	for _, lang := range types.SupportedReadLanguages() {
		c, ok := cases[lang]
		if !ok {
			t.Fatalf("member carrier matrix missing supported language %q", lang)
		}
		t.Run(lang, func(t *testing.T) {
			dir := t.TempDir()
			abs := filepath.Join(dir, filepath.Base(c.relPath))
			if err := os.WriteFile(abs, []byte(c.source), 0o644); err != nil {
				t.Fatal(err)
			}
			fi := parseOneFile(FileEntry{
				RelPath:  c.relPath,
				AbsPath:  abs,
				Language: lang,
				Size:     int64(len(c.source)),
			})
			if fi == nil {
				t.Fatal("parseOneFile returned nil")
			}
			for _, sym := range fi.Symbols {
				if sym.Name == c.name && sym.Kind == c.kind && sym.Parent == c.parent {
					return
				}
			}
			t.Fatalf("missing %s member %s/%s parent=%s; symbols=%+v", lang, c.name, c.kind, c.parent, fi.Symbols)
		})
	}
}
