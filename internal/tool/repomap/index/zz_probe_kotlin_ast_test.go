package index

// THROWAWAY probe — dumps the tree-sitter-kotlin AST for a Spring
// controller so the annotation reader is written against observed node
// shapes, not guesses. Deleted before shipping.

import (
	"fmt"
	"strings"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func dumpKt(n *sitter.Node, src []byte, depth int, sb *strings.Builder) {
	if n == nil {
		return
	}
	txt := nodeText(n, src)
	if len(txt) > 60 {
		txt = txt[:60] + "..."
	}
	txt = strings.ReplaceAll(txt, "\n", "\\n")
	named := ""
	if !n.IsNamed() {
		named = " (anon)"
	}
	fmt.Fprintf(sb, "%s%s%s  %q\n", strings.Repeat("  ", depth), n.Type(), named, txt)
	for i := 0; i < int(n.ChildCount()); i++ {
		dumpKt(n.Child(i), src, depth+1, sb)
	}
}

func TestZZProbeKotlinAST2(t *testing.T) {
	src := []byte(`package com.example

import org.springframework.web.bind.annotation.PostMapping

@RestController
class EdgeController {
    @PostMapping
    fun marker(): String = "m"

    @GetMapping("")
    fun empty(): String = "e"

    @GetMapping("\${app.prefix}/users")
    fun escapedDollar(): String = "p"

    @GetMapping("/a" + SUFFIX)
    fun concat(): String = "x"
}
`)
	root, ok := parseTreeSitterIfPossible(types.LangKotlin, src)
	if !ok {
		t.Fatal("no kotlin binding")
	}
	var sb strings.Builder
	dumpKt(root, src, 0, &sb)
	t.Log("\n" + sb.String())
}

func TestZZProbeKotlinAST(t *testing.T) {
	src := []byte(`package com.example

import org.springframework.web.bind.annotation.GetMapping
import org.springframework.web.bind.annotation.RequestMapping
import org.springframework.web.bind.annotation.RestController

@RestController
@RequestMapping("/api/users")
class UserController {
    @GetMapping("/{id}")
    fun get(): String = "g"

    @PostMapping(value = ["/x"])
    fun create(): String = "c"

    @PutMapping(path = "/y")
    fun update(): String = "u"

    @GetMapping(BASE)
    fun dyn(): String = "d"

    @GetMapping("/tmpl/${'$'}{x}")
    fun tmpl(): String = "t"

    @GetMapping("/interp/$x")
    fun interp(): String = "i"

    @org.springframework.web.bind.annotation.DeleteMapping("/q")
    fun qualified(): String = "q"
}
`)
	root, ok := parseTreeSitterIfPossible(types.LangKotlin, src)
	if !ok {
		t.Fatal("no kotlin binding")
	}
	var sb strings.Builder
	dumpKt(root, src, 0, &sb)
	t.Log("\n" + sb.String())
}
