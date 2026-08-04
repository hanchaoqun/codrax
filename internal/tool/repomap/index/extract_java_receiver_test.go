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

func TestJavaSameNameFieldsResolveInsideTheirOwningClasses(t *testing.T) {
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
	want := map[int]string{5: "Alpha", 9: "Beta"}
	for _, rel := range rels {
		if rel.ToEP.Name != "run" {
			continue
		}
		if receiver, ok := want[rel.Line]; ok {
			if rel.ToEP.Receiver != receiver {
				t.Fatalf("line %d receiver=%q, want owning class type %q: %+v", rel.Line, rel.ToEP.Receiver, receiver, rel)
			}
			delete(want, rel.Line)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing class-scoped calls: %+v; relations=%+v", want, rels)
	}
}

func TestJavaDeclaredTypeNameUsesGenericContainerNotTypeArgument(t *testing.T) {
	src := []byte(`import java.util.List;
class Holder {
  List<String> values;
  void run() { values.size(); }
}`)
	root := parseJava(t, src)
	_, _, _, rels := extractJava(root, src, "Holder.java")
	for _, rel := range rels {
		if rel.ToEP.Name == "size" {
			if rel.ToEP.Receiver != "List" {
				t.Fatalf("generic binding receiver=%q, want container List: %+v", rel.ToEP.Receiver, rel)
			}
			return
		}
	}
	t.Fatalf("missing values.size call: %+v", rels)
}

func TestJavaVarDoesNotMintDeclaredReceiverType(t *testing.T) {
	src := []byte(`class Alpha { void run() {} }
class Beta { void run() {} }
class Caller {
  void invoke() {
    var alpha = new Alpha();
    var beta = new Beta();
    alpha.run();
    beta.run();
  }
}`)
	root := parseJava(t, src)
	_, _, _, rels := extractJava(root, src, "Caller.java")
	want := map[int]string{7: "alpha", 8: "beta"}
	for _, rel := range rels {
		if rel.ToEP.Name != "run" {
			continue
		}
		if got, ok := want[rel.Line]; ok {
			if rel.ToEP.Receiver != got {
				t.Fatalf("line %d receiver=%q, want source binding %q: %+v", rel.Line, rel.ToEP.Receiver, got, rel)
			}
			delete(want, rel.Line)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing var receiver calls: %+v; relations=%+v", want, rels)
	}
}

func TestJavaVarDeclarationShadowsFileWideExplicitTypeAuthority(t *testing.T) {
	src := []byte(`class Worker { void run() {} }
class Other { void run() {} }
class Caller {
  Worker repo;
  void explicit() { repo.run(); }
  void inferred() {
    var repo = new Other();
    repo.run();
  }
}`)
	root := parseJava(t, src)
	_, _, _, rels := extractJava(root, src, "Caller.java")
	want := map[int]string{5: "Worker", 8: "repo"}
	for _, rel := range rels {
		if rel.ToEP.Name != "run" {
			continue
		}
		if receiver, ok := want[rel.Line]; ok {
			if rel.ToEP.Receiver != receiver {
				t.Fatalf("line %d receiver=%q, want %q: %+v", rel.Line, rel.ToEP.Receiver, receiver, rel)
			}
			delete(want, rel.Line)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing Java scope calls: %+v; relations=%+v", want, rels)
	}
}
