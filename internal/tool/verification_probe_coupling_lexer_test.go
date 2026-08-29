package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestVerificationProbeCouplingExtractorsIgnoreLiteralAndCommentLookalikes(t *testing.T) {
	t.Run("python multiline docstring", func(t *testing.T) {
		got := pythonImportDeclarations("\"\"\"\nimport changed\n\"\"\"\nimport safe\n")
		assertProbeModuleRefs(t, got, []string{"safe"}, []string{"changed"})
	})

	t.Run("javascript literals and comments", func(t *testing.T) {
		got := javascriptImportDeclarations(types.VerificationProbe{Code: "const note = \"require('./changed')\";\n/* import fake from './changed'; */\nconst safe = require('./safe');\n"})
		assertProbeModuleRefs(t, got, []string{"safe"}, []string{"changed"})
	})

	t.Run("ruby literals and comments", func(t *testing.T) {
		got := rubyRequireDeclarations(types.VerificationProbe{Code: "message = \"require 'changed'\"\n# require 'changed'\nrequire 'safe'\n"})
		assertProbeModuleRefs(t, got, []string{"safe"}, []string{"changed"})
	})

	t.Run("java literals and block comments", func(t *testing.T) {
		got := javaImportDeclarations(types.VerificationProbe{Code: "String command = \"javac Main.java\";\n/* Main.value(); */\nSafe.value();\n"})
		assertProbeModuleRefs(t, got, []string{"Safe"}, []string{"Main"})
	})

	t.Run("go raw string", func(t *testing.T) {
		got := goImportDeclarations(types.VerificationProbe{Code: "package main\n\nimport \"fmt\"\n\nvar note = `import \\\"example.com/changed\\\"`\nvar _ = fmt.Sprint\n"})
		assertProbeModuleRefs(t, got, []string{"fmt"}, []string{"example.com/changed"})
	})
}

func TestEmitChangePlanRejectsJavaCommandWrapperWhoseOnlyTargetNameIsInLiteral(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "fix Main.java compile typo",
		"summary": "Modify Main.java and attach a wrapper that only launches the compiler.",
		"changes": [
			{"path": "Main.java", "kind": "modify", "new_content": "public class Main { static String greet() { return \"ok\"; } }\n", "rationale": "repair syntax"}
		],
		"verification_probes": [
			{"id": "compile_wrapper", "language": "java", "code": "public class CompileCheck { public static void main(String[] args) throws Exception { Process p = Runtime.getRuntime().exec(\"javac Main.java\"); if (p.waitFor() != 0) System.exit(1); } }", "changed_symbol_refs": ["path:Main.java"]}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatalf("compiler wrapper must not acquire changed-class coupling from a string literal: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "changed Java production module") {
		t.Fatalf("rejection must identify the missing typed Java coupling edge: %s", res.Summary)
	}
}

func assertProbeModuleRefs(t *testing.T, got map[string]struct{}, want, forbidden []string) {
	t.Helper()
	for _, ref := range want {
		if _, ok := got[ref]; !ok {
			t.Fatalf("module ref %q missing: %+v", ref, got)
		}
	}
	for _, ref := range forbidden {
		if _, ok := got[ref]; ok {
			t.Fatalf("literal/comment text minted module ref %q: %+v", ref, got)
		}
	}
}
