package index

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestJavaCallsCarryUniqueDeclaredReceiverTypes(t *testing.T) {
	src := []byte(`package com.clinic;
class Worker {
  void fieldCall() {}
  void paramCall() {}
  void localCall() {}
}
class Caller {
  private final Worker field = new Worker();
  void run(Worker param) {
    field.fieldCall();
    param.paramCall();
    Worker local = param;
    local.localCall();
  }
}`)
	root := parseJava(t, src)
	pkg, syms, _, rels := extractJava(root, src, "Caller.java")
	fi := &types.FileInfo{RelPath: "Caller.java", Language: types.LangJava, Package: pkg, Symbols: syms, Relations: rels}
	graph := BuildGraph(t.TempDir(), []*types.FileInfo{fi})
	want := map[string]string{"fieldCall": "Worker", "paramCall": "Worker", "localCall": "Worker"}
	for _, rel := range fi.Relations {
		receiver, ok := want[rel.ToEP.Name]
		if !ok {
			continue
		}
		if rel.ToEP.Receiver != receiver {
			t.Fatalf("%s receiver=%q, want %q: %+v", rel.ToEP.Name, rel.ToEP.Receiver, receiver, rel)
		}
		target := graph.ResolveCallTarget(fi, rel)
		if target == nil || target.Parent != "Worker" || target.Name != rel.ToEP.Name {
			t.Fatalf("%s did not resolve to Worker method: %+v", rel.ToEP.Name, target)
		}
		delete(want, rel.ToEP.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing receiver-aware Java call relations: %+v", want)
	}
}

func TestJavaCallsKeepSourceReceiverWhenFileWideTypeBindingConflicts(t *testing.T) {
	src := []byte(`class Alpha { void run() {} }
class Beta { void run() {} }
class One {
  Alpha worker;
  void invoke() { worker.run(); }
}
class Two {
  Beta worker;
  void invoke() { worker.run(); }
}`)
	root := parseJava(t, src)
	_, _, _, rels := extractJava(root, src, "Ambiguous.java")
	seen := 0
	for _, rel := range rels {
		if rel.ToEP.Name != "run" {
			continue
		}
		seen++
		if rel.ToEP.Receiver != "worker" {
			t.Fatalf("file-wide conflicting type must preserve source receiver worker, got receiver=%q: %+v", rel.ToEP.Receiver, rel)
		}
	}
	if seen != 2 {
		t.Fatalf("run call relation count=%d, want 2", seen)
	}
}

func TestJavaDeclaredTypeNameUsesGenericContainerNotTypeArgument(t *testing.T) {
	src := []byte(`import java.util.List;
class Holder {
  List<String> values;
  void run() { values.size(); }
}`)
	root := parseJava(t, src)
	bindings := javaUniqueReceiverTypeBindings(root, src)
	if got := bindings["values"]; got != "List" {
		t.Fatalf("generic binding type=%q, want container List", got)
	}
}
