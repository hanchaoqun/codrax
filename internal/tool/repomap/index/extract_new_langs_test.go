package index

import (
	"context"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// parseSourceFor is a tiny test helper: get the tree-sitter language
// for `lang`, parse `src`, return the root node. Skips the test when
// the grammar isn't wired (defensive — we want clean failures, not
// nil-deref panics).
func parseSourceFor(t *testing.T, lang, src string) *sitter.Node {
	t.Helper()
	g := types.GetSitterLanguage(lang)
	if g == nil {
		t.Fatalf("no tree-sitter grammar for %q", lang)
	}
	parser := sitter.NewParser()
	parser.SetLanguage(g)
	tree, err := parser.ParseCtx(context.Background(), nil, []byte(src))
	if err != nil {
		t.Fatalf("parse %s: %v", lang, err)
	}
	if tree == nil {
		t.Fatalf("nil tree for %s", lang)
	}
	return tree.RootNode()
}

func symbolNames(syms []types.Symbol) []string {
	out := make([]string, 0, len(syms))
	for _, s := range syms {
		out = append(out, s.Name)
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

// ── Ruby ──────────────────────────────────────────────────────────

func TestExtractRuby_ClassesAndMethods(t *testing.T) {
	src := `require "json"

class Greeter < Object
  def initialize(name)
    @name = name
  end

  def greet
    "Hello, #{@name}!"
  end
end

module Util
  def self.upper(s)
    s.upcase
  end
end

CONST_VAL = 42

def top_level_fn(x); x + 1; end
`
	root := parseSourceFor(t, "ruby", src)
	pkg, syms, imps, rels := extractRuby(root, []byte(src), "lib/greeter/greeter.rb")
	_ = pkg
	names := symbolNames(syms)
	for _, want := range []string{"Greeter", "Util", "initialize", "greet", "upper", "CONST_VAL", "top_level_fn"} {
		if !contains(names, want) {
			t.Errorf("missing symbol %q in %v", want, names)
		}
	}
	if len(imps) == 0 || imps[0].Path != "json" {
		t.Errorf("require \"json\" import missing; got %v", imps)
	}
	// Inheritance: Greeter < Object
	hasInherit := false
	for _, r := range rels {
		if r.Kind == "inheritance" && r.ToEP.Name == "Object" {
			hasInherit = true
		}
	}
	if !hasInherit {
		t.Errorf("Greeter < Object inheritance not captured; got %+v", rels)
	}
}

// ── Swift ─────────────────────────────────────────────────────────

func TestExtractSwift_ClassFuncProtocol(t *testing.T) {
	src := `import Foundation

protocol Greetable {
    func greet() -> String
}

class Greeter: NSObject, Greetable {
    private let name: String

    init(name: String) {
        self.name = name
    }

    func greet() -> String {
        return "Hello, \(name)!"
    }
}

func topLevel(x: Int) -> Int { return x + 1 }
`
	root := parseSourceFor(t, "swift", src)
	_, syms, imps, rels := extractSwift(root, []byte(src), "Sources/Greeter/Greeter.swift")
	names := symbolNames(syms)
	for _, want := range []string{"Greetable", "Greeter", "greet", "topLevel", "init"} {
		if !contains(names, want) {
			t.Errorf("missing symbol %q in %v", want, names)
		}
	}
	if len(imps) == 0 || imps[0].Path != "Foundation" {
		t.Errorf("Foundation import missing; got %v", imps)
	}
	hasInheritance := false
	for _, r := range rels {
		if r.Kind == "inheritance" && (r.ToEP.Name == "NSObject" || r.ToEP.Name == "Greetable") {
			hasInheritance = true
			break
		}
	}
	if !hasInheritance {
		t.Errorf("Greeter inheritance/conformance missing; got %+v", rels)
	}
}

func TestExtractSwift_StructExtensionActorKinds(t *testing.T) {
	src := `struct User: Codable {
    let name: String
    init(name: String) {
        self.name = name
    }
}

extension User: CustomStringConvertible {
    func render() -> String {
        return name
    }
}

actor Loader {
    func load() async throws -> Int {
        return 1
    }
}
`
	root := parseSourceFor(t, "swift", src)
	_, syms, _, rels := extractSwift(root, []byte(src), "Sources/App/Models.swift")

	hasStruct := false
	hasExtend := false
	hasCtor := false
	hasActor := false
	for _, s := range syms {
		switch {
		case s.Name == "User" && s.Kind == "struct":
			hasStruct = true
		case s.Name == "User" && s.Kind == "extend":
			hasExtend = true
		case s.Name == "init" && s.Kind == "ctor" && s.Parent == "User":
			hasCtor = true
		case s.Name == "Loader" && s.Kind == "actor":
			hasActor = true
		}
	}
	if !hasStruct || !hasExtend || !hasCtor || !hasActor {
		t.Fatalf("swift kinds missing: struct=%v extend=%v ctor=%v actor=%v syms=%+v", hasStruct, hasExtend, hasCtor, hasActor, syms)
	}
	hasConformance := false
	for _, r := range rels {
		if r.Kind == "inheritance" && r.ToEP.Name == "CustomStringConvertible" {
			hasConformance = true
			break
		}
	}
	if !hasConformance {
		t.Errorf("extension conformance missing; got %+v", rels)
	}
}

// ── Lua ───────────────────────────────────────────────────────────

func TestExtractLua_ModulePattern(t *testing.T) {
	src := `local M = {}

local function private_helper(x)
    return x * 2
end

function M.greet(name)
    return "Hello, " .. name
end

function M:method_with_self()
    return self
end

local socket = require "socket"

return M
`
	root := parseSourceFor(t, "lua", src)
	_, syms, imps, _ := extractLua(root, []byte(src), "lua/greeter/init.lua")
	names := symbolNames(syms)
	for _, want := range []string{"private_helper", "greet", "method_with_self"} {
		if !contains(names, want) {
			t.Errorf("missing symbol %q in %v", want, names)
		}
	}
	// `private_helper` must be Exported=false (local function).
	for _, s := range syms {
		if s.Name == "private_helper" && s.Exported {
			t.Errorf("local function should not be Exported; got %+v", s)
		}
	}
	if len(imps) == 0 {
		t.Errorf("require \"socket\" missing; got %v", imps)
	}
}

// ── Proto ─────────────────────────────────────────────────────────

func TestExtractProto_MessageServiceRpc(t *testing.T) {
	src := `syntax = "proto3";
package mypkg.v1;

import "google/protobuf/empty.proto";

message Greeting {
    string name = 1;
    int32 age = 2;
}

enum Status {
    UNKNOWN = 0;
    OK = 1;
}

service Greeter {
    rpc Hello(Greeting) returns (Greeting);
    rpc Goodbye(Greeting) returns (google.protobuf.Empty);
}
`
	root := parseSourceFor(t, "proto", src)
	pkg, syms, imps, _ := extractProto(root, []byte(src), "proto/mypkg/v1/greeter.proto")
	if pkg != "mypkg.v1" {
		t.Errorf("package = %q; want mypkg.v1", pkg)
	}
	names := symbolNames(syms)
	for _, want := range []string{"Greeting", "Status", "Greeter", "Hello", "Goodbye"} {
		if !contains(names, want) {
			t.Errorf("missing symbol %q in %v", want, names)
		}
	}
	if len(imps) == 0 || imps[0].Path != "google/protobuf/empty.proto" {
		t.Errorf("import missing; got %v", imps)
	}
	// Sanity: rpc methods carry Parent="Greeter".
	for _, s := range syms {
		if s.Kind == "rpc" && s.Parent != "Greeter" {
			t.Errorf("rpc %s parent = %q; want Greeter", s.Name, s.Parent)
		}
	}
}

// ── DetectLanguage edge: CUDA + Objective-C remap ─────────────────

func TestDetectLanguage_CudaAndObjC(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"src/kernel.cu", "cpp"},
		{"include/kernel.cuh", "cpp"},
		{"src/AppDelegate.m", "c"},
		{"src/AppDelegate.mm", "cpp"},
		{"foo.rb", "ruby"},
		{"foo.swift", "swift"},
		{"foo.lua", "lua"},
		{"foo.proto", "proto"},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			if got := types.DetectLanguage(c.path); got != c.want {
				t.Errorf("DetectLanguage(%q) = %q; want %q", c.path, got, c.want)
			}
		})
	}
}
