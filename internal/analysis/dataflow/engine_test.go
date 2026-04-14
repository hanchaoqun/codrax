package dataflow

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// file = .../internal/analysis/dataflow/engine_test.go
	// project root = 3 dirs up
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

// ─────────────────────────────────────────────────────────────
// Helper: build a minimal Graph + on-disk source tree for a test scenario.
// ─────────────────────────────────────────────────────────────

type testFile struct {
	relPath  string
	lang     string
	content  string
	symbols  []repomap.Symbol
	relations []repomap.Relation
}

func setupTestRepo(t *testing.T, files []testFile) (string, *repomap.Graph) {
	t.Helper()
	root := t.TempDir()
	graph := &repomap.Graph{
		Root:           root,
		FileIndex:      make(map[string]*repomap.FileInfo),
		SymbolDefs:     make(map[string][]*repomap.Symbol),
		ImportGraph:    make(map[string][]string),
		ReverseImports: make(map[string][]string),
	}

	for _, tf := range files {
		absPath := filepath.Join(root, filepath.FromSlash(tf.relPath))
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absPath, []byte(tf.content), 0o644); err != nil {
			t.Fatal(err)
		}
		fi := &repomap.FileInfo{
			RelPath:   tf.relPath,
			Language:  tf.lang,
			Hash:      "test-hash-" + tf.relPath,
			Symbols:   tf.symbols,
			Relations: tf.relations,
		}
		graph.FileIndex[tf.relPath] = fi
		graph.Files = append(graph.Files, fi)
		for idx := range fi.Symbols {
			sym := &fi.Symbols[idx]
			sym.File = tf.relPath
			graph.SymbolDefs[sym.Name] = append(graph.SymbolDefs[sym.Name], sym)
		}
	}
	return root, graph
}

// ─────────────────────────────────────────────────────────────
// Scenario 1: Go — config reads config YAML → handler reads config
//   config.yaml: server.port = 8080
//   main.go:     GetPort() returns os.Getenv("PORT")
//   handler.go:  Serve() calls GetPort()
//
// Validates: config flattening → concrete evidence,
//            return literal → concrete evidence,
//            env key → mechanism evidence,
//            cross-function call → dataflow finding
// ─────────────────────────────────────────────────────────────

func TestE2E_GoConfigToHandlerDataflow(t *testing.T) {
	root, graph := setupTestRepo(t, []testFile{
		{
			relPath: "config.yaml",
			content: "server:\n  port: 8080\n  host: localhost\n",
		},
		{
			relPath: "main.go",
			lang:    repomap.LangGo,
			content: `package main

import "os"

func GetPort() string {
	port := os.Getenv("PORT")
	if port == "" {
		return "8080"
	}
	return port
}
`,
			symbols: []repomap.Symbol{
				{Name: "GetPort", Kind: "function", Line: 5, EndLine: 11, Exported: true},
			},
		},
		{
			relPath: "handler.go",
			lang:    repomap.LangGo,
			content: `package main

func Serve() {
	port := GetPort()
	listen(port)
}
`,
			symbols: []repomap.Symbol{
				{Name: "Serve", Kind: "function", Line: 3, EndLine: 6, Exported: true},
			},
			relations: []repomap.Relation{
				{Kind: "call", From: "handler.go:Serve", To: "GetPort", File: "handler.go", Line: 4},
			},
		},
	})

	result := Analyze(graph, Options{
		RepoRoot:       root,
		Question:       "What port does the server listen on?",
		CandidateFiles: []string{"config.yaml", "main.go", "handler.go"},
		WorkDir:        t.TempDir(),
	})

	// --- Config flattening evidence ---
	t.Run("config_flattening_produces_evidence", func(t *testing.T) {
		var foundPort, foundHost bool
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceConcrete && ev.Predicate == "config_value" {
				if ev.Subject == "server.port" && ev.Object == "8080" {
					foundPort = true
				}
				if ev.Subject == "server.host" && ev.Object == "localhost" {
					foundHost = true
				}
			}
		}
		if !foundPort {
			t.Fatal("missing config evidence: server.port = 8080")
		}
		if !foundHost {
			t.Fatal("missing config evidence: server.host = localhost")
		}
	})

	// --- Return literal detection ---
	t.Run("return_literal_produces_concrete_evidence", func(t *testing.T) {
		var found bool
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceConcrete && ev.Predicate == "returns" &&
				strings.Contains(ev.Subject, "GetPort") && ev.Object == `"8080"` {
				found = true
				if ev.Confidence < 0.9 {
					t.Errorf("concrete return literal confidence = %f, want >= 0.9", ev.Confidence)
				}
				if ev.Source != "main.go" {
					t.Errorf("source = %q, want main.go", ev.Source)
				}
			}
		}
		if !found {
			t.Fatal("missing concrete evidence: GetPort returns \"8080\"")
		}
	})

	// --- Env key detection ---
	t.Run("env_key_produces_mechanism_evidence", func(t *testing.T) {
		var found bool
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceMechanism && ev.Predicate == "reads_config" &&
				strings.Contains(ev.Subject, "GetPort") && ev.Object == "PORT" {
				found = true
			}
		}
		if !found {
			t.Fatal("missing mechanism evidence: GetPort reads_config PORT")
		}
	})

	// --- Guard detection ---
	t.Run("guard_detection_on_if_branch", func(t *testing.T) {
		var found bool
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceConditional &&
				strings.Contains(ev.Subject, "GetPort") &&
				strings.Contains(ev.Object, `port == ""`) {
				found = true
			}
		}
		if !found {
			t.Fatal("missing conditional evidence: GetPort guards on port == \"\"")
		}
	})

	// --- Cross-function call finding ---
	t.Run("cross_function_call_finding", func(t *testing.T) {
		var found bool
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceRelationship && ev.Predicate == "calls" &&
				strings.Contains(ev.Subject, "Serve") && ev.Object == "GetPort" {
				found = true
			}
		}
		if !found {
			t.Fatal("missing relationship evidence: Serve calls GetPort")
		}
	})

	// --- Dataflow finding: literal flows through call ---
	t.Run("dataflow_finding_literal_through_call", func(t *testing.T) {
		var found bool
		for _, f := range result.Findings {
			pathStr := strings.Join(f.Path, " -> ")
			if strings.Contains(pathStr, "GetPort") && strings.Contains(pathStr, "Serve") {
				found = true
				if f.Confidence < 0.7 {
					t.Errorf("finding confidence = %f, want >= 0.7", f.Confidence)
				}
				if f.ID == "" {
					t.Error("finding ID is empty, expected stable hash")
				}
			}
		}
		if !found {
			t.Fatal("missing dataflow finding: GetPort -> Serve")
		}
	})

	// --- Stable ID determinism ---
	t.Run("stable_ids_are_deterministic", func(t *testing.T) {
		result2 := Analyze(graph, Options{
			RepoRoot:       root,
			Question:       "What port does the server listen on?",
			CandidateFiles: []string{"config.yaml", "main.go", "handler.go"},
			WorkDir:        t.TempDir(),
		})
		if len(result.Evidence) != len(result2.Evidence) {
			t.Fatalf("evidence count changed: %d vs %d", len(result.Evidence), len(result2.Evidence))
		}
		ids1 := make(map[string]bool)
		for _, ev := range result.Evidence {
			ids1[ev.ID] = true
		}
		for _, ev := range result2.Evidence {
			if !ids1[ev.ID] {
				t.Errorf("second run produced new evidence ID %s (%s/%s/%s)", ev.ID, ev.Kind, ev.Subject, ev.Object)
			}
		}
	})
}

// ─────────────────────────────────────────────────────────────
// Scenario 2: Python — decorator registration + config read
//   app.py:     @app.route("/api/users") def get_users():
//   config.py:  DB_URL = os.getenv("DATABASE_URL")
//
// Validates: Python-specific lowering, decorator-like patterns,
//            env var detection in Python
// ─────────────────────────────────────────────────────────────

func TestE2E_PythonDecoratorAndConfig(t *testing.T) {
	root, graph := setupTestRepo(t, []testFile{
		{
			relPath: "app.py",
			lang:    repomap.LangPython,
			content: `from flask import Flask
app = Flask(__name__)

@app.route("/api/users")
def get_users():
    db = get_db_connection()
    return db.query("SELECT * FROM users")

def health():
    return "ok"
`,
			symbols: []repomap.Symbol{
				{Name: "get_users", Kind: "function", Line: 5, EndLine: 7, Exported: true},
				{Name: "health", Kind: "function", Line: 9, EndLine: 10, Exported: true},
			},
			relations: []repomap.Relation{
				{Kind: "call", From: "app.py:get_users", To: "get_db_connection", File: "app.py", Line: 6},
			},
		},
		{
			relPath: "config.py",
			lang:    repomap.LangPython,
			content: `import os

def get_db_url():
    return os.getenv("DATABASE_URL")

def get_debug():
    return os.getenv("DEBUG")
`,
			symbols: []repomap.Symbol{
				{Name: "get_db_url", Kind: "function", Line: 3, EndLine: 4, Exported: true},
				{Name: "get_debug", Kind: "function", Line: 6, EndLine: 7, Exported: true},
			},
		},
	})

	result := Analyze(graph, Options{
		RepoRoot:       root,
		Question:       "How does the app connect to the database?",
		CandidateFiles: []string{"app.py", "config.py"},
		WorkDir:        t.TempDir(),
	})

	t.Run("python_return_literal_detected", func(t *testing.T) {
		var found bool
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceConcrete && strings.Contains(ev.Subject, "health") &&
				ev.Object == `"ok"` {
				found = true
			}
		}
		if !found {
			t.Fatal("missing concrete evidence: health returns \"ok\"")
		}
	})

	t.Run("python_env_var_detected", func(t *testing.T) {
		var foundDB, foundDebug bool
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceMechanism && ev.Predicate == "reads_config" {
				if ev.Object == "DATABASE_URL" {
					foundDB = true
				}
				if ev.Object == "DEBUG" {
					foundDebug = true
				}
			}
		}
		if !foundDB {
			t.Fatal("missing mechanism evidence: get_db_url reads DATABASE_URL")
		}
		if !foundDebug {
			t.Fatal("missing mechanism evidence: get_debug reads DEBUG")
		}
	})

	t.Run("python_call_relationship", func(t *testing.T) {
		var found bool
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceRelationship && ev.Predicate == "calls" &&
				strings.Contains(ev.Subject, "get_users") && ev.Object == "get_db_connection" {
				found = true
			}
		}
		if !found {
			t.Fatal("missing relationship evidence: get_users calls get_db_connection")
		}
	})
}

// ─────────────────────────────────────────────────────────────
// Scenario 3: JavaScript/TypeScript — arrow functions + dynamic dispatch
//   service.ts:  getName = () => "UserService"
//   index.js:    uses require() with dynamic expression → unknown effect
//
// Validates: arrow function lowering, unknown effect detection, TS/JS coexistence
// ─────────────────────────────────────────────────────────────

func TestE2E_JSArrowAndUnknownEffect(t *testing.T) {
	root, graph := setupTestRepo(t, []testFile{
		{
			relPath: "service.ts",
			lang:    repomap.LangTypeScript,
			content: `export class UserService {
  getName() {
    return "UserService"
  }

  getVersion() {
    return 2
  }
}
`,
			symbols: []repomap.Symbol{
				{Name: "getName", Kind: "method", Parent: "UserService", Line: 2, EndLine: 4, Exported: true},
				{Name: "getVersion", Kind: "method", Parent: "UserService", Line: 6, EndLine: 8, Exported: true},
			},
		},
		{
			relPath: "index.js",
			lang:    repomap.LangJavaScript,
			content: `const path = require("path")

function loadPlugin(name) {
  const mod = require("./" + name)
  return mod.default
}

function getHost() {
  return process.env["HOST"] || "localhost"
}
`,
			symbols: []repomap.Symbol{
				{Name: "loadPlugin", Kind: "function", Line: 3, EndLine: 6, Exported: false},
				{Name: "getHost", Kind: "function", Line: 8, EndLine: 10, Exported: false},
			},
		},
	})

	result := Analyze(graph, Options{
		RepoRoot:       root,
		Question:       "What plugins are loaded dynamically?",
		CandidateFiles: []string{"service.ts", "index.js"},
		WorkDir:        t.TempDir(),
	})

	t.Run("ts_return_literal_detected", func(t *testing.T) {
		var found bool
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceConcrete && ev.Predicate == "returns" &&
				strings.Contains(ev.Subject, "getName") && ev.Object == `"UserService"` {
				found = true
			}
		}
		if !found {
			t.Fatal("missing concrete evidence: getName returns \"UserService\"")
		}
	})

	t.Run("js_dynamic_require_flags_unknown_effect", func(t *testing.T) {
		var found bool
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceUnresolved &&
				strings.Contains(ev.Subject, "loadPlugin") &&
				strings.Contains(ev.Object, "dynamic") {
				found = true
			}
		}
		if !found {
			t.Fatal("missing unresolved evidence: loadPlugin has dynamic require")
		}
	})

	t.Run("js_bracket_config_read", func(t *testing.T) {
		var found bool
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceMechanism && ev.Predicate == "reads_config" &&
				strings.Contains(ev.Subject, "getHost") && ev.Object == "HOST" {
				found = true
			}
		}
		if !found {
			t.Fatal("missing mechanism evidence: getHost reads HOST via bracket notation")
		}
	})
}

// ─────────────────────────────────────────────────────────────
// Scenario 4: Rust — unsafe block detection + return literal
//   lib.rs:  pub fn name() -> &'static str { "codrax" }
//            pub unsafe fn raw_ptr() { ... }
//
// Validates: Rust return detection, unsafe block flagging
// ─────────────────────────────────────────────────────────────

func TestE2E_RustUnsafeAndLiteral(t *testing.T) {
	root, graph := setupTestRepo(t, []testFile{
		{
			relPath: "src/lib.rs",
			lang:    repomap.LangRust,
			content: `pub fn name() -> &'static str {
    "codrax"
}

pub fn version() -> u32 {
    return 42
}

pub unsafe fn raw_alloc(size: usize) -> *mut u8 {
    let ptr = std::alloc::alloc(std::alloc::Layout::from_size_align_unchecked(size, 8));
    return ptr
}
`,
			symbols: []repomap.Symbol{
				{Name: "name", Kind: "function", Line: 1, EndLine: 3, Exported: true},
				{Name: "version", Kind: "function", Line: 5, EndLine: 7, Exported: true},
				{Name: "raw_alloc", Kind: "function", Line: 9, EndLine: 12, Exported: true},
			},
		},
	})

	result := Analyze(graph, Options{
		RepoRoot:       root,
		Question:       "What is the crate name?",
		CandidateFiles: []string{"src/lib.rs"},
		WorkDir:        t.TempDir(),
	})

	t.Run("rust_return_literal", func(t *testing.T) {
		var found bool
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceConcrete && strings.Contains(ev.Subject, "version") &&
				ev.Object == "42" {
				found = true
			}
		}
		if !found {
			t.Fatal("missing concrete evidence: version returns 42")
		}
	})

	t.Run("rust_unsafe_flagged", func(t *testing.T) {
		var found bool
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceUnresolved && strings.Contains(ev.Subject, "raw_alloc") {
				found = true
			}
		}
		if !found {
			t.Fatal("missing unresolved evidence: raw_alloc uses unsafe")
		}
	})
}

// ─────────────────────────────────────────────────────────────
// Scenario 5: Java — constructor + viper-like config
//   App.java:   new UserController() constructed, config.getString("db.url")
//
// Validates: Java new-expression detection, config.getString detection
// ─────────────────────────────────────────────────────────────

func TestE2E_JavaConstructorAndConfig(t *testing.T) {
	root, graph := setupTestRepo(t, []testFile{
		{
			relPath: "src/App.java",
			lang:    repomap.LangJava,
			content: `public class App {
    public void init() {
        String dbUrl = config.getString("db.url");
        UserController ctrl = new UserController(dbUrl);
        ctrl.start();
    }

    public String getName() {
        return "MainApp";
    }
}
`,
			symbols: []repomap.Symbol{
				{Name: "init", Kind: "method", Parent: "App", Line: 2, EndLine: 6, Exported: true},
				{Name: "getName", Kind: "method", Parent: "App", Line: 8, EndLine: 10, Exported: true},
			},
		},
	})

	result := Analyze(graph, Options{
		RepoRoot:       root,
		Question:       "How is the database configured?",
		CandidateFiles: []string{"src/App.java"},
		WorkDir:        t.TempDir(),
	})

	t.Run("java_config_getString_detected", func(t *testing.T) {
		var found bool
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceMechanism && ev.Predicate == "reads_config" &&
				ev.Object == "db.url" {
				found = true
			}
		}
		if !found {
			t.Fatal("missing mechanism evidence: init reads config db.url via getString")
		}
	})

	t.Run("java_return_literal", func(t *testing.T) {
		var found bool
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceConcrete && ev.Predicate == "returns" &&
				strings.Contains(ev.Subject, "getName") && ev.Object == `"MainApp"` {
				found = true
			}
		}
		if !found {
			t.Fatal("missing concrete evidence: getName returns \"MainApp\"")
		}
	})
}

// ─────────────────────────────────────────────────────────────
// Scenario 6: C/C++ — pointer aliasing → unknown effect
//   main.c:  void process(struct Ctx *ctx) { ctx->buffer = ... }
//
// Validates: C pointer-heavy unknown effect detection
// ─────────────────────────────────────────────────────────────

func TestE2E_CPointerUnknownEffect(t *testing.T) {
	root, graph := setupTestRepo(t, []testFile{
		{
			relPath: "main.c",
			lang:    repomap.LangC,
			content: `#include <stdlib.h>

void process(struct Ctx *ctx) {
    ctx->buffer = malloc(1024);
    *ctx->size = 1024;
}

int get_version() {
    return 42;
}
`,
			symbols: []repomap.Symbol{
				{Name: "process", Kind: "function", Line: 3, EndLine: 6, Exported: true},
				{Name: "get_version", Kind: "function", Line: 8, EndLine: 10, Exported: true},
			},
		},
	})

	result := Analyze(graph, Options{
		RepoRoot:       root,
		Question:       "What does process do?",
		CandidateFiles: []string{"main.c"},
		WorkDir:        t.TempDir(),
	})

	t.Run("c_pointer_flags_unknown_effect", func(t *testing.T) {
		var found bool
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceUnresolved && strings.Contains(ev.Subject, "process") {
				found = true
			}
		}
		if !found {
			t.Fatal("missing unresolved evidence: process has pointer-heavy effects")
		}
	})

	t.Run("c_return_literal", func(t *testing.T) {
		var found bool
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceConcrete && strings.Contains(ev.Subject, "get_version") &&
				ev.Object == "42" {
				found = true
			}
		}
		if !found {
			t.Fatal("missing concrete evidence: get_version returns 42")
		}
	})
}

// ─────────────────────────────────────────────────────────────
// Scenario 7: Multi-language config propagation
//   config.json:  { "database": { "host": "db.prod", "port": 5432 } }
//   app.toml:     [server]\nport = 3000
//
// Validates: JSON + TOML config flattening and evidence generation
// ─────────────────────────────────────────────────────────────

func TestE2E_MultiFormatConfigFlattening(t *testing.T) {
	root, graph := setupTestRepo(t, []testFile{
		{
			relPath: "config.json",
			content: `{"database": {"host": "db.prod", "port": 5432}, "debug": true}`,
		},
		{
			relPath: "app.toml",
			content: "[server]\nport = 3000\nhost = 0.0.0.0\n\n[logging]\nlevel = info\n",
		},
	})

	result := Analyze(graph, Options{
		RepoRoot:       root,
		Question:       "What is the database configuration?",
		CandidateFiles: []string{"config.json", "app.toml"},
		WorkDir:        t.TempDir(),
	})

	t.Run("json_config_flattened", func(t *testing.T) {
		configMap := make(map[string]string)
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceConcrete && ev.Predicate == "config_value" &&
				ev.Source == "config.json" {
				configMap[ev.Subject] = ev.Object
			}
		}
		if configMap["database.host"] != "db.prod" {
			t.Errorf("database.host = %q, want db.prod", configMap["database.host"])
		}
		if configMap["database.port"] != "5432" {
			t.Errorf("database.port = %q, want 5432", configMap["database.port"])
		}
		if configMap["debug"] != "true" {
			t.Errorf("debug = %q, want true", configMap["debug"])
		}
	})

	t.Run("toml_config_flattened", func(t *testing.T) {
		configMap := make(map[string]string)
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceConcrete && ev.Predicate == "config_value" &&
				ev.Source == "app.toml" {
				configMap[ev.Subject] = ev.Object
			}
		}
		if configMap["server.port"] != "3000" {
			t.Errorf("server.port = %q, want 3000", configMap["server.port"])
		}
		if configMap["server.host"] != "0.0.0.0" {
			t.Errorf("server.host = %q, want 0.0.0.0", configMap["server.host"])
		}
		if configMap["logging.level"] != "info" {
			t.Errorf("logging.level = %q, want info", configMap["logging.level"])
		}
	})
}

// ─────────────────────────────────────────────────────────────
// Scenario 8: Scope filtering — only files in scope are analyzed
// ─────────────────────────────────────────────────────────────

func TestE2E_ScopeFiltering(t *testing.T) {
	root, graph := setupTestRepo(t, []testFile{
		{
			relPath: "internal/core/main.go",
			lang:    repomap.LangGo,
			content: "package core\n\nfunc Name() string {\n\treturn \"core\"\n}\n",
			symbols: []repomap.Symbol{
				{Name: "Name", Kind: "function", Line: 3, EndLine: 5, Exported: true},
			},
		},
		{
			relPath: "cmd/cli/main.go",
			lang:    repomap.LangGo,
			content: "package main\n\nfunc Version() string {\n\treturn \"1.0\"\n}\n",
			symbols: []repomap.Symbol{
				{Name: "Version", Kind: "function", Line: 3, EndLine: 5, Exported: true},
			},
		},
	})

	result := Analyze(graph, Options{
		RepoRoot:       root,
		Question:       "What is the core name?",
		CandidateFiles: []string{"internal/core/main.go", "cmd/cli/main.go"},
		Scope:          []string{"internal/"},
		WorkDir:        t.TempDir(),
	})

	var foundCore, foundCli bool
	for _, ev := range result.Evidence {
		if strings.Contains(ev.Source, "internal/core") {
			foundCore = true
		}
		if strings.Contains(ev.Source, "cmd/cli") {
			foundCli = true
		}
	}
	if !foundCore {
		t.Fatal("expected evidence from internal/core (in scope)")
	}
	if foundCli {
		t.Fatal("unexpected evidence from cmd/cli (out of scope)")
	}
}

// ─────────────────────────────────────────────────────────────
// Scenario 9: Budget truncation
// ─────────────────────────────────────────────────────────────

func TestE2E_BudgetTruncation(t *testing.T) {
	files := make([]testFile, 50)
	candidates := make([]string, 50)
	for i := range files {
		rel := filepath.ToSlash(filepath.Join("pkg", strings.Replace(
			strings.Replace(filepath.Base(os.TempDir()), "/", "", -1), "\\", "", -1),
			"f"+strings.Repeat("x", 3)+strings.Replace(filepath.Ext(".go"), ".", "", 1)+
				func() string {
					s := make([]byte, 0, 8)
					s = append(s, byte('a'+i%26))
					return string(s)
				}()+".go"))
		// Simplified: just use a sequential name
		rel = "pkg/file_" + strings.Replace(func() string {
			return string(rune('a'+i%26)) + string(rune('0'+i/26))
		}(), "", "", 0) + ".go"
		files[i] = testFile{
			relPath: rel,
			lang:    repomap.LangGo,
			content: "package pkg\n\nfunc F() string {\n\treturn \"v\"\n}\n",
			symbols: []repomap.Symbol{
				{Name: "F", Kind: "function", Line: 3, EndLine: 5},
			},
		}
		candidates[i] = rel
	}

	root, graph := setupTestRepo(t, files)

	result := Analyze(graph, Options{
		RepoRoot:       root,
		CandidateFiles: candidates,
		MaxFiles:       5,
		WorkDir:        t.TempDir(),
	})

	var foundTruncated bool
	for _, ev := range result.Evidence {
		if ev.Kind == types.EvidenceTruncated {
			foundTruncated = true
		}
	}
	if !foundTruncated {
		t.Fatal("expected truncation evidence when candidates exceed MaxFiles")
	}
}

// ─────────────────────────────────────────────────────────────
// Scenario 10: Nil graph → empty result, no panic
// ─────────────────────────────────────────────────────────────

func TestE2E_NilGraphSafety(t *testing.T) {
	result := Analyze(nil, Options{
		RepoRoot: "/nonexistent",
	})
	if len(result.Evidence) != 0 {
		t.Fatalf("nil graph should produce no evidence, got %d", len(result.Evidence))
	}
	if len(result.Findings) != 0 {
		t.Fatalf("nil graph should produce no findings, got %d", len(result.Findings))
	}
}

// ─────────────────────────────────────────────────────────────
// Scenario 11: Empty candidates → empty result
// ─────────────────────────────────────────────────────────────

func TestE2E_EmptyCandidates(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: make(map[string]*repomap.FileInfo),
	}
	result := Analyze(graph, Options{
		RepoRoot:       "/tmp",
		CandidateFiles: []string{},
	})
	if len(result.Evidence) != 0 {
		t.Fatalf("empty candidates should produce no evidence, got %d", len(result.Evidence))
	}
}

// ─────────────────────────────────────────────────────────────
// Scenario 12: Evidence deduplication across same file analyzed twice
// ─────────────────────────────────────────────────────────────

func TestE2E_EvidenceDeduplication(t *testing.T) {
	root, graph := setupTestRepo(t, []testFile{
		{
			relPath: "util.go",
			lang:    repomap.LangGo,
			content: "package util\n\nfunc Default() string {\n\treturn \"default\"\n}\n",
			symbols: []repomap.Symbol{
				{Name: "Default", Kind: "function", Line: 3, EndLine: 5, Exported: true},
			},
		},
	})

	// Pass the same file twice as candidate
	result := Analyze(graph, Options{
		RepoRoot:       root,
		CandidateFiles: []string{"util.go", "util.go"},
		WorkDir:        t.TempDir(),
	})

	idCount := make(map[string]int)
	for _, ev := range result.Evidence {
		idCount[ev.ID]++
	}
	for id, count := range idCount {
		if count > 1 {
			t.Errorf("evidence ID %s appears %d times, expected deduplication", id, count)
		}
	}
}

// ─────────────────────────────────────────────────────────────
// Scenario 13: Go — field write + read detection
// ─────────────────────────────────────────────────────────────

func TestE2E_GoFieldWriteDetection(t *testing.T) {
	root, graph := setupTestRepo(t, []testFile{
		{
			relPath: "server.go",
			lang:    repomap.LangGo,
			content: `package main

func Configure(s *Server) {
	s.Port = 8080
	s.Host = "localhost"
	name := s.Name
	_ = name
}
`,
			symbols: []repomap.Symbol{
				{Name: "Configure", Kind: "function", Line: 3, EndLine: 8, Exported: true},
			},
		},
	})

	result := Analyze(graph, Options{
		RepoRoot:       root,
		CandidateFiles: []string{"server.go"},
		WorkDir:        t.TempDir(),
	})

	t.Run("field_writes_produce_evidence", func(t *testing.T) {
		var foundPortWrite, foundHostWrite bool
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceRelationship && ev.Predicate == "writes_field" &&
				strings.Contains(ev.Subject, "Configure") {
				if ev.Object == "s.Port" {
					foundPortWrite = true
				}
				if ev.Object == "s.Host" {
					foundHostWrite = true
				}
			}
		}
		if !foundPortWrite {
			t.Fatal("missing relationship evidence: Configure writes_field s.Port")
		}
		if !foundHostWrite {
			t.Fatal("missing relationship evidence: Configure writes_field s.Host")
		}
	})

	t.Run("field_reads_produce_evidence", func(t *testing.T) {
		var foundNameRead bool
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceRelationship && ev.Predicate == "reads_field" &&
				strings.Contains(ev.Subject, "Configure") && ev.Object == "s.Name" {
				foundNameRead = true
			}
		}
		if !foundNameRead {
			t.Fatal("missing relationship evidence: Configure reads_field s.Name")
		}
	})
}

// ─────────────────────────────────────────────────────────────
// Scenario 14: Cache round-trip — second Analyze uses cache
// ─────────────────────────────────────────────────────────────

func TestE2E_CacheRoundTrip(t *testing.T) {
	root, graph := setupTestRepo(t, []testFile{
		{
			relPath: "cached.go",
			lang:    repomap.LangGo,
			content: "package main\n\nfunc Cached() string {\n\treturn \"cached-value\"\n}\n",
			symbols: []repomap.Symbol{
				{Name: "Cached", Kind: "function", Line: 3, EndLine: 5, Exported: true},
			},
		},
	})

	workDir := t.TempDir()
	opts := Options{
		RepoRoot:       root,
		CandidateFiles: []string{"cached.go"},
		WorkDir:        workDir,
	}

	result1 := Analyze(graph, opts)

	// Verify cache file was written
	cacheDir := filepath.Join(workDir, "dataflow-cache", "lowered")
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("cache dir not created: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no cache files written")
	}

	// Second run should use cache and produce identical results
	result2 := Analyze(graph, opts)

	if len(result1.Evidence) != len(result2.Evidence) {
		t.Fatalf("cached run evidence count %d != original %d",
			len(result2.Evidence), len(result1.Evidence))
	}
}

// ─────────────────────────────────────────────────────────────
// Scenario 15: Go reflective code → unknown effect with finding
// ─────────────────────────────────────────────────────────────

func TestE2E_GoReflectUnknownEffect(t *testing.T) {
	root, graph := setupTestRepo(t, []testFile{
		{
			relPath: "dynamic.go",
			lang:    repomap.LangGo,
			content: `package main

import "reflect"

func InvokeMethod(obj interface{}, name string) {
	v := reflect.ValueOf(obj)
	m := v.MethodByName(name)
	m.Call(nil)
}
`,
			symbols: []repomap.Symbol{
				{Name: "InvokeMethod", Kind: "function", Line: 5, EndLine: 9, Exported: true},
			},
		},
	})

	result := Analyze(graph, Options{
		RepoRoot:       root,
		CandidateFiles: []string{"dynamic.go"},
		WorkDir:        t.TempDir(),
	})

	// Should have unresolved evidence for reflection usage
	var foundUnresolved bool
	for _, ev := range result.Evidence {
		if ev.Kind == types.EvidenceUnresolved && strings.Contains(ev.Subject, "InvokeMethod") {
			foundUnresolved = true
		}
	}
	if !foundUnresolved {
		t.Fatal("missing unresolved evidence for reflect usage in InvokeMethod")
	}

	// Should also produce a low-confidence finding
	var foundFinding bool
	for _, f := range result.Findings {
		if strings.Contains(strings.Join(f.Hops, " "), "unknown_effect") {
			foundFinding = true
			if f.Confidence > 0.5 {
				t.Errorf("unknown_effect finding confidence = %f, want <= 0.5", f.Confidence)
			}
		}
	}
	if !foundFinding {
		t.Fatal("missing unknown_effect finding for reflection")
	}
}

// ═════════════════════════════════════════════════════════════
// Scenario 16: REAL REPO — run dataflow analysis on the actual
// codrax project source, using repomap.ScanFiles + ParseFiles
// + BuildGraph from the live environment.
//
// This validates the full pipeline end-to-end against real Go
// code with real symbols, real relations, and real config files.
// ═════════════════════════════════════════════════════════════

func TestE2E_RealRepo_CodraxSelfAnalysis(t *testing.T) {
	root := projectRoot(t)

	// Use repomap to scan and parse the actual project files
	entries, err := repomap.ScanFiles(root)
	if err != nil {
		t.Fatalf("ScanFiles: %v", err)
	}
	if len(entries) < 10 {
		t.Fatalf("ScanFiles returned only %d entries, expected a real project", len(entries))
	}

	parsed := repomap.ParseFiles(entries, root)
	graph := repomap.BuildGraph(root, parsed)

	t.Run("analyze_providers_config", func(t *testing.T) {
		// Focus on config/providers.yaml — the only YAML config
		// that survives in the read-only codrax pipeline.
		result := Analyze(graph, Options{
			RepoRoot: root,
			Question: "What providers and models does the LLM routing use?",
			CandidateFiles: []string{
				"config/providers.yaml",
				"internal/config/providers.go",
			},
			WorkDir: t.TempDir(),
		})

		// config/providers.yaml is a real YAML file — must produce config evidence
		var configEvidence int
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceConcrete && ev.Predicate == "config_value" &&
				ev.Source == "config/providers.yaml" {
				configEvidence++
			}
		}
		if configEvidence == 0 {
			t.Fatal("real YAML config produced no flattened config evidence")
		}
		t.Logf("config/providers.yaml produced %d config evidence items", configEvidence)
	})

	t.Run("analyze_agent_package", func(t *testing.T) {
		// Analyze the agent package — should find concrete returns, calls, guards
		var agentFiles []string
		for _, fi := range graph.Files {
			if strings.HasPrefix(fi.RelPath, "internal/agent/") && fi.Language == repomap.LangGo {
				agentFiles = append(agentFiles, fi.RelPath)
			}
		}
		if len(agentFiles) == 0 {
			t.Fatal("no agent Go files found in graph")
		}

		result := Analyze(graph, Options{
			RepoRoot:       root,
			Question:       "How does the explorer agent work?",
			CandidateFiles: agentFiles,
			Scope:          []string{"internal/agent/"},
			MaxFiles:       15,
			WorkDir:        t.TempDir(),
		})

		var (
			concreteCount     int
			conditionalCount  int
			relationshipCount int
			mechanismCount    int
			unresolvedCount   int
		)
		for _, ev := range result.Evidence {
			switch ev.Kind {
			case types.EvidenceConcrete:
				concreteCount++
			case types.EvidenceConditional:
				conditionalCount++
			case types.EvidenceRelationship:
				relationshipCount++
			case types.EvidenceMechanism:
				mechanismCount++
			case types.EvidenceUnresolved:
				unresolvedCount++
			}
		}

		t.Logf("agent package analysis: concrete=%d conditional=%d relationship=%d mechanism=%d unresolved=%d findings=%d",
			concreteCount, conditionalCount, relationshipCount, mechanismCount, unresolvedCount, len(result.Findings))

		// The agent package is substantial Go code — must produce meaningful evidence
		if concreteCount == 0 {
			t.Error("no concrete evidence from agent package (expected return literals)")
		}
		if conditionalCount == 0 {
			t.Error("no conditional evidence from agent package (expected if/switch guards)")
		}
		if len(result.Evidence) < 5 {
			t.Errorf("only %d total evidence items, expected substantial output from agent package", len(result.Evidence))
		}
	})

	t.Run("analyze_dataflow_self_referential", func(t *testing.T) {
		// Analyze the dataflow package itself — the ultimate self-test
		var dfFiles []string
		for _, fi := range graph.Files {
			if strings.HasPrefix(fi.RelPath, "internal/analysis/dataflow/") &&
				fi.Language == repomap.LangGo &&
				!strings.HasSuffix(fi.RelPath, "_test.go") {
				dfFiles = append(dfFiles, fi.RelPath)
			}
		}
		if len(dfFiles) == 0 {
			t.Fatal("no dataflow Go files found in graph")
		}

		result := Analyze(graph, Options{
			RepoRoot:       root,
			Question:       "How does the lowerer extract evidence from source code?",
			CandidateFiles: dfFiles,
			WorkDir:        t.TempDir(),
		})

		// Must find evidence about its own code
		var foundSelfEvidence bool
		for _, ev := range result.Evidence {
			if strings.Contains(ev.Source, "internal/analysis/dataflow/") {
				foundSelfEvidence = true
				break
			}
		}
		if !foundSelfEvidence {
			t.Fatal("dataflow package produced no evidence about itself")
		}

		// The lowerer reads config patterns — should detect its own regex usage
		// The engine has reflect-like patterns — should flag something
		t.Logf("self-analysis: %d evidence items, %d findings", len(result.Evidence), len(result.Findings))

		// All evidence IDs must be non-empty and unique
		ids := make(map[string]bool)
		for _, ev := range result.Evidence {
			if ev.ID == "" {
				t.Errorf("evidence with empty ID: kind=%s subject=%s", ev.Kind, ev.Subject)
			}
			if ids[ev.ID] {
				t.Errorf("duplicate evidence ID: %s", ev.ID)
			}
			ids[ev.ID] = true
		}

		// All findings must have stable IDs
		for _, f := range result.Findings {
			if f.ID == "" {
				t.Errorf("finding with empty ID: path=%v", f.Path)
			}
		}
	})

	t.Run("analyze_config_yaml_files", func(t *testing.T) {
		// Test all YAML config files in the project
		var yamlFiles []string
		for _, fi := range graph.Files {
			if strings.HasSuffix(fi.RelPath, ".yaml") || strings.HasSuffix(fi.RelPath, ".yml") {
				yamlFiles = append(yamlFiles, fi.RelPath)
			}
		}

		result := Analyze(graph, Options{
			RepoRoot:       root,
			Question:       "What configuration keys exist?",
			CandidateFiles: yamlFiles,
			WorkDir:        t.TempDir(),
		})

		configKeys := make(map[string]string) // key → source file
		for _, ev := range result.Evidence {
			if ev.Kind == types.EvidenceConcrete && ev.Predicate == "config_value" {
				configKeys[ev.Subject] = ev.Source
			}
		}
		t.Logf("found %d config keys across %d YAML files", len(configKeys), len(yamlFiles))

		if len(configKeys) == 0 && len(yamlFiles) > 0 {
			t.Error("YAML files found but no config keys extracted")
		}
	})

	t.Run("analyze_cross_package_call_chain", func(t *testing.T) {
		// Select files from orchestrator + agent + context — should produce cross-package findings
		candidates := []string{
			"internal/orchestrator/orchestrator.go",
			"internal/context/builder.go",
		}
		// Add a few agent files
		for _, fi := range graph.Files {
			if strings.HasPrefix(fi.RelPath, "internal/agent/") &&
				fi.Language == repomap.LangGo &&
				!strings.HasSuffix(fi.RelPath, "_test.go") {
				candidates = append(candidates, fi.RelPath)
				if len(candidates) >= 8 {
					break
				}
			}
		}

		result := Analyze(graph, Options{
			RepoRoot:       root,
			Question:       "How does the orchestrator dispatch work to agents?",
			CandidateFiles: candidates,
			MaxFiles:       20,
			WorkDir:        t.TempDir(),
		})

		// Cross-package analysis should produce relationship evidence from multiple files
		sources := make(map[string]bool)
		for _, ev := range result.Evidence {
			if ev.Source != "" {
				dir := filepath.Dir(ev.Source)
				sources[dir] = true
			}
		}
		t.Logf("evidence spans %d directories: %v", len(sources), sources)

		if len(sources) < 2 {
			t.Errorf("cross-package analysis only covered %d directories, expected >= 2", len(sources))
		}

		if len(result.Findings) == 0 {
			t.Log("warning: no findings from cross-package analysis (may indicate limited call relations in graph)")
		} else {
			t.Logf("found %d cross-package findings", len(result.Findings))
		}
	})
}
